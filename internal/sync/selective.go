package sync

import (
	"context"
	"fmt"

	"github.com/lovelaze/nebula-sync/internal/config"
)

func (target *target) SelectiveSync(ctx context.Context, conf *config.Sync) (*Report, error) {
	return target.sync(ctx, func(currentRun *run) error {
		return target.selective(ctx, currentRun, conf)
	}, "selective")
}

func (target *target) selective(ctx context.Context, currentRun *run, conf *config.Sync) error {
	if err := target.syncTeleporters(ctx, currentRun, conf.GravitySettings); err != nil {
		return fmt.Errorf("sync teleporters: %w", err)
	}

	if err := target.syncConfigs(ctx, currentRun, conf.ConfigSettings); err != nil {
		return fmt.Errorf("sync configs: %w", err)
	}

	if conf.RunGravity {
		if err := target.runGravity(ctx, currentRun); err != nil {
			return fmt.Errorf("run gravity: %w", err)
		}
	}
	return nil
}
