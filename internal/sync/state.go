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
	Mode      string
	Duration  time.Duration
	Err       error
	Replicas  []ReplicaReport
}

func NewOutcome(report *Report, err error) Outcome {
	outcome := Outcome{
		Timestamp: time.Now(),
		Success:   err == nil,
		Err:       err,
	}

	if report != nil {
		outcome.Mode = report.Mode
		outcome.Duration = report.Duration
		outcome.Replicas = slices.Clone(report.Replicas)
	}

	return outcome
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

func (s *State) Record(report *Report, err error) {
	s.Add(NewOutcome(report, err))
}
