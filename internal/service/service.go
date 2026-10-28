package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/api"
	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/pihole"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
	"github.com/lovelaze/nebula-sync/internal/schedule"
	"github.com/lovelaze/nebula-sync/internal/store"
	"github.com/lovelaze/nebula-sync/internal/sync"
	"github.com/lovelaze/nebula-sync/internal/sync/retry"
	"github.com/lovelaze/nebula-sync/internal/webhook"
	"github.com/lovelaze/nebula-sync/version"
)

const shutdownTimeout = 30 * time.Second

// runtime holds everything a sync needs that the api is allowed to change while
// the process runs. It is replaced as a whole, so a sync always reads one
// consistent configuration.
type runtime struct {
	target    sync.Target
	plan      sync.Plan
	conf      config.Config
	callbacks []sync.Callback
}

// contextHolder carries the process context to the cron callback.
type contextHolder struct {
	ctx context.Context
}

type Service struct {
	State *sync.State

	current    atomic.Pointer[runtime]
	root       atomic.Pointer[contextHolder]
	running    atomic.Bool
	scheduler  *schedule.Runner
	store      *store.Store
	server     *api.Server
	httpClient *http.Client
}

func NewService(target sync.Target, plan sync.Plan, conf config.Config, callbacks ...sync.Callback) *Service {
	service := &Service{State: sync.NewState()}
	service.current.Store(&runtime{target: target, plan: plan, conf: conf, callbacks: callbacks})
	service.scheduler = schedule.NewRunner(service.scheduledSync)

	return service
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

	service, err := initService(ctx, configStore, conf)
	if err != nil {
		_ = configStore.Close()

		return nil, err
	}

	return service, nil
}

func initService(ctx context.Context, configStore *store.Store, conf config.Config) (*Service, error) {
	if err := seedFromEnv(ctx, configStore); err != nil {
		return nil, err
	}

	snapshot, err := applySnapshot(ctx, configStore, &conf)
	if err != nil {
		return nil, err
	}

	httpClient := conf.Client.NewHTTPClient()
	retry.Init(conf.Client)

	service := NewService(
		sync.NewTarget(pihole.NewClient(conf.Primary, httpClient)),
		newPlan(snapshot.Replicas, httpClient),
		conf,
		webhook.NewClient(conf.Sync.WebhookSettings),
	)

	service.store = configStore
	service.httpClient = httpClient

	if conf.API.Enabled {
		if err := ensureAdmin(ctx, configStore, conf.API); err != nil {
			return nil, err
		}

		service.server = api.NewServer(service.State, configStore, service, conf.API)
	}

	return service, nil
}

// ensureAdmin creates the configured administrator on an empty database, and
// otherwise points the operator at the bootstrap endpoint.
func ensureAdmin(ctx context.Context, configStore *store.Store, conf *config.API) error {
	password, err := conf.AdminSecret()
	if err != nil {
		return err
	}

	if conf.AdminUsername != "" && password != "" {
		created, err := configStore.EnsureAdmin(ctx, conf.AdminUsername, password)
		if err != nil {
			return err
		}

		if created {
			log.Info().Str("user", conf.AdminUsername).Msg("Created the configured administrator")
		}

		return nil
	}

	count, err := configStore.UserCount(ctx)
	if err != nil {
		return err
	}

	if count == 0 {
		log.Warn().Msg("No accounts exist, create the first one with POST /api/v1/auth/bootstrap")
	}

	return nil
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

// seedFromEnv fills an empty database from the environment, once.
func seedFromEnv(ctx context.Context, configStore *store.Store) error {
	seeded, err := configStore.Seeded(ctx)
	if err != nil {
		return err
	}

	if seeded {
		if ignored := config.IgnoredSyncEnvVars(); len(ignored) > 0 {
			log.Warn().
				Strs("vars", ignored).
				Msg("Stored configuration is authoritative, these environment variables are ignored")
		}

		return nil
	}

	envConf := config.Config{}
	if err := envConf.Load(); err != nil {
		return fmt.Errorf("no stored configuration and environment is incomplete: %w", err)
	}

	if err := configStore.SeedFromEnv(ctx, &envConf); err != nil {
		return err
	}

	log.Info().Msg("Seeded configuration from environment")

	return nil
}

// applySnapshot copies the stored configuration into conf and returns it.
func applySnapshot(ctx context.Context, configStore *store.Store, conf *config.Config) (*store.Snapshot, error) {
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
	service.scheduler.Stop(0)

	if service.store == nil {
		return nil
	}

	return service.store.Close()
}

func (service *Service) Run(ctx context.Context) error {
	current := service.current.Load()

	log.Info().Msgf("Starting nebula-sync %s", version.Version)
	log.Debug().Str("config", current.conf.String()).Msgf("Settings")

	service.root.Store(&contextHolder{ctx: ctx})

	if service.server != nil {
		service.server.Start(ctx)
	}

	syncErr := service.syncOnce(ctx, store.SourceStartup)

	if err := service.applySchedule(current.conf.Sync.Cron); err != nil {
		return err
	}

	// Without the console there is nobody to fix a broken configuration, so a
	// failed first sync still ends the process as it always has.
	if syncErr != nil && service.server == nil {
		return syncErr
	}

	if syncErr != nil {
		log.Error().Err(syncErr).Msg("Initial sync failed, the api stays available")
	}

	if service.server == nil && service.scheduler.Spec() == "" {
		return nil
	}

	<-ctx.Done()

	log.Info().Msg("Shutting down...")
	service.scheduler.Stop(shutdownTimeout)

	return nil
}

// Reload rebuilds the sync plan, the webhook callbacks and the schedule from the
// stored configuration. It is what makes an api write take effect without a restart.
func (service *Service) Reload(ctx context.Context) error {
	if service.store == nil {
		return errors.New("no configuration store attached")
	}

	conf := service.current.Load().conf

	snapshot, err := applySnapshot(ctx, service.store, &conf)
	if err != nil {
		return err
	}

	service.current.Store(&runtime{
		target:    sync.NewTarget(pihole.NewClient(conf.Primary, service.httpClient)),
		plan:      newPlan(snapshot.Replicas, service.httpClient),
		conf:      conf,
		callbacks: []sync.Callback{webhook.NewClient(conf.Sync.WebhookSettings)},
	})

	log.Info().Int("replicas", len(snapshot.Replicas)).Msg("Reloaded stored configuration")

	return service.applySchedule(conf.Sync.Cron)
}

// Sync starts a sync in the background so a slow run does not hold the request open.
func (service *Service) Sync(ctx context.Context) error {
	if !service.running.CompareAndSwap(false, true) {
		return sync.ErrInProgress
	}

	runCtx := context.WithoutCancel(ctx)

	go func() {
		defer service.running.Store(false)

		if err := service.runSync(runCtx, store.SourceManual); err != nil {
			log.Error().Err(err).Msg("Requested sync failed")
		}
	}()

	return nil
}

func (service *Service) Syncing() bool {
	return service.running.Load()
}

func (service *Service) Schedule() string {
	return service.scheduler.Spec()
}

func (service *Service) NextRun() time.Time {
	return service.scheduler.Next()
}

func (service *Service) applySchedule(cron *string) error {
	spec := ""
	if cron != nil {
		spec = *cron
	}

	if err := service.scheduler.Set(spec); err != nil {
		return fmt.Errorf("apply schedule: %w", err)
	}

	return nil
}

func (service *Service) scheduledSync() {
	ctx := context.Background()
	if holder := service.root.Load(); holder != nil {
		ctx = holder.ctx
	}

	if err := service.syncOnce(ctx, store.SourceSchedule); err != nil {
		if errors.Is(err, sync.ErrInProgress) {
			log.Warn().Msg("Skipped the scheduled sync, one is already running")

			return
		}

		log.Error().Err(err).Msg("Scheduled sync failed")
	}
}

// syncOnce runs a sync and waits for it, refusing to overlap with another one.
func (service *Service) syncOnce(ctx context.Context, source string) error {
	if !service.running.CompareAndSwap(false, true) {
		return sync.ErrInProgress
	}

	defer service.running.Store(false)

	return service.runSync(ctx, source)
}

func (service *Service) runSync(ctx context.Context, source string) error {
	current := service.current.Load()
	started := time.Now()

	report, err := current.target.Sync(ctx, current.plan)

	service.State.Record(report, err)
	service.recordRun(ctx, source, started, report, err)
	runCallbacks(current.callbacks, err)

	if err == nil {
		log.Info().Msg("Sync completed")
	}

	return err
}

func (service *Service) recordRun(
	ctx context.Context,
	source string,
	started time.Time,
	report *sync.Report,
	syncErr error,
) {
	if service.store == nil {
		return
	}

	run := store.Run{
		StartedAt:  started,
		FinishedAt: time.Now(),
		Duration:   time.Since(started),
		Source:     source,
		Success:    syncErr == nil,
		Replicas:   []store.RunReplica{},
	}

	if syncErr != nil {
		run.Error = syncErr.Error()
	}

	if report != nil {
		run.Mode = report.Mode

		for _, replica := range report.Replicas {
			stored := store.RunReplica{
				Target:   replica.Target,
				Profile:  replica.Profile,
				Mode:     replica.Mode,
				Duration: replica.Duration,
			}

			if replica.Err != nil {
				stored.Error = replica.Err.Error()
			}

			run.Replicas = append(run.Replicas, stored)
		}
	}

	if _, err := service.store.RecordRun(context.WithoutCancel(ctx), run); err != nil {
		log.Warn().Err(err).Msg("Failed to record the sync run")
	}
}

func runCallbacks(callbacks []sync.Callback, syncError error) {
	for _, callback := range callbacks {
		if syncError != nil {
			callback.OnFailure(syncError)
		} else {
			callback.OnSuccess()
		}
	}
}
