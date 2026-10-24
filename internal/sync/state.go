package sync

import (
	"slices"
	"sync"
	"time"
)

const stackSize = 5

type State struct {
	mu    sync.RWMutex
	stack []Outcome
}

func NewState() *State {
	return &State{
		stack: []Outcome{},
	}
}

type Outcome struct {
	Timestamp time.Time
	Success   bool
}

func NewOutcome(success bool) *Outcome {
	return &Outcome{
		Timestamp: time.Now(),
		Success:   success,
	}
}

func (s *State) Add(outcome Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stack = append([]Outcome{outcome}, s.stack...)

	if len(s.stack) > stackSize {
		s.stack = s.stack[:stackSize]
	}
}

func (s *State) Outcomes() []Outcome {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return slices.Clone(s.stack)
}

func (s *State) Latest() (Outcome, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.stack) == 0 {
		return Outcome{}, false
	}

	return s.stack[0], true
}

func (s *State) OnSuccess() {
	s.Add(*NewOutcome(true))
}

func (s *State) OnFailure(err error) {
	s.Add(*NewOutcome(false))
}
