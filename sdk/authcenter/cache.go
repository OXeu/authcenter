package authcenter

import (
	"strings"
	"time"
)

// DefaultCacheTTL limits how long a successful read or write stays in memory.
const DefaultCacheTTL = 30 * time.Second

type cacheEntry struct {
	item      Item
	expiresAt time.Time
}

type memoryCache struct {
	entries  map[string]cacheEntry
	prefixes map[string]time.Time
	ttl      time.Duration
}

func (c *memoryCache) prune(now time.Time) {
	for key, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	for prefix, expiresAt := range c.prefixes {
		if !now.Before(expiresAt) {
			delete(c.prefixes, prefix)
		}
	}
}

func (c *memoryCache) covers(prefix string) bool {
	for cachedPrefix := range c.prefixes {
		if strings.HasPrefix(prefix, cachedPrefix) {
			return true
		}
	}
	return false
}

func (c *memoryCache) get(key string, now time.Time) (Item, bool) {
	entry, ok := c.entries[key]
	return entry.item, ok && now.Before(entry.expiresAt)
}

func (c *memoryCache) put(item Item, now time.Time) {
	c.entries[item.Key] = cacheEntry{item: item, expiresAt: now.Add(c.ttl)}
}

func (c *memoryCache) store(prefix string, items []Item, now time.Time, refresh bool) {
	c.prune(now)
	if refresh {
		for key := range c.entries {
			if strings.HasPrefix(key, prefix) {
				delete(c.entries, key)
			}
		}
	}
	for _, item := range items {
		if _, exists := c.entries[item.Key]; !exists {
			c.put(item, now)
		}
	}
	expiresAt := now.Add(c.ttl)
	for key, entry := range c.entries {
		if strings.HasPrefix(key, prefix) && entry.expiresAt.Before(expiresAt) {
			expiresAt = entry.expiresAt
		}
	}
	for cachedPrefix := range c.prefixes {
		if strings.HasPrefix(cachedPrefix, prefix) {
			delete(c.prefixes, cachedPrefix)
		}
	}
	c.prefixes[prefix] = expiresAt
}
