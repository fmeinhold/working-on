package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/spf13/cobra"
)

func newWorkspacesCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))
			workspaces, err := client.Workspaces.List()
			if err != nil {
				return err
			}
			for _, workspace := range workspaces {
				fmt.Println(workspace.Id, workspace.Name)
			}
			return nil
		},
	}
	return command
}
