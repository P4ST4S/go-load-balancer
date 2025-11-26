package core

import (
	"net/url"
	"testing"
	"time"
)

func TestServerPool_AddBackend(t *testing.T) {
	pool := &ServerPool{}
	u, _ := url.Parse("http://localhost:8080")
	b := &Backend{URL: u}

	pool.AddBackend(b)

	if len(pool.Backends) != 1 {
		t.Errorf("Expected 1 backend, got %d", len(pool.Backends))
	}
}

func TestServerPool_GetNextPeer(t *testing.T) {
	pool := &ServerPool{}
	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")
	u3, _ := url.Parse("http://localhost:8083")

	b1 := &Backend{URL: u1, Alive: true}
	b2 := &Backend{URL: u2, Alive: true}
	b3 := &Backend{URL: u3, Alive: false} // Dead backend

	pool.AddBackend(b1)
	pool.AddBackend(b2)
	pool.AddBackend(b3)

	tests := []struct {
		name     string
		expected *Backend
	}{
		{"First Call (RR)", b2},  // Implementation detail: NextIndex increments first, so 0->1
		{"Second Call (RR)", b1}, // 1->2 (dead) -> 0 (alive)
		{"Third Call (RR)", b2},  // 0->1
	}

	// Note: The exact sequence depends on initial state of 'current'.
	// current is 0. NextIndex() -> 1. Returns b2.
	// NextIndex() -> 2. b3 dead. Loop to 0. Returns b1.
	// NextIndex() -> 1. Returns b2.

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pool.GetNextPeer()
			if got != tt.expected {
				t.Errorf("GetNextPeer() = %v, want %v", got.URL, tt.expected.URL)
			}
		})
	}

	t.Run("All Backends Dead", func(t *testing.T) {
		b1.SetAlive(false)
		b2.SetAlive(false)
		b3.SetAlive(false)

		got := pool.GetNextPeer()
		if got != nil {
			t.Errorf("Expected nil when all backends are dead, got %v", got.URL)
		}
	})
}

func TestServerPool_GetLeastConnPeer(t *testing.T) {
	pool := &ServerPool{}
	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")
	u3, _ := url.Parse("http://localhost:8083")

	b1 := &Backend{URL: u1, Alive: true}
	b2 := &Backend{URL: u2, Alive: true}
	b3 := &Backend{URL: u3, Alive: true}

	pool.AddBackend(b1)
	pool.AddBackend(b2)
	pool.AddBackend(b3)

	// Scenario 1: Different connections
	b1.ConnCount = 10
	b2.ConnCount = 5
	b3.ConnCount = 20

	t.Run("Pick Least Connections", func(t *testing.T) {
		peer := pool.GetLeastConnPeer()
		if peer != b2 {
			t.Errorf("Expected b2 (5 conns), got %v (%d conns)", peer.URL, peer.GetConnCount())
		}
	})

	// Scenario 2: All busy
	b2.ConnCount = 100
	t.Run("Pick New Least", func(t *testing.T) {
		peer := pool.GetLeastConnPeer()
		if peer != b1 { // b1 has 10, b3 has 20
			t.Errorf("Expected b1 (10 conns), got %v (%d conns)", peer.URL, peer.GetConnCount())
		}
	})

	// Scenario 3: Ignore dead backends
	b1.SetAlive(false)
	t.Run("Ignore Dead Backend", func(t *testing.T) {
		peer := pool.GetLeastConnPeer()
		if peer != b3 { // b1 dead, b2=100, b3=20
			t.Errorf("Expected b3 (20 conns), got %v (%d conns)", peer.URL, peer.GetConnCount())
		}
	})
}

func TestServerPool_GetStats(t *testing.T) {
	pool := &ServerPool{}
	u, _ := url.Parse("http://localhost:8080")
	b := &Backend{URL: u, Alive: true}
	pool.AddBackend(b)

	stats := pool.GetStats()
	if len(stats) != 1 {
		t.Errorf("Expected 1 stat entry, got %d", len(stats))
	}
	if stats[0].URL != "http://localhost:8080" {
		t.Errorf("Expected URL http://localhost:8080, got %s", stats[0].URL)
	}
}

func TestServerPool_Uptime(t *testing.T) {
	pool := &ServerPool{}
	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")

	// Simulate backends running for 10 seconds
	startTime := time.Now().Add(-10 * time.Second)
	b1 := &Backend{URL: u1, Alive: true, StartTime: startTime}
	b2 := &Backend{URL: u2, Alive: true, StartTime: startTime}

	pool.AddBackend(b1)
	pool.AddBackend(b2)

	t.Run("GetUpTimeInSeconds", func(t *testing.T) {
		// Total uptime should be around 20 seconds (10s + 10s)
		total := pool.GetUpTimeInSeconds()
		if total < 20 || total > 22 {
			t.Errorf("Expected total uptime around 20s, got %d", total)
		}
	})

	t.Run("GetUpTime Average", func(t *testing.T) {
		// Average should be around 10s
		avg := pool.GetUpTime()
		// formatSecondsToDuration output for 10s is "00h:00m:10s"
		if avg != "00h:00m:10s" && avg != "00h:00m:11s" {
			t.Errorf("Expected average uptime around 10s, got %s", avg)
		}
	})

	t.Run("GetUpTime No Alive Backends", func(t *testing.T) {
		b1.SetAlive(false)
		b2.SetAlive(false)
		avg := pool.GetUpTime()
		if avg != "0s" {
			t.Errorf("Expected 0s for no alive backends, got %s", avg)
		}
	})
}
