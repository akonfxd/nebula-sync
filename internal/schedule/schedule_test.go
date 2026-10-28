package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	require.NoError(t, Validate(""))
	require.NoError(t, Validate("0 * * * *"))
	require.NoError(t, Validate("@every 1h"))
	require.Error(t, Validate("not a cron expression"))
	require.Error(t, Validate("0 * * *"))
}

func TestRunner_setAndClear(t *testing.T) {
	runner := NewRunner(func() {})
	t.Cleanup(func() { runner.Stop(0) })

	assert.Empty(t, runner.Spec())
	assert.True(t, runner.Next().IsZero())

	require.NoError(t, runner.Set("0 * * * *"))
	assert.Equal(t, "0 * * * *", runner.Spec())
	assert.False(t, runner.Next().IsZero())

	require.NoError(t, runner.Set("@every 2h"))
	assert.Equal(t, "@every 2h", runner.Spec())

	require.NoError(t, runner.Set(""))
	assert.Empty(t, runner.Spec())
	assert.True(t, runner.Next().IsZero())
}

func TestRunner_keepsTheCurrentScheduleWhenTheNewOneIsInvalid(t *testing.T) {
	runner := NewRunner(func() {})
	t.Cleanup(func() { runner.Stop(0) })

	require.NoError(t, runner.Set("0 * * * *"))
	require.Error(t, runner.Set("every other tuesday"))

	assert.Equal(t, "0 * * * *", runner.Spec())
}

func TestRunner_runsTheCommand(t *testing.T) {
	fired := make(chan struct{}, 1)

	runner := NewRunner(func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	})
	t.Cleanup(func() { runner.Stop(0) })

	require.NoError(t, runner.Set("@every 1s"))

	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("the scheduled command never ran")
	}
}
