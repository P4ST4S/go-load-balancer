package core

import (
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestBackend_SetAlive(t *testing.T) {
	u, _ := url.Parse("http://localhost:8080")
	b := &Backend{URL: u, Alive: false}

	tests := []struct {
		name     string
		alive    bool
		expected bool
	}{
		{"SetAlive True", true, true},
		{"SetAlive False", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b.SetAlive(tt.alive)
			if got := b.IsAlive(); got != tt.expected {
				t.Errorf("Backend.IsAlive() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBackend_ConcurrentAccess(t *testing.T) {
	u, _ := url.Parse("http://localhost:8080")
	b := &Backend{URL: u, Alive: false}

	t.Run("Concurrent SetAlive/IsAlive", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				b.SetAlive(true)
				_ = b.IsAlive()
			}()
		}
		wg.Wait()
	})
}

func TestBackend_ConnectionCounters(t *testing.T) {
	u, _ := url.Parse("http://localhost:8080")
	b := &Backend{URL: u}

	t.Run("Increment and Decrement", func(t *testing.T) {
		b.IncConn()
		if count := b.GetConnCount(); count != 1 {
			t.Errorf("Expected 1 connection, got %d", count)
		}

		b.DecConn()
		if count := b.GetConnCount(); count != 0 {
			t.Errorf("Expected 0 connections, got %d", count)
		}
	})

	t.Run("Underflow Protection", func(t *testing.T) {
		b.DecConn() // Should not go below 0
		if count := b.GetConnCount(); count != 0 {
			t.Errorf("Expected 0 connections (underflow protection), got %d", count)
		}
	})

	t.Run("Concurrent Counters", func(t *testing.T) {
		var wg sync.WaitGroup
		iterations := 1000

		// Increment concurrently
		for i := 0; i < iterations; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				b.IncConn()
			}()
		}
		wg.Wait()

		if count := b.GetConnCount(); count != uint64(iterations) {
			t.Errorf("Expected %d connections, got %d", iterations, count)
		}

		// Decrement concurrently
		for i := 0; i < iterations; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				b.DecConn()
			}()
		}
		wg.Wait()

		if count := b.GetConnCount(); count != 0 {
			t.Errorf("Expected 0 connections, got %d", count)
		}
	})
}

func TestBackend_Uptime(t *testing.T) {
	u, _ := url.Parse("http://localhost:8080")
	b := &Backend{URL: u, Alive: false}

	t.Run("Uptime for Dead Backend", func(t *testing.T) {
		if uptime := b.GetUpTimeInSeconds(); uptime != 0 {
			t.Errorf("Expected 0 uptime for dead backend, got %d", uptime)
		}
	})

	t.Run("Uptime for Alive Backend", func(t *testing.T) {
		b.SetAlive(true)
		// Manually set StartTime to the past to avoid sleeping
		b.StartTime = time.Now().Add(-2 * time.Second)
		if uptime := b.GetUpTimeInSeconds(); uptime == 0 {
			t.Error("Expected > 0 uptime for alive backend")
		}
	})
}

func TestBackend_MemoryUsage(t *testing.T) {
	u, _ := url.Parse("http://localhost:8080")
	b := &Backend{URL: u}

	t.Run("Set and Get", func(t *testing.T) {
		b.SetMemoryUsage(1024)
		if mem := b.GetMemoryUsage(); mem != 1024 {
			t.Errorf("Expected 1024, got %d", mem)
		}
	})

	t.Run("Formatting", func(t *testing.T) {
		tests := []struct {
			bytes    uint64
			expected string
		}{
			{500, "500 B"},
			{1024, "1.00 KB"},
			{1536, "1.50 KB"},
			{1048576, "1.00 MB"},
			{2097152, "2.00 MB"},
		}

		for _, tt := range tests {
			b.SetMemoryUsage(tt.bytes)
			if got := b.GetMemoryUsageString(); got != tt.expected {
				t.Errorf("For %d bytes, expected %s, got %s", tt.bytes, tt.expected, got)
			}
		}
	})
}
