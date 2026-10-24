package videoai

import (
	"sync"
	"time"
)

// tickCache holds values that are read on every analysis tick but change only
// when an operator edits configuration.
//
// A tick at 2 FPS across 16 cameras is 32 ticks/s; re-reading the stream URL,
// the boundary geometry and the model version — and re-parsing their JSON — on
// each one makes SQLite the bottleneck of the analysis loop. Everything here is
// either immutable once published (a published boundary version, a published
// model version) or slow-moving (a stream URL), so a short TTL plus explicit
// invalidation from the handlers that mutate them is safe: the worst case is a
// few seconds of stale configuration, never stale detections.
type tickCache struct {
	ttl   time.Duration
	mu    sync.RWMutex
	items map[string]cached
}

type cached struct {
	value any
	until time.Time
}

func newTickCache(ttl time.Duration) *tickCache {
	return &tickCache{ttl: ttl, items: map[string]cached{}}
}

func (c *tickCache) get(key string) (any, bool) {
	c.mu.RLock()
	it, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(it.until) {
		return nil, false
	}
	return it.value, true
}

// set stores a value; expiry is refreshed on every store so a hot key stays
// warm instead of expiring mid-burst.
func (c *tickCache) set(key string, v any) {
	c.mu.Lock()
	c.items[key] = cached{value: v, until: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

// drop removes one key, and dropAll clears everything. Handlers call these
// when the underlying row changes, so a re-published model takes effect on the
// next tick instead of up to a TTL later.
func (c *tickCache) drop(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

func (c *tickCache) dropAll() {
	c.mu.Lock()
	c.items = map[string]cached{}
	c.mu.Unlock()
}

// ------------------------------------------------------------------ keys ----

func (m *Manager) streamURLCache() *tickCache { return m.cacheStream }
func (m *Manager) modelCache() *tickCache     { return m.cacheModel }
func (m *Manager) geometryCache() *tickCache  { return m.cacheGeom }

// InvalidateModel drops the cached resolution for a template; callers invoke it
// after publishing, rebinding or editing a model version.
func (m *Manager) InvalidateModel(templateID uint) {
	if m == nil || m.cacheModel == nil {
		return
	}
	m.cacheModel.drop(cacheKeyModel(templateID))
}

// InvalidateBoundary drops the cached geometry of a boundary version; callers
// invoke it after publishing or editing one.
func (m *Manager) InvalidateBoundary(versionID uint) {
	if m == nil || m.cacheGeom == nil {
		return
	}
	m.cacheGeom.drop(cacheKeyGeom(versionID))
}

// InvalidateStream drops the cached stream URL of a camera; callers invoke it
// after editing a camera or its stream profiles.
func (m *Manager) InvalidateStream(cameraID uint) {
	if m == nil || m.cacheStream == nil {
		return
	}
	m.cacheStream.drop(cacheKeyStream(cameraID))
}

func cacheKeyModel(templateID uint) string { return "model:" + utoa(templateID) }
func cacheKeyGeom(versionID uint) string   { return "geom:" + utoa(versionID) }
func cacheKeyStream(cameraID uint) string  { return "stream:" + utoa(cameraID) }

func utoa(v uint) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
