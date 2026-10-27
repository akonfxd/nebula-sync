package sync

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_Add(t *testing.T) {
	s := NewState()
	require.Empty(t, s.Outcomes())

	outcome := NewOutcome(nil, nil)
	s.Add(outcome)

	assert.Len(t, s.Outcomes(), 1)
	assert.Contains(t, s.Outcomes(), outcome)
}

func TestState_Record_success(t *testing.T) {
	s := NewState()
	require.Empty(t, s.Outcomes())

	s.Record(nil, nil)

	assert.Len(t, s.Outcomes(), 1)
	assert.True(t, s.Outcomes()[0].Success)
	now := time.Now()
	assert.WithinRange(t, s.Outcomes()[0].Timestamp, now.Add(-1*time.Second), now.Add(1*time.Second))
}

func TestState_Record_failure(t *testing.T) {
	s := NewState()
	require.Empty(t, s.Outcomes())

	syncErr := errors.New("test error")
	s.Record(nil, syncErr)

	assert.Len(t, s.Outcomes(), 1)
	assert.False(t, s.Outcomes()[0].Success)
	require.ErrorIs(t, s.Outcomes()[0].Err, syncErr)
	now := time.Now()
	assert.WithinRange(t, s.Outcomes()[0].Timestamp, now.Add(-1*time.Second), now.Add(1*time.Second))
}

func TestState_ConcurrentAccess(t *testing.T) {
	s := NewState()

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			s.Record(nil, nil)
		})
		wg.Go(func() {
			s.Latest()
			s.Outcomes()
		})
	}
	wg.Wait()

	assert.Len(t, s.Outcomes(), stackSize)
}

func TestState_Latest_Empty(t *testing.T) {
	s := NewState()

	_, ok := s.Latest()

	assert.False(t, ok)
}

func TestState_Outcomes_IsACopy(t *testing.T) {
	s := NewState()
	s.Record(nil, nil)

	outcomes := s.Outcomes()
	outcomes[0].Success = false

	latest, ok := s.Latest()
	require.True(t, ok)
	assert.True(t, latest.Success)
}

func TestState_Record_carriesReport(t *testing.T) {
	s := NewState()

	report := &Report{
		Mode:     "full",
		Duration: time.Second,
		Replicas: []ReplicaReport{{Target: "http://replica", Duration: time.Millisecond}},
	}
	s.Record(report, nil)

	latest, ok := s.Latest()
	require.True(t, ok)
	assert.Equal(t, "full", latest.Mode)
	assert.Equal(t, time.Second, latest.Duration)
	require.Len(t, latest.Replicas, 1)
	assert.Equal(t, "http://replica", latest.Replicas[0].Target)

	report.Replicas[0].Target = "mutated"
	latest, ok = s.Latest()
	require.True(t, ok)
	assert.Equal(t, "http://replica", latest.Replicas[0].Target)
}
