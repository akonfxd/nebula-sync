package schedule

import (
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

// Runner owns the cron scheduler so the schedule can be replaced while the process runs.
type Runner struct {
	cmd func()

	mu        sync.Mutex
	spec      string
	scheduler *cron.Cron
}

func NewRunner(cmd func()) *Runner {
	return &Runner{cmd: cmd}
}

// Validate reports whether a cron expression is usable. An empty expression means "no schedule".
func Validate(spec string) error {
	if spec == "" {
		return nil
	}

	if _, err := cron.ParseStandard(spec); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	return nil
}

// Set installs a schedule, replacing any current one. An empty expression stops scheduling.
func (r *Runner) Set(spec string) error {
	if err := Validate(spec); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if spec == r.spec && (spec == "") == (r.scheduler == nil) {
		return nil
	}

	r.stopLocked(0)
	r.spec = spec

	if spec == "" {
		log.Info().Msg("Sync schedule cleared")

		return nil
	}

	scheduler := cron.New()
	if _, err := scheduler.AddFunc(spec, r.cmd); err != nil {
		return fmt.Errorf("cron job: %w", err)
	}

	scheduler.Start()
	r.scheduler = scheduler

	log.Info().Str("cron", spec).Msg("Sync schedule installed")

	return nil
}

// Spec returns the active cron expression, empty when nothing is scheduled.
func (r *Runner) Spec() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.scheduler == nil {
		return ""
	}

	return r.spec
}

// Next reports when the next run is due, or the zero time when nothing is scheduled.
func (r *Runner) Next() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.scheduler == nil {
		return time.Time{}
	}

	entries := r.scheduler.Entries()
	if len(entries) == 0 {
		return time.Time{}
	}

	return entries[0].Next
}

// Stop ends scheduling and waits up to timeout for a running job to finish.
func (r *Runner) Stop(timeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stopLocked(timeout)
	r.spec = ""
}

func (r *Runner) stopLocked(timeout time.Duration) {
	if r.scheduler == nil {
		return
	}

	stopped := r.scheduler.Stop().Done()
	r.scheduler = nil

	if timeout <= 0 {
		return
	}

	select {
	case <-stopped:
	case <-time.After(timeout):
		log.Warn().Msg("Timed out waiting for the running sync to finish")
	}
}
