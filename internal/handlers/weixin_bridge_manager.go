package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// BridgeInfo holds runtime information about a weixin-bridge container
type BridgeInfo struct {
	BotID         string    `json:"bot_id"`
	ContainerName string    `json:"container_name"`
	Port          int       `json:"port"`
	Status        string    `json:"status"` // running, stopped, error, pending
	CreatedAt     time.Time `json:"created_at"`
	LastError     string    `json:"last_error,omitempty"`
}

// WeixinBridgeManager handles dynamic creation and management of weixin-bridge containers
type WeixinBridgeManager struct {
	dockerClient *http.Client
	bridges      map[string]*BridgeInfo // botID -> BridgeInfo
	mu           sync.RWMutex
	nextPort     int
	portMu       sync.Mutex
	logger       *slog.Logger
}

// NewWeixinBridgeManager creates a new bridge manager
func NewWeixinBridgeManager(logger *slog.Logger) *WeixinBridgeManager {
	if logger == nil {
		logger = slog.Default()
	}

	// Create HTTP client that talks to Docker via unix socket
	dockerClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
		Timeout: 30 * time.Second,
	}

	return &WeixinBridgeManager{
		dockerClient: dockerClient,
		bridges:      make(map[string]*BridgeInfo),
		nextPort:     3001, // Start from 3001, reserve 3000 for default bridge
		logger:       logger.With(slog.String("handler", "weixin_bridge_manager")),
	}
}

// Register registers the bridge management routes
func (h *WeixinBridgeManager) Register(e *echo.Echo) {
	group := e.Group("/api/weixin-bridge")

	// Management endpoints
	group.POST("", h.StartBridge)
	group.DELETE("/:bot_id", h.StopBridge)
	group.GET("/:bot_id/info", h.GetBridgeInfo)

	// Proxy endpoints - forward to bridge container
	group.GET("/:bot_id/status", h.ProxyToBridge)
	group.GET("/:bot_id/qrcode", h.ProxyToBridge)
	group.GET("/:bot_id/health", h.ProxyToBridge)
	group.GET("/:bot_id/qrcode.txt", h.ProxyToBridge)
	group.GET("/:bot_id/qrcode-image", h.ProxyToBridge)
	group.GET("/:bot_id/events", h.ProxyToBridge)
}

// StartBridgeRequest represents the request to start a bridge container
type StartBridgeRequest struct {
	BotID   string `json:"bot_id"`
	APIKey  string `json:"api_key"`
	BotName string `json:"bot_name,omitempty"` // Optional, for container naming
}

// StartBridge creates and starts a weixin-bridge container for the specified bot
func (h *WeixinBridgeManager) StartBridge(c echo.Context) error {
	var req StartBridgeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid request body",
		})
	}

	if strings.TrimSpace(req.BotID) == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "bot_id is required",
		})
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if bridge already exists
	if info, exists := h.bridges[req.BotID]; exists && info.Status == "running" {
		return c.JSON(http.StatusConflict, map[string]any{
			"success": false,
			"error":   "bridge already running for this bot",
			"info":    info,
		})
	}

	// Allocate port
	port := h.allocatePort(req.BotID)

	// Generate container name
	botShort := req.BotID
	if len(botShort) > 8 {
		botShort = botShort[:8]
	}
	containerName := fmt.Sprintf("memoh-weixin-bridge-%s", botShort)

	// Create bridge info
	info := &BridgeInfo{
		BotID:         req.BotID,
		ContainerName: containerName,
		Port:          port,
		Status:        "pending",
		CreatedAt:     time.Now(),
	}
	h.bridges[req.BotID] = info

	// Start container in background
	go func() {
		err := h.createAndStartContainer(info, req.APIKey)
		if err != nil {
			h.mu.Lock()
			info.Status = "error"
			info.LastError = err.Error()
			h.mu.Unlock()
			h.logger.Error("failed to start bridge container",
				slog.String("bot_id", req.BotID),
				slog.String("error", err.Error()),
			)
		}
	}()

	return c.JSON(http.StatusAccepted, map[string]any{
		"success": true,
		"message": "bridge container is starting",
		"info":    info,
	})
}

// StopBridge stops and removes a bridge container
func (h *WeixinBridgeManager) StopBridge(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "bot_id is required",
		})
	}

	h.mu.Lock()
	info, exists := h.bridges[botID]
	if exists {
		delete(h.bridges, botID)
	}
	h.mu.Unlock()

	// If not in memory, try to discover from container
	if !exists {
		info = h.discoverBridgeFromContainer(botID)
	}

	// Generate container name for cleanup
	botShort := botID
	if len(botShort) > 8 {
		botShort = botShort[:8]
	}
	containerName := fmt.Sprintf("memoh-weixin-bridge-%s", botShort)

	// Use discovered info or default to generated name
	if info != nil && info.ContainerName != "" {
		containerName = info.ContainerName
	}

	// Stop and remove container
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop container
	stopReq, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://localhost/containers/%s/stop?t=10", containerName), nil)
	h.dockerClient.Do(stopReq)

	// Remove container
	delReq, _ := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("http://localhost/containers/%s?force=true", containerName), nil)
	resp, err := h.dockerClient.Do(delReq)
	if err != nil {
		h.logger.Error("failed to remove container",
			slog.String("container", containerName),
			slog.String("error", err.Error()),
		)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   "failed to remove container: " + err.Error(),
		})
	}
	defer resp.Body.Close()

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"message": "bridge stopped and removed",
	})
}

// GetBridgeInfo returns information about a bridge
func (h *WeixinBridgeManager) GetBridgeInfo(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "bot_id is required",
		})
	}

	h.mu.RLock()
	info, exists := h.bridges[botID]
	h.mu.RUnlock()

	if !exists {
		// Try to discover from existing container
		info = h.discoverBridgeFromContainer(botID)
		if info == nil {
			return c.JSON(http.StatusNotFound, map[string]any{
				"success": false,
				"error":   "bridge not found",
			})
		}
	}

	// Refresh status from container
	if info.Status == "running" || info.Status == "pending" {
		status := h.getContainerStatus(info.ContainerName)
		if status != "" {
			h.mu.Lock()
			info.Status = status
			h.mu.Unlock()
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"info":    info,
	})
}

// discoverBridgeFromContainer tries to find an existing bridge container for the bot
func (h *WeixinBridgeManager) discoverBridgeFromContainer(botID string) *BridgeInfo {
	botShort := botID
	if len(botShort) > 8 {
		botShort = botShort[:8]
	}
	containerName := fmt.Sprintf("memoh-weixin-bridge-%s", botShort)

	// Check if container exists
	status := h.getContainerStatus(containerName)
	if status == "" || status == "stopped" {
		return nil
	}

	// Get container details to find port
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("http://localhost/containers/%s/json", containerName), nil)
	if err != nil {
		return nil
	}

	resp, err := h.dockerClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var result struct {
		NetworkSettings struct {
			Ports map[string][]struct {
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	// Find the port
	port := 0
	for _, bindings := range result.NetworkSettings.Ports {
		for _, binding := range bindings {
			if binding.HostPort != "" {
				fmt.Sscanf(binding.HostPort, "%d", &port)
				break
			}
		}
		if port > 0 {
			break
		}
	}

	if port == 0 {
		return nil
	}

	// Register bridge
	info := &BridgeInfo{
		BotID:         botID,
		ContainerName: containerName,
		Port:          port,
		Status:        status,
		CreatedAt:     time.Now(),
	}

	h.mu.Lock()
	h.bridges[botID] = info
	// Update nextPort if needed
	if port >= h.nextPort {
		h.nextPort = port + 1
	}
	h.mu.Unlock()

	h.logger.Info("discovered existing bridge container",
		slog.String("bot_id", botID),
		slog.String("container", containerName),
		slog.Int("port", port),
	)

	return info
}

// ProxyToBridge proxies requests to the bridge container
func (h *WeixinBridgeManager) ProxyToBridge(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "bot_id is required",
		})
	}

	h.mu.RLock()
	info, exists := h.bridges[botID]
	h.mu.RUnlock()

	if !exists {
		// Try to discover from existing container
		info = h.discoverBridgeFromContainer(botID)
		if info == nil {
			return c.JSON(http.StatusNotFound, map[string]any{
				"success": false,
				"error":   "bridge not found",
			})
		}
	}

	// Build target URL - use container name since both are on the same Docker network
	targetURL, err := url.Parse(fmt.Sprintf("http://%s:3000", info.ContainerName))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   "failed to parse target URL",
		})
	}

	// Create reverse proxy with path rewriting
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Rewrite path: /api/weixin-bridge/{bot_id}/xxx -> /xxx
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Strip the /api/weixin-bridge/{bot_id} prefix
		prefix := fmt.Sprintf("/api/weixin-bridge/%s", botID)
		req.URL.Path = strings.TrimPrefix(req.URL.Path, prefix)
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		req.Host = targetURL.Host
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		h.logger.Error("proxy error",
			slog.String("bot_id", botID),
			slog.String("target", targetURL.String()),
			slog.String("error", err.Error()),
		)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "bridge unavailable: " + err.Error(),
		})
	}

	// Serve proxy
	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

// allocatePort assigns a unique port for a bot
func (h *WeixinBridgeManager) allocatePort(botID string) int {
	h.portMu.Lock()
	defer h.portMu.Unlock()

	// Check if already allocated
	if info, ok := h.bridges[botID]; ok {
		return info.Port
	}

	port := h.nextPort
	h.nextPort++
	return port
}

// createAndStartContainer creates and starts a Docker container
func (h *WeixinBridgeManager) createAndStartContainer(info *BridgeInfo, apiKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Prepare volume name
	botShort := info.BotID
	if len(botShort) > 8 {
		botShort = botShort[:8]
	}
	volumeName := fmt.Sprintf("weixin_data_%s", botShort)

	// Create container config
	config := map[string]any{
		"Image": "memoh-weixin-bridge:latest",
		"Env": []string{
			fmt.Sprintf("MEMOH_BOT_ID=%s", info.BotID),
			fmt.Sprintf("MEMOH_API_KEY=%s", apiKey),
			"MEMOH_SERVER_URL=http://memoh-server:8080",
			"TOKEN_PATH=/data/.weixin-bot/credentials.json",
			fmt.Sprintf("PUBLIC_BASE_URL=https://wework.jxtvnet.com/api/weixin-bridge/%s", info.BotID),
		},
		"ExposedPorts": map[string]any{
			"3000/tcp": map[string]any{},
		},
		"HostConfig": map[string]any{
			"PortBindings": map[string]any{
				"3000/tcp": []map[string]string{
					{"HostPort": fmt.Sprintf("%d", info.Port)},
				},
			},
			"Binds": []string{
				fmt.Sprintf("%s:/data/.weixin-bot", volumeName),
			},
			"NetworkMode": "memoh_memoh-network",
			"RestartPolicy": map[string]any{
				"Name": "unless-stopped",
			},
		},
		"Labels": map[string]string{
			"memoh.managed":  "true",
			"memoh.bot_id":   info.BotID,
			"memoh.service":  "weixin-bridge",
		},
	}

	configBody, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal container config: %w", err)
	}

	// Create container
	createReq, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://localhost/containers/create?name=%s", info.ContainerName),
		bytes.NewReader(configBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := h.dockerClient.Do(createReq)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("failed to create container: %s - %s", createResp.Status, string(body))
	}

	var createResult struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createResult); err != nil {
		return fmt.Errorf("failed to decode create response: %w", err)
	}

	// Start container
	startReq, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://localhost/containers/%s/start", createResult.ID), nil)
	if err != nil {
		return fmt.Errorf("failed to create start request: %w", err)
	}

	startResp, err := h.dockerClient.Do(startReq)
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	defer startResp.Body.Close()

	if startResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(startResp.Body)
		return fmt.Errorf("failed to start container: %s - %s", startResp.Status, string(body))
	}

	// Update status
	h.mu.Lock()
	info.Status = "running"
	h.mu.Unlock()

	h.logger.Info("bridge container started",
		slog.String("bot_id", info.BotID),
		slog.String("container", info.ContainerName),
		slog.Int("port", info.Port),
	)

	return nil
}

// getContainerStatus queries Docker for container status
func (h *WeixinBridgeManager) getContainerStatus(containerName string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("http://localhost/containers/%s/json", containerName), nil)
	if err != nil {
		return "error"
	}

	resp, err := h.dockerClient.Do(req)
	if err != nil {
		return "stopped"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "stopped"
	}

	var result struct {
		State struct {
			Running bool   `json:"Running"`
			Status  string `json:"Status"`
		} `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "error"
	}

	if result.State.Running {
		return "running"
	}
	return "stopped"
}
