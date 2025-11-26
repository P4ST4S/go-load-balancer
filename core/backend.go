package core

import (
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// Backend is our unique server instance
// Note: Capitalized fields are used to be accessible from other packages
type Backend struct {
	URL          *url.URL
	Alive        bool
	Mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
	StartTime    time.Time
}

// SetAlive is a thread-safe way to set the alive status of the backend
func (b *Backend) SetAlive(alive bool) {
	b.Mux.Lock()
	defer b.Mux.Unlock()

	// If we are transitioning to alive, reset the timer
	if alive && !b.Alive {
		b.StartTime = time.Now()
	}
	b.Alive = alive
}

// IsAlive is a thread-safe way to read the alive status of the backend
func (b *Backend) IsAlive() (alive bool) {
	b.Mux.RLock()
	alive = b.Alive
	b.Mux.RUnlock()
	return
}

// GetUpTime returns the uptime in a human-readable format
func (b *Backend) GetUpTime() string {
	return formatSecondsToDuration(b.GetUpTimeInSeconds())
}

// GetUpTimeInSeconds returns the uptime in seconds
func (b *Backend) GetUpTimeInSeconds() uint64 {
	b.Mux.RLock()
	defer b.Mux.RUnlock()
	if !b.Alive {
		return 0
	}
	return uint64(time.Since(b.StartTime).Seconds())
}

// BackendStats represents the statistics of a backend server
type BackendStats struct {
	URL    string `json:"url"`
	Alive  bool   `json:"alive"`
	UpTime string `json:"uptime"`
}
