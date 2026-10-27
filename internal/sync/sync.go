package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/pihole"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
	"github.com/lovelaze/nebula-sync/internal/sync/filter"
	"github.com/lovelaze/nebula-sync/internal/sync/retry"
)

const cleanupTimeout = 30 * time.Second

type Target interface {
	FullSync(ctx context.Context, sync *config.Sync) (*Report, error)
	SelectiveSync(ctx context.Context, sync *config.Sync) (*Report, error)
}

type target struct {
	Primary  pihole.Client
	Replicas []pihole.Client
	Client   *config.Client
}

func NewTarget(primary pihole.Client, replicas []pihole.Client) Target {
	return &target{
		Primary:  primary,
		Replicas: replicas,
	}
}

func (target *target) sync(ctx context.Context, syncFunc func(*run) error, mode string) (*Report, error) {
	var err error
	currentRun := newRun(mode, target.Replicas)
	log.Info().Str("mode", mode).Int("replicas", len(target.Replicas)).Msg("Running sync")

	defer func() {
		if err != nil {
			log.Error().Err(err).Msg("Error during sync")
		}

		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancel()

		target.deleteSessions(cleanupCtx)
	}()

	if authErr := target.authenticate(ctx, currentRun); authErr != nil {
		err = fmt.Errorf("authenticate: %w", authErr)
		return currentRun.report(), err
	}

	err = syncFunc(currentRun)
	report := currentRun.report()

	if failed := report.Failed(); failed > 0 {
		log.Warn().Int("failed", failed).Int("succeeded", report.Succeeded()).Msg("Replicas failed during sync")
	}

	if err == nil {
		err = report.Err()
	}

	return report, err
}

func (target *target) authenticate(ctx context.Context, currentRun *run) error {
	log.Info().Msg("Authenticating clients...")
	if err := target.Primary.PostAuth(ctx); err != nil {
		return fmt.Errorf("primary: %w", err)
	}

	currentRun.forEachReplica("authenticate", func(replica pihole.Client) error {
		return retry.Fixed(ctx, func() error {
			return replica.PostAuth(ctx)
		}, retry.AttemptsPostAuth)
	})

	return nil
}

func (target *target) deleteSessions(ctx context.Context) {
	log.Info().Msg("Invalidating sessions...")
	if err := target.Primary.DeleteSession(ctx); err != nil {
		log.Warn().Msgf("Failed to invalidate session for target: %s", target.Primary.String())
	}

	for _, replica := range target.Replicas {
		if err := retry.Fixed(ctx, func() error {
			return replica.DeleteSession(ctx)
		}, retry.AttemptsDeleteSession); err != nil {
			log.Warn().Msgf("Failed to invalidate session for target: %s", replica.String())
		}
	}
}

func (target *target) syncTeleporters(ctx context.Context, currentRun *run, gravitySettings *config.GravitySettings) error {
	log.Info().Msg("Syncing teleporters...")
	conf, err := target.Primary.GetTeleporter(ctx)
	if err != nil {
		return fmt.Errorf("primary: %w", err)
	}

	var teleporterRequest *model.PostTeleporterRequest
	if gravitySettings != nil {
		teleporterRequest = createPostTeleporterRequest(gravitySettings)
	}

	currentRun.forEachReplica("teleporter", func(replica pihole.Client) error {
		return retry.Fixed(ctx, func() error {
			return replica.PostTeleporter(ctx, conf, teleporterRequest)
		}, retry.AttemptsPostTeleporter)
	})

	return nil
}

func (target *target) syncConfigs(ctx context.Context, currentRun *run, configSettings *config.ConfigSettings) error {
	log.Info().Msg("Syncing configs...")
	configResponse, err := target.Primary.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("primary: %w", err)
	}

	configRequest := createPatchConfigRequest(configSettings, configResponse)

	currentRun.forEachReplica("config", func(replica pihole.Client) error {
		return retry.Fixed(ctx, func() error {
			return replica.PatchConfig(ctx, configRequest)
		}, retry.AttemptsPatchConfig)
	})

	return nil
}

func (target *target) runGravity(ctx context.Context, currentRun *run) error {
	log.Info().Msg("Running gravity...")

	if err := target.Primary.PostRunGravity(ctx); err != nil {
		return fmt.Errorf("primary: %w", err)
	}

	currentRun.forEachReplica("gravity", func(replica pihole.Client) error {
		return retry.Fixed(ctx, func() error {
			return replica.PostRunGravity(ctx)
		}, retry.AttemptsPostRunGravity)
	})

	return nil
}

func createPatchConfigRequest(config *config.ConfigSettings, configResponse *model.ConfigResponse) *model.PatchConfigRequest {
	patchConfig := model.PatchConfig{}

	if json := filterPatchConfigRequest(config.DNS, configResponse.Get("dns")); json != nil {
		patchConfig.DNS = json
	}
	if json := filterPatchConfigRequest(config.DHCP, configResponse.Get("dhcp")); json != nil {
		patchConfig.DHCP = json
	}
	if json := filterPatchConfigRequest(config.NTP, configResponse.Get("ntp")); json != nil {
		patchConfig.NTP = json
	}
	if json := filterPatchConfigRequest(config.Resolver, configResponse.Get("resolver")); json != nil {
		patchConfig.Resolver = json
	}
	if json := filterPatchConfigRequest(config.Database, configResponse.Get("database")); json != nil {
		patchConfig.Database = json
	}
	if json := filterPatchConfigRequest(config.Misc, configResponse.Get("misc")); json != nil {
		patchConfig.Misc = json
	}
	if json := filterPatchConfigRequest(config.Debug, configResponse.Get("debug")); json != nil {
		patchConfig.Debug = json
	}

	return &model.PatchConfigRequest{Config: patchConfig}
}

func filterPatchConfigRequest(setting *config.ConfigSetting, json map[string]any) map[string]any {
	if !setting.Enabled {
		return nil
	}

	if setting.Filter != nil {
		filteredJSON, err := filter.ByType(setting.Filter.Type, setting.Filter.Keys, json)
		if err != nil {
			log.Warn().Err(err).Msg("Unable to filter json object")
			return nil
		}
		return filteredJSON
	}

	return json
}

func createPostTeleporterRequest(gravity *config.GravitySettings) *model.PostTeleporterRequest {
	return &model.PostTeleporterRequest{
		Config:     false,
		DHCPLeases: gravity.DHCPLeases,
		Gravity: model.PostGravityRequest{
			Group:             gravity.Group,
			Adlist:            gravity.Adlist,
			AdlistByGroup:     gravity.AdlistByGroup,
			Domainlist:        gravity.Domainlist,
			DomainlistByGroup: gravity.DomainlistByGroup,
			Client:            gravity.Client,
			ClientByGroup:     gravity.ClientByGroup,
		},
	}
}
