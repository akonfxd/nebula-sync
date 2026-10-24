package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/lovelaze/nebula-sync/internal/log"
	"github.com/lovelaze/nebula-sync/version"
)

var rootCmd = &cobra.Command{
	Use:     "nebula-sync",
	Version: version.Version,
}

func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	cobra.OnInitialize(log.Init)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}
