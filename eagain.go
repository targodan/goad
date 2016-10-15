package goad

import "sync"

type eagainSychronizer struct {
	wasSignaled bool
	cond        *sync.Cond
}

func newEagainSynchronizer() *eagainSychronizer {
	return &eagainSychronizer{
		wasSignaled: false,
		cond:        sync.NewCond(new(sync.Mutex)),
	}
}

func (s *eagainSychronizer) Signal() {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	if !s.wasSignaled {
		s.wasSignaled = true
		s.cond.Signal()
	}
}

func (s *eagainSychronizer) Wait() {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	s.wasSignaled = false
	for !s.wasSignaled {
		s.cond.Wait()
	}
}
