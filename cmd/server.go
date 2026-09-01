/*
Copyright © 2024 Michael Putera Wardana <michaelputeraw@gmail.com>
*/
package cmd

import (
	"github.com/Cendana-Project/medikaone-api/internal/bootstrap"
	"github.com/spf13/cobra"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "REST API server",
	Long:  `start REST API server`,
	Run: func(cmd *cobra.Command, args []string) {
		bootstrap.StartServer()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
