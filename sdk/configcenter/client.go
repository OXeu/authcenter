// Package configcenter is the Go client for a configcenter server.
//
// The client authenticates with an API key issued for one project. It carries
// no project name: the key itself is bound to a project, so a key can never
// read another project's configuration.
package configcenter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNotFound is returned when the server has no such project or environment.
	ErrNotFound = errors.New("configcenter: not found")
	// ErrKeyNotFound is returned when a key is absent from a loaded snapshot.
	ErrKeyNotFound = errors.New("configcenter: key not found")
)

// Config describes how to reach a server.
type Config struct {
	// BaseURL is the server root, for example http://127.0.0.1:8080.
	BaseURL string
	// APIKey is the token created on the project's API keys page.
	APIKey string
	// Environment is the default environment slug; empty means production.
	Environment string
	// HTTPClient overrides the default client.
	HTTPClient *http.Client
	// Timeout overrides the default request timeout of 30 seconds.
	Timeout time.Duration
}

// Item is one configuration entry.
type Item struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WatchOptions controls the polling loop.
type WatchOptions struct {
	Interval time.Duration
	// OnError is called for transient failures; nil means retry silently.
	OnError func(error)
}

// Client reads configuration of the API key's project.
type Client struct {
	endpoint string
	apiKey   string
	env      string
	http     *http.Client

	mu    sync.RWMutex
	items map[string]string
	order []Item
}

// New validates cfg and returns a client.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("configcenter: BaseURL is required")
	}
	if !strings.HasPrefix(cfg.APIKey, "cc_") {
		return nil, errors.New("configcenter: a cc_ API key is required")
	}
	base, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("configcenter: invalid BaseURL %q", cfg.BaseURL)
	}
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	env := cfg.Environment
	if env == "" {
		env = "production"
	}
	return &Client{
		endpoint: base.String() + "/api/v1/agent",
		apiKey:   cfg.APIKey,
		env:      env,
		http:     client,
		items:    map[string]string{},
	}, nil
}

// Environment returns the environment slug the client reads.
func (c *Client) Environment() string {
	return c.env
}

// Load fetches every item of the environment and replaces the snapshot.
func (c *Client) Load(ctx context.Context) error {
	return c.load(ctx, "")
}

// LoadPrefix fetches the items whose keys start with prefix.
func (c *Client) LoadPrefix(ctx context.Context, prefix string) error {
	return c.load(ctx, prefix)
}

func (c *Client) load(ctx context.Context, prefix string) error {
	query := url.Values{}
	if prefix != "" {
		query.Set("prefix", prefix)
	}
	var body struct {
		Items []Item `json:"items"`
	}
	path := "/environments/" + url.PathEscape(c.env) + "/values"
	if err := c.get(ctx, path, query, &body); err != nil {
		return err
	}
	c.replace(body.Items)
	return nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	target := c.endpoint + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return fmt.Errorf("configcenter: build request: %w", err)
	}
	req.Header.Set("Authorization", "ApiKey "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("configcenter: request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // a close error cannot be reported after the body was read
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("configcenter: %s: %w", path, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		// The status code is the useful error; the body text is not needed. //nolint:errcheck
		return fmt.Errorf("configcenter: %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("configcenter: decode %s: %w", path, err)
	}
	return nil
}

func (c *Client) replace(items []Item) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]string, len(items))
	c.order = items
	for _, item := range items {
		c.items[item.Key] = item.Value
	}
}

// Get returns the value of key from the snapshot.
func (c *Client) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.items[key]
	return value, ok
}

// GetDefault returns the snapshot value of key or fallback.
func (c *Client) GetDefault(key, fallback string) string {
	if value, ok := c.Get(key); ok {
		return value
	}
	return fallback
}

// Int parses the snapshot value of key as an integer, falling back on error.
func (c *Client) Int(key string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(c.GetDefault(key, "")))
	if err != nil {
		return fallback
	}
	return n
}

// Bool parses the snapshot value of key as a boolean, falling back on error.
func (c *Client) Bool(key string, fallback bool) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(c.GetDefault(key, "")))
	if err != nil {
		return fallback
	}
	return b
}

// All returns the snapshot sorted by key.
func (c *Client) All() []Item {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := append([]Item(nil), c.order...)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Watch reloads on interval until ctx is cancelled. The first load happens
// before Watch returns control, so callers can use the snapshot right away.
func (c *Client) Watch(ctx context.Context, opts WatchOptions) error {
	if opts.Interval <= 0 {
		opts.Interval = 30 * time.Second
	}
	if err := c.Load(ctx); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(opts.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.Load(ctx); err != nil && opts.OnError != nil {
					opts.OnError(err)
				}
			}
		}
	}()
	return nil
}
