package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/spf13/cobra"
)

func newNowCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "now",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))
			timeEntry, err := client.TimeEntries.Current()
			if err != nil {
				return err
			}

			if timeEntry.Id == 0 {
				fmt.Println("You are slacking off.")
			} else {
				fmt.Println(timeEntry)
			}
			return nil
		},
	}
	return command
}
