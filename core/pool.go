package core

import (
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
