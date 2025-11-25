package core

import (
	"net/http/httputil"
	"net/url"
	"sync"
)

// Backend is our unique server instance
// Note: Capitalized fields are used to be accessible from other packages
type Backend struct {
	URL          *url.URL
	Alive        bool
	Mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
}

// SetAlive is a thread-safe way to set the alive status of the backend
func (b *Backend) SetAlive(alive bool) {
	b.Mux.Lock()
	b.Alive = alive
	b.Mux.Unlock()
}

// IsAlive is a thread-safe way to read the alive status of the backend
func (b *Backend) IsAlive() (alive bool) {
	b.Mux.RLock()
	alive = b.Alive
	b.Mux.RUnlock()
	return
}
