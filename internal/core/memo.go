package core

import (
	"sync"
	"sync/atomic"
)

// memo remembers the answers of a pure function of a short string, up to a fixed number of entries (item names come from the rules data,
// but a few come from what players type, so the table cannot grow without bound). Past the limit it answers without remembering.
type memo struct {
	m sync.Map
	n atomic.Int64
}

const memoLimit = 20000

func (c *memo) get(key string, compute func(string) string) string {
	if v, ok := c.m.Load(key); ok {
		return v.(string)
	}
	v := compute(key)
	if c.n.Load() < memoLimit {
		if _, loaded := c.m.LoadOrStore(key, v); !loaded {
			c.n.Add(1)
		}
	}
	return v
}
