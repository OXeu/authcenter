package configcenter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"sort"
	"strings"
	"time"
)

// EnsureDefaults creates missing keys on the remote server or in memory with TTL.
// It preserves existing backend values and reloads the snapshot, bypassing cached reads.
// Remote initialization requires a server with the defaults endpoint and config:write permission.
func (c *Client) EnsureDefaults(ctx context.Context, defaults map[string]string) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("configcenter: initialize defaults cancelled: %w", err)
	}
	keys := make([]string, 0, len(defaults))
	for key := range defaults {
		if err := validateKey(key); err != nil {
			return fmt.Errorf("initialize default key %q: %w", key, err)
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	if err := c.ensureDefaults(ctx, defaults); err != nil {
		return fmt.Errorf("initialize default keys [%s]: %w", strings.Join(keys, ", "), err)
	}
	return nil
}

func (c *Client) ensureDefaults(ctx context.Context, defaults map[string]string) error {
	if c.IsLocal() {
		c.mu.Lock()
		now := time.Now()
		c.cache.prune(now)
		for key, value := range defaults {
			if _, exists := c.cache.get(key, now); !exists {
				c.cache.put(Item{Key: key, Value: value, UpdatedAt: now.UTC()}, now)
			}
		}
		c.mu.Unlock()
		return c.loadSnapshot(ctx, "", false)
	}
	items, err := (remoteSource{client: c}).Read(ctx, "")
	if err != nil {
		return err
	}
	missing := maps.Clone(defaults)
	for _, item := range items {
		delete(missing, item.Key)
	}
	if len(missing) == 0 {
		return c.loadSnapshot(ctx, "", true)
	}
	path, err := c.environmentPath(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"defaults": defaults})
	if err != nil {
		return fmt.Errorf("encode defaults: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path+"/defaults", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build defaults request: %w", err)
	}
	req.Header.Set("Authorization", "ApiKey "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send defaults request: %w", err)
	}
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("close defaults response: %w", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("configcenter defaults endpoint: status %d (requires config:write and a server supporting initialization)", resp.StatusCode)
	}
	return c.loadSnapshot(ctx, "", true)
}
