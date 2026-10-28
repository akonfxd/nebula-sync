package config

import (
	"os"
	"slices"
	"strings"
)

var (
	syncEnvNames    = []string{"PRIMARY", "PRIMARY_FILE", "REPLICAS", "REPLICAS_FILE", "FULL_SYNC", "RUN_GRAVITY", "CRON"}
	syncEnvPrefixes = []string{"SYNC_", "WEBHOOK_"}
)

func IgnoredSyncEnvVars() []string {
	present := []string{}

	for _, name := range syncEnvNames {
		if _, found := os.LookupEnv(name); found {
			present = append(present, name)
		}
	}

	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")

		for _, prefix := range syncEnvPrefixes {
			if strings.HasPrefix(name, prefix) {
				present = append(present, name)
				break
			}
		}
	}

	slices.Sort(present)

	return slices.Compact(present)
}
