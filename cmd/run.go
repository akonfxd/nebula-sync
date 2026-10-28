package cmd

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/lovelaze/nebula-sync/internal/config"
	"github.com/lovelaze/nebula-sync/internal/service"
)

var envFile string

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run sync",
	Run: func(cmd *cobra.Command, args []string) {
		readEnvFile()

		svc, err := service.Init(cmd.Context())
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to initialize service")
		}

		runErr := svc.Run(cmd.Context())

		if closeErr := svc.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close configuration database")
		}

		if runErr != nil {
			log.Fatal().Err(runErr).Msg("Sync failed")
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&envFile, "env-file", "", "Read env from `.env` file")
}

func readEnvFile() {
	if envFile == "" {
		return
	}

	if err := config.LoadEnvFile(envFile); err != nil {
		log.Fatal().Err(err).Msg("Failed to load env file")
	}
}
