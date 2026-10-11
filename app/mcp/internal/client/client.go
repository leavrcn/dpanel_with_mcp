// Package client DPanel 面板 API 客户端：JWT 登录 + 缓存 + 401 透明重登。
//
// 鉴权模型（对照 DPanel Go 源码验证）：
//   - POST /dpanel/api/common/user/login {username, password, autoLogin}
//     -> {code: 0, data: {token}}（HS256 JWT；24h，autoLogin 30d）
//   - 其余业务端点全部 POST + JSON body，要求 Authorization: Bearer <token>
//   - 成功信封 code=200 或 0（DPanel 历史版本两种都出现过，两种都认）
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ApiError struct {
	Message    string
	StatusCode int
	Body       string
}

func (e *ApiError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("dpanel api error (%d): %s", e.StatusCode, e.Message)
	}
	return "dpanel api error: " + e.Message
}

type Client struct {
	baseURL     string
	username    string
	password    string
	autoLogin   bool
	http        *http.Client
	token       string
	tokenExp    time.Time
	mu          sync.Mutex
	refreshSkew time.Duration
}

func New(baseURL, username, password string, autoLogin, tlsVerify bool, timeoutSec int) *Client {
	transport := &http.Transport{}
	if !tlsVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		username:    username,
		password:    password,
		autoLogin:   autoLogin,
		http:        &http.Client{Timeout: time.Duration(timeoutSec) * time.Second, Transport: transport},
		refreshSkew: 5 * time.Minute,
	}
}

// login 登录并缓存 JWT（并发安全）。
func (c *Client) login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginLocked(ctx)
}

func (c *Client) loginLocked(ctx context.Context) error {
	body, _ := json.Marshal(map[string]any{
		"username":  c.username,
		"password":  c.password,
		"autoLogin": c.autoLogin,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/dpanel/api/common/user/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return &ApiError{Message: "登录请求失败: " + err.Error()}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &ApiError{Message: "登录响应读取失败: " + err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return &ApiError{StatusCode: resp.StatusCode, Message: "登录失败", Body: string(raw)}
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Token       string `json:"token"`
			AccessToken string `json:"accessToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return &ApiError{Message: "登录响应解析失败: " + err.Error(), Body: string(raw)}
	}
	token := envelope.Data.Token
	if token == "" {
		token = envelope.Data.AccessToken
	}
	if token == "" {
		return &ApiError{Message: "登录响应缺少 token", Body: string(raw)}
	}
	c.token = token
	c.tokenExp = time.Now().Add(24 * time.Hour)
	if c.autoLogin {
		c.tokenExp = time.Now().Add(30 * 24 * time.Hour)
	}
	return nil
}

// tokenValid 缓存 token 是否仍可用（提前 refreshSkew 续期）。
func (c *Client) tokenValid() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token != "" && time.Now().Add(c.refreshSkew).Before(c.tokenExp)
}

// Post 调用 DPanel 业务 API（全部 POST + JSON）。
// 401 时透明重登一次再重试（DPanel 重启会使 JWT secret 轮换，此路径为必需而非优化）。
func (c *Client) Post(ctx context.Context, path string, payload any) (any, error) {
	c.mu.Lock()
	token := c.token
	expired := c.token == "" || !time.Now().Add(c.refreshSkew).Before(c.tokenExp)
	c.mu.Unlock()
	if expired {
		loginCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.login(loginCtx); err != nil {
			return nil, err
		}
		c.mu.Lock()
		token = c.token
		c.mu.Unlock()
	}

	data, err := c.postOnce(ctx, path, payload, token)
	if err == nil {
		return data, nil
	}
	var apiErr *ApiError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
		// 401：token 失效（面板重启/secret 轮换），重登一次
		loginCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err2 := c.login(loginCtx); err2 != nil {
			return nil, err2
		}
		c.mu.Lock()
		token = c.token
		c.mu.Unlock()
		return c.postOnce(ctx, path, payload, token)
	}
	return nil, err
}

func (c *Client) postOnce(ctx context.Context, path string, payload any, token string) (any, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader([]byte("{}"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/dpanel/api"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &ApiError{Message: err.Error()}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, &ApiError{Message: "响应读取失败: " + err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ApiError{StatusCode: resp.StatusCode, Message: http.StatusText(resp.StatusCode), Body: string(raw)}
	}
	// SPA 兜底：未知路由返回 200 + HTML，必须显式诊断而不是把整页 HTML 交给 agent
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		return nil, &ApiError{
			Message: fmt.Sprintf("endpoint not found on DPanel: %s（返回了 HTML 页面，通常是路由不存在或 DPanel 版本不匹配）", path),
		}
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data any    `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, &ApiError{Message: "响应解析失败: " + err.Error(), Body: string(raw)}
	}
	if envelope.Code != 0 && envelope.Code != http.StatusOK {
		msg := envelope.Msg
		if msg == "" {
			msg = fmt.Sprintf("code=%d", envelope.Code)
		}
		return nil, &ApiError{Message: msg, Body: string(raw)}
	}
	return envelope.Data, nil
}
