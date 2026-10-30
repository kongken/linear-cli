// Package plane implements Plane's REST protocol, independent of Linear GraphQL.
package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kongken/linear-cli/internal/planeconfig"
)

type ClientOptions struct {
	HTTP *http.Client
	Wait func(context.Context, time.Duration) error
	Now  func() time.Time
}

type Client struct {
	wire        adapter
	directoryMu sync.Mutex
	directories map[string][]json.RawMessage
	Context     planeconfig.Context
	base        *url.URL
	token       string
	http        *http.Client
	wait        func(context.Context, time.Duration) error
	now         func() time.Time
}

type APIError struct {
	Status        int
	Code          string
	Detail        string
	RetryAfter    time.Duration
	UnknownResult bool
	Cause         error
}

func (e *APIError) Unwrap() error { return e.Cause }

func (e *APIError) Error() string {
	message := "Plane request failed"
	if e.Status != 0 {
		message = fmt.Sprintf("Plane HTTP %d", e.Status)
	}
	if e.Code != "" {
		message += " (" + e.Code + ")"
	}
	if e.Detail != "" {
		message += ": " + e.Detail
	}
	if e.UnknownResult {
		message += "; write result is unknown; read the resource before retrying"
	}
	if e.RetryAfter > 0 {
		message += fmt.Sprintf("; retry after %s", e.RetryAfter.Round(time.Second))
	}
	return message
}

func NewClient(cfg planeconfig.Context, token string, options ClientOptions) (*Client, error) {
	p, err := planeconfig.Normalize(cfg.Profile)
	if err != nil {
		return nil, err
	}
	cfg.Profile = p
	if token == "" {
		return nil, fmt.Errorf("no Plane token; set PLANE_API_KEY or run workctl plane auth login")
	}
	if strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("invalid Plane token")
	}
	base, _ := url.Parse(p.BaseURL)
	h := http.Client{Timeout: 30 * time.Second}
	if options.HTTP != nil {
		h = *options.HTTP
		if h.Timeout == 0 {
			h.Timeout = 30 * time.Second
		}
	}
	// Even same-origin redirects can rewrite a write's method; always surface them.
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	wait := options.Wait
	if wait == nil {
		wait = func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	var wire adapter = v2Adapter{}
	if cfg.APIVersion == "v1" {
		wire = v1Adapter{}
	}
	return &Client{wire: wire, directories: map[string][]json.RawMessage{}, Context: cfg, base: base, token: token, http: &h, wait: wait, now: now}, nil
}
func (c *Client) clean(text string) string {
	text = strings.ReplaceAll(text, c.token, "[redacted]")
	text = strings.ReplaceAll(text, url.QueryEscape(c.token), "[redacted]")
	return text
}
func (c *Client) Prefix() string { return "/api/" + c.Context.APIVersion + "/" }
func (c *Client) workspacePath() string {
	return c.Prefix() + "workspaces/" + url.PathEscape(c.Context.Workspace) + "/"
}
func segment(value string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\?#\x00\r\n") {
		return badInput("invalid resource reference")
	}
	return nil
}
func (c *Client) projectPath(project string) (string, error) {
	if err := segment(project); err != nil {
		return "", fmt.Errorf("project is required and must be a single reference")
	}
	return c.workspacePath() + "projects/" + url.PathEscape(project) + "/", nil
}

// RequestRaw permits only relative paths in the selected API version and retains raw JSON.
func (c *Client) RequestRaw(ctx context.Context, method, path string, body json.RawMessage) (json.RawMessage, error) {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete:
	default:
		return nil, badInput(fmt.Sprintf("unsupported HTTP method %q", method))
	}
	ref, err := url.ParseRequestURI(path)
	if err != nil || ref.IsAbs() || ref.Host != "" || !strings.HasPrefix(ref.Path, c.Prefix()) || ref.Fragment != "" || strings.Contains(ref.Path, "\\") {
		return nil, badInput(fmt.Sprintf("API path must be relative and start with %s", c.Prefix()))
	}
	for _, part := range strings.Split(ref.Path, "/") {
		if part == "." || part == ".." {
			return nil, badInput("API path cannot contain traversal segments")
		}
	}
	if _, err := url.ParseQuery(ref.RawQuery); err != nil {
		return nil, badInput("invalid API query")
	}
	if len(body) > 0 && !json.Valid(body) {
		return nil, badInput("request body must be valid JSON")
	}
	if method == http.MethodGet && len(body) > 0 {
		return nil, badInput("GET does not accept a request body")
	}
	if method != http.MethodGet {
		c.invalidateDirectories()
	}
	target := *c.base
	target.Path = strings.TrimRight(c.base.Path, "/") + ref.Path
	target.RawPath = ""
	target.RawQuery = ref.RawQuery
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create Plane request")
		}
		req.Header.Set("X-API-Key", c.token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "kongken-workctl")
		if len(body) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, requestErr := c.http.Do(req)
		var raw []byte
		var apiErr *APIError
		retry := false
		if requestErr != nil {
			if ctx.Err() != nil {
				if method != http.MethodGet {
					return nil, &APIError{Detail: ctx.Err().Error(), UnknownResult: true, Cause: ctx.Err()}
				}
				return nil, ctx.Err()
			}
			apiErr = &APIError{Detail: c.clean(requestErr.Error()), UnknownResult: method != http.MethodGet}
			var temporary interface{ Timeout() bool }
			retry = errors.As(requestErr, &temporary) && temporary.Timeout()
			var network interface{ Temporary() bool }
			retry = retry || errors.As(requestErr, &network) && network.Temporary()
		} else {
			raw, err = io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024+1))
			resp.Body.Close()
			if err != nil {
				apiErr = &APIError{Status: resp.StatusCode, Detail: "response interrupted", UnknownResult: method != http.MethodGet, Cause: ctx.Err()}
				retry = true
			} else if len(raw) > 16*1024*1024 {
				return nil, &APIError{Status: resp.StatusCode, Detail: "response exceeds 16 MiB; reduce page size", UnknownResult: method != http.MethodGet}
			}
			if apiErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if resp.StatusCode == http.StatusNoContent {
					return nil, nil
				}
				if !json.Valid(raw) {
					return nil, &APIError{Status: resp.StatusCode, Detail: "expected a JSON response", UnknownResult: method != http.MethodGet}
				}
				return json.RawMessage(raw), nil
			}
			if apiErr == nil {
				var problem struct {
					Code   string          `json:"code"`
					Detail string          `json:"detail"`
					Error  string          `json:"error"`
					Errors json.RawMessage `json:"errors"`
				}
				_ = json.Unmarshal(raw, &problem)
				detail := problem.Detail
				if detail == "" {
					detail = problem.Error
				}
				if len(problem.Errors) > 0 {
					detail += " " + string(problem.Errors)
				}
				if detail == "" && json.Valid(raw) {
					detail = string(raw)
				}
				if detail == "" {
					detail = http.StatusText(resp.StatusCode)
				}
				detail = c.clean(detail)
				if len(detail) > 4096 {
					detail = detail[:4096] + "…"
				}
				apiErr = &APIError{Status: resp.StatusCode, Code: c.clean(problem.Code), Detail: c.clean(detail), UnknownResult: method != http.MethodGet && resp.StatusCode >= 500}
			}
			apiErr.RetryAfter = c.retryAfter(resp.Header)
			retry = retry || resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504
		}
		if method != http.MethodGet || !retry || attempt == 2 {
			return nil, apiErr
		}
		delay := apiErr.RetryAfter
		if delay <= 0 {
			delay = time.Duration(1<<attempt)*time.Second + time.Duration(rand.Intn(200))*time.Millisecond
		}
		if deadline, ok := ctx.Deadline(); ok && c.now().Add(delay).After(deadline) {
			return nil, apiErr
		}
		if err := c.wait(ctx, delay); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("Plane request failed")
}
func (c *Client) retryAfter(h http.Header) time.Duration {
	if n, err := strconv.Atoi(h.Get("Retry-After")); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if when, err := http.ParseTime(h.Get("Retry-After")); err == nil && when.After(c.now()) {
		return when.Sub(c.now())
	}
	if epoch, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		if d := time.Unix(epoch, 0).Sub(c.now()); d > 0 {
			return d
		}
	}
	return 0
}
func (c *Client) CurrentUser(ctx context.Context) (json.RawMessage, error) {
	raw, err := c.RequestRaw(ctx, http.MethodGet, c.Prefix()+"users/me/", nil)
	if err != nil {
		return nil, err
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &user); err != nil || user.ID == "" {
		return nil, fmt.Errorf("invalid current-user response: missing ID")
	}
	return raw, nil
}
