package chatlog

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sjzar/chatlog/internal/chatlog/conf"
	"github.com/sjzar/chatlog/pkg/config"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configPostgresCmd)
	configPostgresCmd.Flags().StringVarP(&configPostgresURL, "url", "u", "", "PostgreSQL connection URL")
}

var (
	configPostgresURL string
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage chatlog configuration",
	Long:  `View and update settings in ~/.chatlog/chatlog.json`,
}

var configPostgresCmd = &cobra.Command{
	Use:   "postgres [url]",
	Short: "Set PostgreSQL connection URL for sync",
	Long:  `Add or update the Postgres URL in chatlog.json. Used by 'chatlog sync' when --postgres-url is not provided.`,
	Example: `  chatlog config postgres "postgres://user:pass@localhost:5432/chatlog?sslmode=disable"
  chatlog config postgres --url "postgres://user:pass@localhost:5432/chatlog"
  CHATLOG_POSTGRES_URL="postgres://..." chatlog config postgres`,
	Run: func(cmd *cobra.Command, args []string) {
		url := configPostgresURL
		if url == "" && len(args) > 0 {
			url = args[0]
		}
		if url == "" {
			url = os.Getenv(conf.EnvPrefix + "_POSTGRES_URL")
		}
		if url == "" {
			fmt.Fprintln(os.Stderr, "Postgres URL is required. Provide it as argument, --url flag, or CHATLOG_POSTGRES_URL env.")
			os.Exit(1)
		}

		configPath := os.Getenv(conf.EnvConfigDir)
		cm, err := config.New(conf.AppName, configPath, "", "", true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config failed: %v\n", err)
			os.Exit(1)
		}

		// Load existing config so we don't overwrite other keys
		tuiConf := &conf.TUIConfig{}
		_ = cm.Load(tuiConf)

		if err := cm.SetConfig("postgres.url", url); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
			os.Exit(1)
		}

		path := cm.Path
		if path != "" {
			path += string(os.PathSeparator)
		}
		path += cm.Name + ".json"
		fmt.Printf("Postgres URL saved to %s\n", path)
	},
}
