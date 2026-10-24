package retry

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go"
	"github.com/rs/zerolog/log"

	"github.com/lovelaze/nebula-sync/internal/config"
)

const (
	AttemptsPostTeleporter = 5
	AttemptsPatchConfig    = 5
	AttemptsPostRunGravity = 5
	AttemptsPostAuth       = 3
	AttemptsDeleteSession  = 3
)

var delay atomic.Int64

func Init(clientConfig *config.Client) {
	delay.Store(clientConfig.RetryDelay * int64(time.Second))
}

func Fixed(ctx context.Context, retryFunc func() error, attempts uint) error {
	return retry.Do(
		func() error {
			return retryFunc()
		},
		retry.Context(ctx),
		retry.Attempts(attempts),
		retry.Delay(time.Duration(delay.Load())),
		retry.LastErrorOnly(true),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(n uint, err error) {
			log.Debug().Msg(fmt.Sprintf("Retrying(%d): %v", n+1, err))
		}),
	)
}
