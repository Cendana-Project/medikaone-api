package cmd

import (
	"os"
	"strings"
	"sync"

	"github.com/Cendana-Project/medikaone-api/internal/config"
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	runtimeConfigOnce sync.Once
	runtimeConfigErr  error
)

var rootCmd = &cobra.Command{
	Use:           "medikaone-api",
	Short:         "MedikaOne API administration and server",
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		return initializeRuntimeConfig(validationScopeForCommand(cmd))
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logrus.WithError(err).Error("command failed")
		os.Exit(1)
	}
}

func validationScopeForCommand(cmd *cobra.Command) config.ValidationScope {
	switch cmd.Name() {
	case "database-fingerprint":
		return config.ValidationDatabaseTarget
	case "seed":
		return config.ValidationDatabase
	case "staging-reset-all", "staging-reset-seed":
		return config.ValidationMaintenance
	case "migrate":
		action, _ := cmd.Flags().GetString("action")
		action = strings.ToLower(strings.TrimSpace(action))
		switch action {
		case "create":
			return config.ValidationLocalFileCommand
		case "status":
			return config.ValidationDatabase
		default:
			return config.ValidationMigration
		}
	default:
		return config.ValidationServer
	}
}

func initializeRuntimeConfig(scope config.ValidationScope) error {
	runtimeConfigOnce.Do(func() {
		if err := config.LoadConfigFor(scope); err != nil {
			runtimeConfigErr = err
			return
		}

		level, err := logrus.ParseLevel(config.Env.LogLevel)
		if err != nil {
			runtimeConfigErr = err
			return
		}
		logrus.SetLevel(level)
		logrus.SetReportCaller(config.Env.Env == "development" && level == logrus.DebugLevel)
		if config.Env.Env == constant.ProductionEnvironment {
			logrus.SetFormatter(&logrus.JSONFormatter{})
			return
		}
		logrus.SetFormatter(&logrus.TextFormatter{
			DisableColors: false,
			FullTimestamp: true,
		})
	})
	return runtimeConfigErr
}
