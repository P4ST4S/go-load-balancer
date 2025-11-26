package core

import (
	"fmt"
	"sync/atomic"
)

type ServerPool struct {
	Backends []*Backend
	current  uint64
}

func (s *ServerPool) NextIndex() int {
	return int(atomic.AddUint64(&s.current, uint64(1)) % uint64(len(s.Backends)))
}

func (s *ServerPool) GetNextPeer() *Backend {
	next := s.NextIndex()
	l := len(s.Backends) + next // prevent infinite loop
	for i := next; i < l; i++ {
		idx := i % len(s.Backends)
		if s.Backends[idx].IsAlive() {
			if i != next {
				atomic.StoreUint64(&s.current, uint64(idx))
			}
			return s.Backends[idx]
		}
	}
	return nil
}

func (s *ServerPool) AddBackend(b *Backend) {
	s.Backends = append(s.Backends, b)
}

// formatSecondsToDuration converts seconds to a human-readable duration string
func formatSecondsToDuration(seconds uint64) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	return fmt.Sprintf("%02dh:%02dm:%02ds", hours, minutes, secs)
}

// GetUpTimeInSeconds returns the total uptime in seconds of all alive backends
func (s *ServerPool) GetUpTimeInSeconds() uint64 {
	var total uint64
	for _, b := range s.Backends {
		if b.IsAlive() {
			total += b.GetUpTimeInSeconds()
		}
	}
	return total
}

func (s *ServerPool) GetUpTime() string {
	var totalUpTime uint64
	var aliveCount uint64
	for _, b := range s.Backends {
		if b.IsAlive() {
			totalUpTime += b.GetUpTimeInSeconds()
			aliveCount++
		}
	}
	if aliveCount == 0 {
		return "0s"
	}
	averageUpTime := totalUpTime / aliveCount
	return formatSecondsToDuration(averageUpTime)
}

func (s *ServerPool) GetStats() []BackendStats {
	var stats []BackendStats
	for _, b := range s.Backends {
		stats = append(stats, BackendStats{
			URL:    b.URL.String(),
			Alive:  b.IsAlive(),
			UpTime: b.GetUpTime(),
		})
	}
	return stats
}
