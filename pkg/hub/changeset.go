package hub

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/neptaco/uniforge/pkg/ui"
)

// changesetCache caches version -> changeset lookups in memory
type changesetCache struct {
	mu      sync.RWMutex
	entries map[string]changesetCacheEntry
}

type changesetCacheEntry struct {
	changeset string
	timestamp time.Time
}

const changesetCacheExpiration = 24 * time.Hour

var csCache = &changesetCache{
	entries: make(map[string]changesetCacheEntry),
}

func (c *changesetCache) get(version string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if entry, ok := c.entries[version]; ok {
		if time.Since(entry.timestamp) < changesetCacheExpiration {
			return entry.changeset
		}
	}
	return ""
}

func (c *changesetCache) put(version, changeset string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[version] = changesetCacheEntry{
		changeset: changeset,
		timestamp: time.Now(),
	}
}

// ResolveChangeset fetches the changeset (short revision) for an exact Unity
// version from the Unity Release API, with in-memory caching
func ResolveChangeset(version string) (string, error) {
	if changeset := csCache.get(version); changeset != "" {
		ui.Debug("Using cached changeset", "version", version, "changeset", changeset)
		return changeset, nil
	}

	ui.Debug("Fetching changeset from Unity Release API", "version", version)

	// The version parameter matches exact versions as well as prefixes, so a
	// full version string yields at most one release
	params := url.Values{}
	params.Set("version", version)
	params.Set("limit", "1")

	page, err := defaultReleaseAPI.fetchPage(params)
	if err != nil {
		return "", err
	}

	// Verify the exact version to avoid caching a wrong changeset from a
	// prefix match
	for _, rel := range page.Results {
		if rel.Version == version && rel.ShortRevision != "" {
			csCache.put(version, rel.ShortRevision)
			ui.Debug("Found changeset", "version", version, "changeset", rel.ShortRevision)
			return rel.ShortRevision, nil
		}
	}

	return "", fmt.Errorf("changeset not found for version %s", version)
}
