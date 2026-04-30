package openlist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"openlist-tvbox/internal/config"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124 Safari/537.36"

type Client struct {
	http       *http.Client
	logger     *slog.Logger
	authMu     sync.Mutex
	authStates map[string]*authState
}

type authState struct {
	token   string
	waiting chan struct{}
}

func NewClient(httpClient *http.Client, logger *slog.Logger) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{http: httpClient, logger: logger, authStates: map[string]*authState{}}
}

func (c *Client) List(ctx context.Context, backend config.Backend, path, password string) ([]Item, error) {
	return c.list(ctx, backend, path, password, false)
}

func (c *Client) RefreshList(ctx context.Context, backend config.Backend, path, password string) ([]Item, error) {
	return c.list(ctx, backend, path, password, true)
}

func (c *Client) list(ctx context.Context, backend config.Backend, path, password string, refresh bool) ([]Item, error) {
	var out struct {
		Data struct {
			Content []Item `json:"content"`
		} `json:"data"`
	}
	body := map[string]any{"path": path, "password": password}
	if refresh {
		body["refresh"] = true
	}
	if err := c.post(ctx, backend, "/api/fs/list", body, &out); err != nil {
		return nil, err
	}
	if out.Data.Content == nil {
		return []Item{}, nil
	}
	return out.Data.Content, nil
}

func (c *Client) Get(ctx context.Context, backend config.Backend, path, password string) (Item, error) {
	var out struct {
		Data Item `json:"data"`
	}
	if err := c.post(ctx, backend, "/api/fs/get", map[string]any{"path": path, "password": password}, &out); err != nil {
		return Item{}, err
	}
	return out.Data, nil
}

func (c *Client) Search(ctx context.Context, backend config.Backend, path, keyword, password string) ([]Item, error) {
	var out struct {
		Data struct {
			Content []Item `json:"content"`
		} `json:"data"`
	}
	body := map[string]any{"parent": path, "keywords": keyword, "scope": 0, "page": 1, "per_page": 100, "password": password}
	if err := c.post(ctx, backend, "/api/fs/search", body, &out); err != nil {
		return nil, err
	}
	if out.Data.Content == nil {
		return []Item{}, nil
	}
	return out.Data.Content, nil
}

func (c *Client) post(ctx context.Context, backend config.Backend, apiPath string, body any, out any) error {
	err := c.postOnce(ctx, backend, apiPath, body, out, false)
	if _, ok := err.(authorizationError); ok && authType(backend) == "password" {
		c.invalidateToken(backend.ID)
		err = c.postOnce(ctx, backend, apiPath, body, out, true)
	}
	return err
}

func (c *Client) postOnce(ctx context.Context, backend config.Backend, apiPath string, body any, out any, forceLogin bool) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.Server+apiPath, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if err := c.authorize(ctx, req, backend, forceLogin); err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("openlist request failed")
	}
	defer resp.Body.Close()
	return decodeResponse(resp, out)
}

func (c *Client) authorize(ctx context.Context, req *http.Request, backend config.Backend, forceLogin bool) error {
	switch authType(backend) {
	case "api_key":
		req.Header.Set("Authorization", backend.APIKey)
	case "password":
		req.Header.Set("Client-Id", clientID(backend))
		token, err := c.token(ctx, backend, forceLogin)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", token)
	}
	return nil
}

func authType(backend config.Backend) string {
	return backend.AuthType
}

func (c *Client) token(ctx context.Context, backend config.Backend, force bool) (string, error) {
	state := c.backendAuthState(backend.ID)
	for {
		c.authMu.Lock()
		if state.token != "" && !force {
			token := state.token
			c.authMu.Unlock()
			return token, nil
		}
		if state.waiting != nil {
			waiting := state.waiting
			c.authMu.Unlock()
			select {
			case <-waiting:
				force = false
				continue
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		waiting := make(chan struct{})
		state.waiting = waiting
		c.authMu.Unlock()
		token, err := c.login(ctx, backend)
		c.authMu.Lock()
		if err == nil {
			state.token = token
		}
		state.waiting = nil
		close(waiting)
		c.authMu.Unlock()
		return token, err
	}
}

func (c *Client) backendAuthState(backendID string) *authState {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	state := c.authStates[backendID]
	if state == nil {
		state = &authState{}
		c.authStates[backendID] = state
	}
	return state
}

func (c *Client) invalidateToken(backendID string) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if state := c.authStates[backendID]; state != nil {
		state.token = ""
	}
}

func (c *Client) login(ctx context.Context, backend config.Backend) (string, error) {
	body := map[string]string{
		"username": backend.User,
		"password": backend.Password,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.Server+"/api/auth/login", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Client-Id", clientID(backend))
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("openlist login failed")
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := decodeResponse(resp, &out); err != nil {
		if _, ok := err.(authorizationError); ok {
			return "", fmt.Errorf("openlist login failed; check backend username/password")
		}
		return "", err
	}
	if out.Data.Token == "" {
		return "", fmt.Errorf("openlist login returned empty token")
	}
	return out.Data.Token, nil
}

func clientID(backend config.Backend) string {
	return "openlist-tvbox-" + backend.ID
}

type authorizationError struct{}

func (authorizationError) Error() string {
	return "openlist authorization failed; check backend credentials"
}

func decodeResponse(resp *http.Response, out any) error {
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return authorizationError{}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("openlist returned status %d", resp.StatusCode)
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("invalid openlist response")
	}
	if envelope.Code != 0 && envelope.Code != 200 {
		msg := strings.ToLower(envelope.Message)
		if strings.Contains(msg, "token") || strings.Contains(msg, "authorization") || strings.Contains(msg, "guest user is disabled") {
			return authorizationError{}
		}
		return fmt.Errorf("openlist api error: %s", sanitizeMessage(envelope.Message))
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("invalid openlist response shape")
	}
	return nil
}

func sanitizeMessage(msg string) string {
	msg = strings.ReplaceAll(msg, "\n", " ")
	if len(msg) > 160 {
		return msg[:160]
	}
	return msg
}
