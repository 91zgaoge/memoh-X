package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	defaultToolRegistryCacheTTL = 5 * time.Second
)

type cachedToolRegistry struct {
	expiresAt time.Time
	registry  *ToolRegistry
}

// ToolGatewayService federates tools from executors and sources.
type ToolGatewayService struct {
	logger    *slog.Logger
	executors []ToolExecutor
	sources   []ToolSource
	cacheTTL  time.Duration

	mu    sync.Mutex
	cache map[string]cachedToolRegistry
}

func NewToolGatewayService(log *slog.Logger, executors []ToolExecutor, sources []ToolSource) *ToolGatewayService {
	if log == nil {
		log = slog.Default()
	}
	filteredExecutors := make([]ToolExecutor, 0, len(executors))
	for _, executor := range executors {
		if executor != nil {
			filteredExecutors = append(filteredExecutors, executor)
		}
	}
	filteredSources := make([]ToolSource, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			filteredSources = append(filteredSources, source)
		}
	}
	return &ToolGatewayService{
		logger:    log.With(slog.String("service", "tool_gateway")),
		executors: filteredExecutors,
		sources:   filteredSources,
		cacheTTL:  defaultToolRegistryCacheTTL,
		cache:     map[string]cachedToolRegistry{},
	}
}

func (s *ToolGatewayService) InitializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities": map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]any{
			"name":    "memoh-tools-gateway",
			"version": "1.0.0",
		},
	}
}

func (s *ToolGatewayService) ListTools(ctx context.Context, session ToolSessionContext) ([]ToolDescriptor, error) {
	registry, err := s.getRegistry(ctx, session, false)
	if err != nil {
		return nil, err
	}
	return registry.List(), nil
}

func (s *ToolGatewayService) CallTool(ctx context.Context, session ToolSessionContext, payload ToolCallPayload) (map[string]any, error) {
	toolName := strings.TrimSpace(payload.Name)
	if toolName == "" {
		return nil, fmt.Errorf("tool name is required")
	}

	registry, err := s.getRegistry(ctx, session, false)
	if err != nil {
		return nil, err
	}
	executor, _, ok := registry.Lookup(toolName)
	if !ok {
		// Refresh once for dynamic executors/sources.
		registry, err = s.getRegistry(ctx, session, true)
		if err != nil {
			return nil, err
		}
		executor, _, ok = registry.Lookup(toolName)
		if !ok {
			return BuildToolErrorResult("tool not found: " + toolName), nil
		}
	}

	arguments := payload.Arguments
	if arguments == nil {
		arguments = map[string]any{}
	}
	result, err := executor.CallTool(ctx, session, toolName, arguments)
	if err != nil {
		if errors.Is(err, ErrToolNotFound) {
			return BuildToolErrorResult("tool not found: " + toolName), nil
		}
		// If built-in executor is not capable (e.g., bot has no image model),
		// try fallback to federation sources
		if errors.Is(err, ErrToolNotCapable) {
			s.logger.Info("tool not capable from built-in, trying federation fallback",
				slog.String("tool", toolName),
				slog.String("bot_id", session.BotID),
				slog.Int("source_count", len(s.sources)))
			fallbackResult, fallbackErr := s.tryFederationFallback(ctx, session, toolName, arguments)
			if fallbackErr == nil && fallbackResult != nil {
				s.logger.Info("federation fallback succeeded",
					slog.String("tool", toolName))
				return fallbackResult, nil
			}
			s.logger.Warn("federation fallback failed",
				slog.String("tool", toolName),
				slog.Any("error", fallbackErr))
		}
		return BuildToolErrorResult(err.Error()), nil
	}
	if result == nil {
		return BuildToolSuccessResult(map[string]any{"ok": true}), nil
	}
	return result, nil
}

// tryFederationFallback attempts to find and call a matching tool from federation sources
// when built-in executor returns ErrToolNotCapable. It looks for tools with similar names.
func (s *ToolGatewayService) tryFederationFallback(ctx context.Context, session ToolSessionContext, originalToolName string, arguments map[string]any) (map[string]any, error) {
	// Try to find matching tools from federation sources
	// Look for tools with names containing "image" and "generate" (in any order)
	for _, source := range s.sources {
		tools, err := source.ListTools(ctx, session)
		if err != nil {
			s.logger.Warn("failed to list tools from source for fallback",
				slog.Any("error", err))
			continue
		}
			// Collect all tool names for debugging
		var toolNames []string
		for _, tool := range tools {
			toolNames = append(toolNames, tool.Name)
		}
		s.logger.Info("trying federation source for fallback",
			slog.String("source", fmt.Sprintf("%T", source)),
			slog.Int("tool_count", len(tools)),
			slog.Any("tools", toolNames))

		for _, tool := range tools {
			toolName := strings.ToLower(tool.Name)
			// Look for tools matching common image generation patterns
			// Support patterns like: z-image.generate_image_tool, generate_image, image_generate, etc.
			hasGenerate := strings.Contains(toolName, "generate") || strings.Contains(toolName, "gen")
			hasImage := strings.Contains(toolName, "image") || strings.Contains(toolName, "img") || strings.Contains(toolName, "picture") || strings.Contains(toolName, "draw")

			s.logger.Info("checking tool for fallback match",
				slog.String("tool", tool.Name),
				slog.String("toolName_lower", toolName),
				slog.Bool("hasGenerate", hasGenerate),
				slog.Bool("hasImage", hasImage))

			if hasGenerate && hasImage {
				s.logger.Info("found federation fallback tool",
					slog.String("original", originalToolName),
					slog.String("fallback", tool.Name))
				return source.CallTool(ctx, session, tool.Name, arguments)
			}
		}
	}
	return nil, fmt.Errorf("no suitable federation fallback found for %s", originalToolName)
}

func (s *ToolGatewayService) getRegistry(ctx context.Context, session ToolSessionContext, force bool) (*ToolRegistry, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, fmt.Errorf("bot id is required")
	}
	if !force {
		s.mu.Lock()
		cached, ok := s.cache[botID]
		if ok && time.Now().Before(cached.expiresAt) && cached.registry != nil {
			s.mu.Unlock()
			return cached.registry, nil
		}
		s.mu.Unlock()
	}

	registry := NewToolRegistry()
	for _, executor := range s.executors {
		tools, err := executor.ListTools(ctx, session)
		if err != nil {
			s.logger.Warn("list tools from executor failed", slog.Any("error", err))
			continue
		}
		for _, tool := range tools {
			if err := registry.Register(executor, tool); err != nil {
				s.logger.Warn("skip duplicated/invalid tool", slog.String("tool", tool.Name), slog.Any("error", err))
			}
		}
	}
	for _, source := range s.sources {
		tools, err := source.ListTools(ctx, session)
		if err != nil {
			s.logger.Warn("list tools from source failed", slog.Any("error", err))
			continue
		}
		for _, tool := range tools {
			if err := registry.Register(source, tool); err != nil {
				s.logger.Warn("skip duplicated/invalid tool", slog.String("tool", tool.Name), slog.Any("error", err))
			}
		}
	}

	s.mu.Lock()
	s.cache[botID] = cachedToolRegistry{
		expiresAt: time.Now().Add(s.cacheTTL),
		registry:  registry,
	}
	s.mu.Unlock()
	return registry, nil
}
