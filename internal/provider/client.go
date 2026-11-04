package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the Crontinel configuration API (/api/v1/apps/{slug}/...).
type Client struct {
	base string
	key  string
	ua   string
	http *http.Client
}

func NewClient(base, key, version string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"),
		key:  key,
		ua:   "terraform-provider-crontinel/" + version,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError is a non-2xx response in the API's {error, message, errors} shape.
type APIError struct {
	Status  int
	Code    string
	Message string
	Fields  map[string][]string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("%s (HTTP %d): %s", e.Code, e.Status, e.Message)
	for field, problems := range e.Fields {
		msg += fmt.Sprintf("; %s: %s", field, strings.Join(problems, " "))
	}
	return msg
}

func (e *APIError) IsNotFound() bool { return e.Status == http.StatusNotFound }
func (e *APIError) IsConflict() bool { return e.Status == http.StatusConflict }

// Do sends a request and decodes {"data": ...} into out (when out is not nil).
// It returns the HTTP status so callers can tell a create (201) from an update (200).
func (c *Client) Do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.ua)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	var resp *http.Response
	for attempt := 0; ; attempt++ {
		resp, err = c.http.Do(req)
		if err != nil {
			return 0, err
		}
		// 60 requests a minute: wait out a 429 instead of failing a large apply.
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= 4 {
			break
		}
		wait := 5 * time.Second
		if s := resp.Header.Get("Retry-After"); s != "" {
			var secs int
			if _, perr := fmt.Sscanf(s, "%d", &secs); perr == nil && secs > 0 && secs <= 60 {
				wait = time.Duration(secs) * time.Second
			}
		}
		resp.Body.Close()
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		if body != nil {
			b, _ := json.Marshal(body)
			req.Body = io.NopCloser(bytes.NewReader(b))
		}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode, Code: "http_error", Message: strings.TrimSpace(string(raw))}
		var parsed struct {
			Error   string              `json:"error"`
			Message string              `json:"message"`
			Errors  map[string][]string `json:"errors"`
		}
		if json.Unmarshal(raw, &parsed) == nil && parsed.Error != "" {
			apiErr.Code, apiErr.Message, apiErr.Fields = parsed.Error, parsed.Message, parsed.Errors
		}
		if len(apiErr.Message) > 300 {
			apiErr.Message = apiErr.Message[:300]
		}
		return resp.StatusCode, apiErr
	}
	if out != nil && len(raw) > 0 {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return resp.StatusCode, fmt.Errorf("unreadable response: %w", err)
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("unexpected response shape: %w", err)
		}
	}
	return resp.StatusCode, nil
}

func appPath(app, resource string) string {
	return "/api/v1/apps/" + url.PathEscape(app) + "/" + resource
}
