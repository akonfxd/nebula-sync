package sync

import (
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/pihole"
)

type ReplicaReport struct {
	Target   string
	Profile  string
	Mode     string
	Duration time.Duration
	Err      error
}

type Report struct {
	Mode     string
	Start    time.Time
	Duration time.Duration
	Replicas []ReplicaReport
}

func (r *Report) Failed() int {
	failed := 0
	for _, replica := range r.Replicas {
		if replica.Err != nil {
			failed++
		}
	}

	return failed
}

func (r *Report) Succeeded() int {
	return len(r.Replicas) - r.Failed()
}

func (r *Report) Err() error {
	errs := make([]error, 0, len(r.Replicas))
	for _, replica := range r.Replicas {
		if replica.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", replica.Target, replica.Err))
		}
	}

	return errors.Join(errs...)
}

type replicaRun struct {
	client   pihole.Client
	settings *Settings
	duration time.Duration
	err      error
}

type run struct {
	mode     string
	start    time.Time
	replicas []*replicaRun
}

func newRun(plan Plan) *run {
	runs := make([]*replicaRun, 0, len(plan.Replicas))
	for _, replica := range plan.Replicas {
		settings := replica.Settings
		if settings == nil {
			settings = NewSettings(nil)
		}

		runs = append(runs, &replicaRun{client: replica.Client, settings: settings})
	}

	return &run{
		mode:     plan.mode(),
		start:    time.Now(),
		replicas: runs,
	}
}

func (r *run) runGravity() bool {
	for _, replica := range r.replicas {
		if replica.err == nil && replica.settings.RunGravity {
			return true
		}
	}

	return false
}

func (r *run) forEachReplica(step string, fn func(replica pihole.Client, settings *Settings) error) {
	for _, replica := range r.replicas {
		if replica.err != nil {
			continue
		}

		start := time.Now()
		err := fn(replica.client, replica.settings)
		replica.duration += time.Since(start)

		if err != nil {
			replica.err = fmt.Errorf("%s: %w", step, err)
			log.Error().
				Err(err).
				Str("target", replica.client.String()).
				Str("step", step).
				Msg("Replica failed, continuing with the remaining replicas")
		}
	}
}

func (r *run) report() *Report {
	replicas := make([]ReplicaReport, 0, len(r.replicas))
	for _, replica := range r.replicas {
		replicas = append(replicas, ReplicaReport{
			Target:   replica.client.String(),
			Profile:  replica.settings.Profile,
			Mode:     replica.settings.Mode,
			Duration: replica.duration,
			Err:      replica.err,
		})
	}

	return &Report{
		Mode:     r.mode,
		Start:    r.start,
		Duration: time.Since(r.start),
		Replicas: replicas,
	}
}
