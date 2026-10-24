package cmd

import (
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"

	"github.com/lovelaze/nebula-sync/internal/health"
)

const defaultAPIAddr = ":8080"

var healthCmd = &cobra.Command{
	Use: "healthcheck",
	Run: func(cmd *cobra.Command, args []string) {
		if err := health.Check(cmd.Context(), healthURL()); err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	healthCmd.Hidden = true
	rootCmd.AddCommand(healthCmd)
}

func healthURL() string {
	addr := os.Getenv("API_ADDR")
	if addr == "" {
		addr = defaultAPIAddr
	}

	if host, port, err := net.SplitHostPort(addr); err == nil {
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		addr = net.JoinHostPort(host, port)
	}

	//nolint:revive // the healthcheck always targets the loopback api over plain http.
	return fmt.Sprintf("http://%s/health", addr)
}
