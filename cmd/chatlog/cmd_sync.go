package chatlog

import (
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/sjzar/chatlog/internal/chatlog"
	"github.com/sjzar/chatlog/internal/chatlog/conf"
)

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.PersistentPreRun = initLog
	syncCmd.Flags().StringVarP(&syncPostgresURL, "postgres-url", "u", "", "PostgreSQL connection URL")
	syncCmd.Flags().StringVarP(&syncAccount, "account", "a", "", "Sync only this account (default: all history accounts)")
	syncCmd.Flags().StringVarP(&syncWorkDir, "work-dir", "w", "", "Work dir (decrypted data path)")
	syncCmd.Flags().StringVarP(&syncPlatform, "platform", "p", "", "Platform (darwin, windows)")
	syncCmd.Flags().IntVarP(&syncVersion, "version", "v", 0, "WeChat version (3 or 4)")
}

var (
	syncPostgresURL string
	syncAccount     string
	syncWorkDir     string
	syncPlatform    string
	syncVersion     int
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync raw conversation data to PostgreSQL",
	Long:  `Sync messages, contacts, and chat rooms from decrypted WeChat DBs to PostgreSQL for downstream usage.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmdConf := getSyncConfig()
		m := chatlog.New()
		if err := m.CommandSync("", cmdConf); err != nil {
			log.Fatal().Err(err).Msg("sync failed")
		}
		log.Info().Msg("sync completed")
	},
}

func getSyncConfig() map[string]any {
	cmdConf := make(map[string]any)
	if syncPostgresURL != "" {
		cmdConf["postgres_url"] = syncPostgresURL
	}
	if syncAccount != "" {
		cmdConf["account"] = syncAccount
	}
	if syncWorkDir != "" {
		cmdConf["work_dir"] = syncWorkDir
	}
	if syncPlatform != "" {
		cmdConf["platform"] = syncPlatform
	}
	if syncVersion != 0 {
		cmdConf["version"] = syncVersion
	}
	// Env fallback for postgres URL
	if _, ok := cmdConf["postgres_url"]; !ok {
		if url := os.Getenv(conf.EnvPrefix + "_POSTGRES_URL"); url != "" {
			cmdConf["postgres_url"] = url
		}
	}
	return cmdConf
}
