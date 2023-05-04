package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/spf13/cobra"
)

func newStopCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "stop",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))

			current, err := client.TimeEntries.StopCurrent()
			if err != nil {
				return err
			}

			fmt.Printf("Stopped time entry %s \n", current)

			return nil
		},
	}
	return command
}
