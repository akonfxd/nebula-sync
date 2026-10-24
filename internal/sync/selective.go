package sync

import (
	"context"
	"fmt"

	"github.com/lovelaze/nebula-sync/internal/config"
)

func (target *target) SelectiveSync(ctx context.Context, conf *config.Sync) error {
	return target.sync(ctx, func() error {
		return target.selective(ctx, conf)
	}, "selective")
}

func (target *target) selective(ctx context.Context, conf *config.Sync) error {
	if err := target.syncTeleporters(ctx, conf.GravitySettings); err != nil {
		return fmt.Errorf("sync teleporters: %w", err)
	}

	if err := target.syncConfigs(ctx, conf.ConfigSettings); err != nil {
		return fmt.Errorf("sync configs: %w", err)
	}

	if conf.RunGravity {
		if err := target.runGravity(ctx); err != nil {
			return fmt.Errorf("run gravity: %w", err)
		}
	}
	return nil
}
