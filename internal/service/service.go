package service

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/api"
	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/pihole"
	"github.com/lovelaze/nebula-sync/internal/sync"
	"github.com/lovelaze/nebula-sync/internal/sync/retry"
	"github.com/lovelaze/nebula-sync/internal/webhook"
	"github.com/lovelaze/nebula-sync/version"
)

const shutdownTimeout = 30 * time.Second

type Service struct {
	target    sync.Target
	conf      config.Config
	callbacks []sync.Callback
	State     *sync.State
	server    *api.Server
}

func NewService(target sync.Target, conf config.Config, callbacks ...sync.Callback) *Service {
	return &Service{
		target:    target,
		conf:      conf,
		callbacks: callbacks,
		State:     sync.NewState(),
	}
}

func Init() (*Service, error) {
	conf := config.Config{}
	if err := conf.Load(); err != nil {
		return nil, err
	}

	httpClient := conf.Client.NewHTTPClient()
	retry.Init(conf.Client)

	primary := pihole.NewClient(conf.Primary, httpClient)
	var replicas []pihole.Client
	for _, replica := range conf.Replicas {
		replicas = append(replicas, pihole.NewClient(replica, httpClient))
	}

	webhookClient := webhook.NewClient(conf.Sync.WebhookSettings)

	target := sync.NewTarget(primary, replicas)
	service := NewService(target, conf, webhookClient)

	if conf.API.Enabled {
		service.server = api.NewServer(service.State, conf.API.Addr)
	}

	return service, nil
}

func (service *Service) Run(ctx context.Context) error {
	log.Info().Msgf("Starting nebula-sync %s", version.Version)
	log.Debug().Str("config", service.conf.String()).Msgf("Settings")

	if service.server != nil {
		service.server.Start(ctx)
	}

	if err := service.sync(ctx, service.target); err != nil {
		return err
	}

	if service.conf.Sync.Cron != nil {
		return service.startCron(ctx, func() {
			if err := service.sync(ctx, service.target); err != nil {
				log.Error().Err(err).Msg("Sync failed")
			}
		})
	}

	return nil
}

func (service *Service) sync(ctx context.Context, t sync.Target) error {
	var report *sync.Report
	var err error

	if service.conf.Sync.FullSync {
		report, err = t.FullSync(ctx, service.conf.Sync)
	} else {
		report, err = t.SelectiveSync(ctx, service.conf.Sync)
	}

	service.State.Record(report, err)
	service.runCallbacks(err)

	if err == nil {
		log.Info().Msg("Sync completed")
	}

	return err
}

func (service *Service) runCallbacks(syncError error) {
	for _, callback := range service.callbacks {
		if syncError != nil {
			callback.OnFailure(syncError)
		} else {
			callback.OnSuccess()
		}
	}
}

func (service *Service) startCron(ctx context.Context, cmd func()) error {
	scheduler := cron.New()

	if _, err := scheduler.AddFunc(*service.conf.Sync.Cron, cmd); err != nil {
		return fmt.Errorf("cron job: %w", err)
	}

	scheduler.Start()
	<-ctx.Done()

	log.Info().Msg("Shutting down...")

	select {
	case <-scheduler.Stop().Done():
	case <-time.After(shutdownTimeout):
		log.Warn().Msg("Timed out waiting for the running sync to finish")
	}

	return nil
}
