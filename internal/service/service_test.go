package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lovelaze/nebula-sync/internal/config"
	syncmock "github.com/lovelaze/nebula-sync/internal/mocks/sync"
	"github.com/lovelaze/nebula-sync/internal/pihole/model"
)

func TestRun_full(t *testing.T) {
	conf := config.Config{
		Primary:  model.PiHole{},
		Replicas: []model.PiHole{},
		Sync: &config.Sync{
			FullSync: true,
			Cron:     nil,
		},
	}

	target := syncmock.NewTarget(t)
	callback := syncmock.NewCallback(t)
	target.On("FullSync", mock.Anything, conf.Sync).Return(nil, nil)
	callback.On("OnSuccess").Return(nil)

	service := NewService(target, conf, callback)

	err := service.Run(t.Context())
	require.NoError(t, err)

	target.AssertCalled(t, "FullSync", mock.Anything, conf.Sync)
}

func TestRun_selective(t *testing.T) {
	conf := config.Config{
		Primary:  model.PiHole{},
		Replicas: []model.PiHole{},
		Sync: &config.Sync{
			FullSync: false,
			Cron:     nil,
		},
	}

	target := syncmock.NewTarget(t)
	callback := syncmock.NewCallback(t)
	target.On("SelectiveSync", mock.Anything, conf.Sync).Return(nil, nil)
	callback.On("OnSuccess").Return(nil)

	service := NewService(target, conf, callback)

	err := service.Run(t.Context())
	require.NoError(t, err)

	target.AssertCalled(t, "SelectiveSync", mock.Anything, conf.Sync)
}

func TestRun_webhook_success(t *testing.T) {
	conf := config.Config{
		Primary:  model.PiHole{},
		Replicas: []model.PiHole{},
		Sync: &config.Sync{
			FullSync: false,
			Cron:     nil,
		},
	}

	target := syncmock.NewTarget(t)
	callback := syncmock.NewCallback(t)

	target.On("SelectiveSync", mock.Anything, conf.Sync).Return(nil, nil)
	callback.On("OnSuccess").Return(nil)

	service := NewService(target, conf, callback)

	err := service.Run(t.Context())
	require.NoError(t, err)

	target.AssertCalled(t, "SelectiveSync", mock.Anything, conf.Sync)
	callback.AssertCalled(t, "OnSuccess")
	callback.AssertNotCalled(t, "OnFailure")
}

func TestRun_webhook_failure(t *testing.T) {
	conf := config.Config{
		Primary:  model.PiHole{},
		Replicas: []model.PiHole{},
		Sync: &config.Sync{
			FullSync: false,
			Cron:     nil,
		},
	}

	syncErr := errors.New("sync failed")
	target := syncmock.NewTarget(t)
	callback := syncmock.NewCallback(t)

	target.On("SelectiveSync", mock.Anything, conf.Sync).Return(nil, syncErr)
	callback.On("OnFailure", syncErr).Return(nil)

	service := NewService(target, conf, callback)

	err := service.Run(t.Context())
	require.ErrorIs(t, err, syncErr)

	target.AssertCalled(t, "SelectiveSync", mock.Anything, conf.Sync)
	callback.AssertCalled(t, "OnFailure", syncErr)
	callback.AssertNotCalled(t, "OnSuccess")
}

func TestRun_webhook_error_does_not_affect_result(t *testing.T) {
	conf := config.Config{
		Primary:  model.PiHole{},
		Replicas: []model.PiHole{},
		Sync: &config.Sync{
			FullSync: true,
			Cron:     nil,
		},
	}

	target := syncmock.NewTarget(t)
	callback := syncmock.NewCallback(t)

	target.On("FullSync", mock.Anything, conf.Sync).Return(nil, nil)
	callback.On("OnSuccess").Return(nil)

	service := NewService(target, conf, callback)

	err := service.Run(t.Context())
	require.NoError(t, err)

	target.AssertCalled(t, "FullSync", mock.Anything, conf.Sync)
	callback.AssertCalled(t, "OnSuccess")
}
