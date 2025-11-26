package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/P4ST4S/go-load-balancer/core"
)

func TestLbHandler(t *testing.T) {
	// Reset serverPool for isolation
	serverPool = core.ServerPool{}

	t.Run("No Backends Available", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		lbHandler(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected 503 Service Unavailable, got %d", w.Code)
		}
	})

	t.Run("With Healthy Backend", func(t *testing.T) {
		// Mock a backend server
		backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Backend Response"))
		}))
		defer backendServer.Close()

		backendUrl, _ := url.Parse(backendServer.URL)
		proxy := httputil.NewSingleHostReverseProxy(backendUrl)

		b := &core.Backend{
			URL:          backendUrl,
			Alive:        true,
			ReverseProxy: proxy,
		}

		// Reset pool and add backend
		serverPool = core.ServerPool{}
		serverPool.AddBackend(b)

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		lbHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", w.Code)
		}
		if w.Body.String() != "Backend Response" {
			t.Errorf("Expected 'Backend Response', got '%s'", w.Body.String())
		}

		// Verify that connection counter was decremented back to 0
		if count := b.GetConnCount(); count != 0 {
			t.Errorf("Expected 0 connections after request completion, got %d", count)
		}
	})

	t.Run("Favicon Ignored", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/favicon.ico", nil)
		w := httptest.NewRecorder()

		lbHandler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected 404 for favicon, got %d", w.Code)
		}
	})
}

func TestStatsHandler(t *testing.T) {
	serverPool = core.ServerPool{}
	u, _ := url.Parse("http://localhost:8080")
	serverPool.AddBackend(&core.Backend{URL: u, Alive: true})

	req := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()

	statsHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected application/json, got %s", w.Header().Get("Content-Type"))
	}

	if !strings.Contains(w.Body.String(), "http://localhost:8080") {
		t.Errorf("Expected stats to contain backend URL")
	}
}

func TestWriteJSON_Error(t *testing.T) {
	w := httptest.NewRecorder()
	// Pass a channel to force json.Marshal error
	badData := make(chan int)

	writeJSON(w, badData)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500 Internal Server Error, got %d", w.Code)
	}
}

func TestIsBackendAlive(t *testing.T) {
	t.Run("Backend Alive", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		u, _ := url.Parse(server.URL)
		if !isBackendAlive(u) {
			t.Error("Expected backend to be detected as alive")
		}
	})

	t.Run("Backend Dead (Connection Refused)", func(t *testing.T) {
		// Use a port that is definitely closed or invalid host
		u, _ := url.Parse("http://localhost:59999")
		if isBackendAlive(u) {
			t.Error("Expected backend to be detected as dead")
		}
	})

	t.Run("Backend Error Status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		u, _ := url.Parse(server.URL)
		if isBackendAlive(u) {
			t.Error("Expected backend returning 500 to be considered dead")
		}
	})
}

func TestUpdateBackendStats(t *testing.T) {
	// Mock backend health endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"memory_usage": 5000}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	b := &core.Backend{URL: u, Alive: true}

	updateBackendStats(b)

	if mem := b.GetMemoryUsage(); mem != 5000 {
		t.Errorf("Expected memory usage 5000, got %d", mem)
	}
}

func TestUpdateBackendStats_Error(t *testing.T) {
	// Mock server that returns error
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	b := &core.Backend{URL: u, Alive: true}

	// Should not panic and should log error (we can't easily check log output here without capturing it,
	// but we can check that memory usage wasn't updated if we set it to something else first)
	b.SetMemoryUsage(100)
	updateBackendStats(b)

	if mem := b.GetMemoryUsage(); mem != 100 {
		t.Errorf("Expected memory usage to remain 100 on error, got %d", mem)
	}
}

func TestUpdateBackendStats_JSONError(t *testing.T) {
	// Mock server that returns invalid JSON
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid-json}`))
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	b := &core.Backend{URL: u, Alive: true}

	// Should log error and not update memory
	b.SetMemoryUsage(100)
	updateBackendStats(b)

	if mem := b.GetMemoryUsage(); mem != 100 {
		t.Errorf("Expected memory usage to remain 100 on JSON error, got %d", mem)
	}
}

func TestUpdateBackendStats_NetworkError(t *testing.T) {
	// Use a URL that will definitely fail (invalid port or non-existent host)
	u, _ := url.Parse("http://localhost:54321")
	b := &core.Backend{URL: u, Alive: true}

	// Should log error and return (not panic)
	updateBackendStats(b)
}

func TestHealthCheck(t *testing.T) {
	// Setup a backend
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	b := &core.Backend{URL: u, Alive: true}

	serverPool = core.ServerPool{}
	serverPool.AddBackend(b)

	// Run healthCheck with a very short interval and a context that cancels quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// This should run at least once or twice
	healthCheck(ctx, 10*time.Millisecond)

	// If it returns, it means context cancellation worked.
	// We can verify if IsAlive was called or updated, but since it's already alive, it stays alive.
	if !b.IsAlive() {
		t.Error("Backend should remain alive")
	}
}

func TestHealthCheck_StatusChange(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	// Backend marked as dead, but server is actually up
	b := &core.Backend{URL: u, Alive: false}

	serverPool = core.ServerPool{}
	serverPool.AddBackend(b)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	healthCheck(ctx, 10*time.Millisecond)

	if !b.IsAlive() {
		t.Error("Backend should have been marked as alive")
	}
}

func TestSetupServer(t *testing.T) {
	serverPool = core.ServerPool{}

	srv, err := setupServer("http://localhost:8081,http://localhost:8082", 3031)
	if err != nil {
		t.Fatalf("setupServer failed: %v", err)
	}

	if srv.Addr != ":3031" {
		t.Errorf("Expected port :3031, got %s", srv.Addr)
	}

	if len(serverPool.Backends) != 2 {
		t.Errorf("Expected 2 backends, got %d", len(serverPool.Backends))
	}
}

func TestSetupServer_Error(t *testing.T) {
	_, err := setupServer(":%^&", 3031) // Invalid URL
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestHealthCheck_PoolFull(t *testing.T) {
	// Mock updateBackendStatsFunc to block
	old := updateBackendStatsFunc
	defer func() { updateBackendStatsFunc = old }()

	// Block workers
	updateBackendStatsFunc = func(b *core.Backend) {
		time.Sleep(100 * time.Millisecond)
	}

	// Setup many backends to fill the channel
	// Channel size is len(Backends). Wait, if channel size = len(Backends), it never fills unless we send more than len?
	// The loop iterates over backends and sends them.
	// If channel size == number of backends, it can hold all of them.
	// So "Worker pool full" log only happens if we try to send MORE than capacity.
	// But we only send each backend once per tick.
	// So the channel size is exactly enough to hold one update per backend.
	// So it should NEVER be full unless a previous tick's updates are still pending?
	// Yes, if workers are slow, previous updates accumulate?
	// No, we iterate `for _, b := range serverPool.Backends`.
	// If channel is full, we skip.

	// So to trigger it:
	// 1. Tick 1: Send all backends. Workers are blocked. Channel fills up.
	// 2. Tick 2: Try to send again. Channel is full. Default case triggers.

	serverPool = core.ServerPool{}
	// Add 5 backends. Channel size 5. Workers 3.
	for i := 0; i < 5; i++ {
		u, _ := url.Parse("http://localhost:8080")
		serverPool.AddBackend(&core.Backend{URL: u, Alive: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Run with very short interval to trigger multiple ticks
	healthCheck(ctx, 10*time.Millisecond)

	// We can't easily assert that the log was printed, but this executes the code path.
}
