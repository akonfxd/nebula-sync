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
	duration time.Duration
	err      error
}

type run struct {
	mode     string
	start    time.Time
	replicas []*replicaRun
}

func newRun(mode string, replicas []pihole.Client) *run {
	runs := make([]*replicaRun, 0, len(replicas))
	for _, replica := range replicas {
		runs = append(runs, &replicaRun{client: replica})
	}

	return &run{
		mode:     mode,
		start:    time.Now(),
		replicas: runs,
	}
}

func (r *run) forEachReplica(step string, fn func(replica pihole.Client) error) {
	for _, replica := range r.replicas {
		if replica.err != nil {
			continue
		}

		start := time.Now()
		err := fn(replica.client)
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
