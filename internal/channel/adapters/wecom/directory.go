package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
)

const (
	wecomAPIBase       = "https://qyapi.weixin.qq.com/cgi-bin"
	tokenCacheDuration = 115 * time.Minute // 2 hours minus 5 minutes buffer
)

// tokenCache manages WeCom access_token with expiration
type tokenCache struct {
	token     string
	expiresAt time.Time
	mu        sync.RWMutex
}

// getToken returns a valid access_token, fetching a new one if expired
func (c *tokenCache) getToken(corpID, corpSecret string) (string, error) {
	c.mu.RLock()
	if c.token != "" && time.Now().Before(c.expiresAt) {
		token := c.token
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	// Need to fetch new token
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.token != "" && time.Now().Before(c.expiresAt) {
		return c.token, nil
	}

	token, err := fetchAccessToken(corpID, corpSecret)
	if err != nil {
		return "", err
	}

	c.token = token
	c.expiresAt = time.Now().Add(tokenCacheDuration)
	return token, nil
}

// fetchAccessToken fetches a new access_token from WeCom API
func fetchAccessToken(corpID, corpSecret string) (string, error) {
	if corpID == "" || corpSecret == "" {
		return "", fmt.Errorf("corp_id and corp_secret are required")
	}

	url := fmt.Sprintf("%s/gettoken?corpid=%s&corpsecret=%s", wecomAPIBase, corpID, corpSecret)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch access_token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode access_token response: %w", err)
	}

	if result.ErrCode != 0 {
		return "", fmt.Errorf("wecom API error %d: %s", result.ErrCode, result.ErrMsg)
	}

	if result.AccessToken == "" {
		return "", fmt.Errorf("access_token is empty")
	}

	return result.AccessToken, nil
}

// WeCom user structure from API response
type wecomUser struct {
	UserID     string `json:"userid"`
	Name       string `json:"name"`
	Department []int  `json:"department"`
	Avatar     string `json:"avatar,omitempty"`
	Email      string `json:"email,omitempty"`
	Mobile     string `json:"mobile,omitempty"`
}

// userListResponse is the response from user/simplelist API
type userListResponse struct {
	ErrCode  int         `json:"errcode"`
	ErrMsg   string      `json:"errmsg"`
	UserList []wecomUser `json:"userlist"`
}

// ListPeers lists users from WeCom contact directory
func (a *Adapter) ListPeers(ctx context.Context, cfg channel.ChannelConfig, query channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	config, err := ParseConfig(cfg.Credentials)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if config.CorpID == "" || config.CorpSecret == "" {
		return nil, fmt.Errorf("corp_id and corp_secret are required for directory lookup")
	}

	// Get access token
	token, err := a.getAccessToken(config)
	if err != nil {
		return nil, err
	}

	// Fetch user list
	users, err := a.fetchUserList(token)
	if err != nil {
		return nil, err
	}

	// Filter and convert to DirectoryEntry
	entries := make([]channel.DirectoryEntry, 0)
	queryStr := strings.ToLower(strings.TrimSpace(query.Query))

	for _, u := range users {
		entry := wecomUserToEntry(&u)

		// Apply query filter if provided
		if queryStr != "" {
			searchText := strings.ToLower(entry.Name + entry.Handle + entry.ID)
			if !strings.Contains(searchText, queryStr) {
				continue
			}
		}

		entries = append(entries, entry)

		// Respect limit
		if query.Limit > 0 && len(entries) >= query.Limit {
			break
		}
	}

	return entries, nil
}

// ListGroups returns empty list as WeCom doesn't expose group chats via API
func (a *Adapter) ListGroups(ctx context.Context, cfg channel.ChannelConfig, query channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	// WeCom doesn't provide API to list group chats
	return []channel.DirectoryEntry{}, nil
}

// ListGroupMembers returns error as WeCom doesn't support this operation
func (a *Adapter) ListGroupMembers(ctx context.Context, cfg channel.ChannelConfig, groupID string, query channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, fmt.Errorf("wecom does not support listing group members via API")
}

// ResolveEntry resolves a user by userid or name
func (a *Adapter) ResolveEntry(ctx context.Context, cfg channel.ChannelConfig, input string, kind channel.DirectoryEntryKind) (channel.DirectoryEntry, error) {
	if kind != channel.DirectoryEntryUser {
		return channel.DirectoryEntry{}, fmt.Errorf("wecom only supports user lookup, not group")
	}

	config, err := ParseConfig(cfg.Credentials)
	if err != nil {
		return channel.DirectoryEntry{}, fmt.Errorf("parse config: %w", err)
	}

	if config.CorpID == "" || config.CorpSecret == "" {
		return channel.DirectoryEntry{}, fmt.Errorf("corp_id and corp_secret are required for directory lookup")
	}

	// Get access token
	token, err := a.getAccessToken(config)
	if err != nil {
		return channel.DirectoryEntry{}, err
	}

	// Parse input - support "userid:xxx" format or plain userid
	userID := strings.TrimSpace(input)
	if strings.HasPrefix(strings.ToLower(userID), "userid:") {
		userID = strings.TrimSpace(userID[7:])
	}

	// Try to get user by ID first
	user, err := a.fetchUserByID(token, userID)
	if err == nil && user != nil {
		return wecomUserToEntry(user), nil
	}

	// If not found by ID, try searching by name
	users, err := a.fetchUserList(token)
	if err != nil {
		return channel.DirectoryEntry{}, err
	}

	inputLower := strings.ToLower(strings.TrimSpace(input))
	for _, u := range users {
		if strings.ToLower(u.Name) == inputLower || strings.ToLower(u.UserID) == inputLower {
			return wecomUserToEntry(&u), nil
		}
	}

	return channel.DirectoryEntry{}, fmt.Errorf("user not found: %s", input)
}

// getAccessToken gets a valid access token for the config
func (a *Adapter) getAccessToken(config *Config) (string, error) {
	// Use a per-adapter token cache
	if a.tokenCache == nil {
		a.tokenCache = &tokenCache{}
	}
	return a.tokenCache.getToken(config.CorpID, config.CorpSecret)
}

// fetchUserList fetches all users from WeCom
func (a *Adapter) fetchUserList(token string) ([]wecomUser, error) {
	// Use department_id=1 (root) and fetch_child=1 to get all users
	url := fmt.Sprintf("%s/user/simplelist?access_token=%s&department_id=1&fetch_child=1", wecomAPIBase, token)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user list: %w", err)
	}
	defer resp.Body.Close()

	var result userListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode user list response: %w", err)
	}

	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wecom API error %d: %s", result.ErrCode, result.ErrMsg)
	}

	return result.UserList, nil
}

// fetchUserByID fetches a specific user by ID
func (a *Adapter) fetchUserByID(token, userID string) (*wecomUser, error) {
	url := fmt.Sprintf("%s/user/get?access_token=%s&userid=%s", wecomAPIBase, token, userID)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		wecomUser
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wecom API error %d: %s", result.ErrCode, result.ErrMsg)
	}

	return &result.wecomUser, nil
}

// wecomUserToEntry converts a WeCom user to DirectoryEntry
func wecomUserToEntry(u *wecomUser) channel.DirectoryEntry {
	meta := map[string]any{
		"department_ids": u.Department,
	}
	if u.Email != "" {
		meta["email"] = u.Email
	}
	if u.Mobile != "" {
		meta["mobile"] = u.Mobile
	}

	return channel.DirectoryEntry{
		Kind:      channel.DirectoryEntryUser,
		ID:        fmt.Sprintf("userid:%s", u.UserID),
		Name:      u.Name,
		Handle:    u.UserID,
		AvatarURL: u.Avatar,
		Metadata:  meta,
	}
}
