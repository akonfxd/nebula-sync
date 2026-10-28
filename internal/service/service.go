package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/api"
	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/pihole"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
	"github.com/lovelaze/nebula-sync/internal/store"
	"github.com/lovelaze/nebula-sync/internal/sync"
	"github.com/lovelaze/nebula-sync/internal/sync/retry"
	"github.com/lovelaze/nebula-sync/internal/webhook"
	"github.com/lovelaze/nebula-sync/version"
)

const shutdownTimeout = 30 * time.Second

type Service struct {
	target    sync.Target
	plan      sync.Plan
	conf      config.Config
	callbacks []sync.Callback
	State     *sync.State
	server    *api.Server
	store     *store.Store
}

func NewService(target sync.Target, plan sync.Plan, conf config.Config, callbacks ...sync.Callback) *Service {
	return &Service{
		target:    target,
		plan:      plan,
		conf:      conf,
		callbacks: callbacks,
		State:     sync.NewState(),
	}
}

func Init(ctx context.Context) (*Service, error) {
	conf := config.Config{}
	if err := conf.LoadRuntime(); err != nil {
		return nil, err
	}

	configStore, err := store.Open(ctx, conf.Store)
	if err != nil {
		return nil, err
	}

	snapshot, err := loadStoredConfig(ctx, configStore, &conf)
	if err != nil {
		_ = configStore.Close()
		return nil, err
	}

	httpClient := conf.Client.NewHTTPClient()
	retry.Init(conf.Client)

	primary := pihole.NewClient(conf.Primary, httpClient)
	plan := newPlan(snapshot.Replicas, httpClient)

	webhookClient := webhook.NewClient(conf.Sync.WebhookSettings)

	target := sync.NewTarget(primary)
	service := NewService(target, plan, conf, webhookClient)

	if conf.API.Enabled {
		service.server = api.NewServer(service.State, conf.API.Addr)
	}

	service.store = configStore

	return service, nil
}

func newPlan(replicas []store.Replica, httpClient *http.Client) sync.Plan {
	plan := sync.Plan{Replicas: make([]sync.ReplicaPlan, 0, len(replicas))}

	for _, replica := range replicas {
		settings := sync.NewSettings(replica.Profile)
		client := pihole.NewClient(replica.PiHole, httpClient)

		log.Debug().
			Str("target", client.String()).
			Str("profile", settings.Profile).
			Str("mode", settings.Mode).
			Msg("Replica configuration")

		plan.Replicas = append(plan.Replicas, sync.ReplicaPlan{Client: client, Settings: settings})
	}

	return plan
}

func loadStoredConfig(ctx context.Context, configStore *store.Store, conf *config.Config) (*store.Snapshot, error) {
	seeded, err := configStore.Seeded(ctx)
	if err != nil {
		return nil, err
	}

	if seeded {
		if ignored := config.IgnoredSyncEnvVars(); len(ignored) > 0 {
			log.Warn().
				Strs("vars", ignored).
				Msg("Stored configuration is authoritative, these environment variables are ignored")
		}
	} else {
		envConf := config.Config{}
		if err := envConf.Load(); err != nil {
			return nil, fmt.Errorf("no stored configuration and environment is incomplete: %w", err)
		}

		if err := configStore.SeedFromEnv(ctx, &envConf); err != nil {
			return nil, err
		}

		log.Info().Msg("Seeded configuration from environment")
	}

	snapshot, err := configStore.LoadSnapshot(ctx)
	if err != nil {
		return nil, err
	}

	conf.Primary = snapshot.Primary
	conf.Sync = snapshot.Sync

	conf.Replicas = make([]model.PiHole, 0, len(snapshot.Replicas))
	for _, replica := range snapshot.Replicas {
		conf.Replicas = append(conf.Replicas, replica.PiHole)
	}

	return snapshot, nil
}

func (service *Service) Close() error {
	return service.store.Close()
}

func (service *Service) Run(ctx context.Context) error {
	log.Info().Msgf("Starting nebula-sync %s", version.Version)
	log.Debug().Str("config", service.conf.String()).Msgf("Settings")

	if service.server != nil {
		service.server.Start(ctx)
	}

	if err := service.sync(ctx); err != nil {
		return err
	}

	if service.conf.Sync.Cron != nil {
		return service.startCron(ctx, func() {
			if err := service.sync(ctx); err != nil {
				log.Error().Err(err).Msg("Sync failed")
			}
		})
	}

	return nil
}

func (service *Service) sync(ctx context.Context) error {
	report, err := service.target.Sync(ctx, service.plan)

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
