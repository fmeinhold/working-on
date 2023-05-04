package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/spf13/cobra"
)

func newContinueCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "continue",
		Short: "Continue a time entry",
		RunE: func(cmd *cobra.Command, args []string) error {

			client := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))

			timeEntry, err := client.TimeEntries.MostRecent()
			if err != nil {
				return err
			}

			fmt.Printf("Continue %s \n", timeEntry)

			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {

			client := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))

			timeEntries, err := client.TimeEntries.MostRecent()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			var keys []string
			for _, timeEntry := range timeEntries {
				keys = append(keys, fmt.Sprintf(`"%d: %s"`, timeEntry.Id, timeEntry.Description))
			}

			return keys, cobra.ShellCompDirectiveNoFileComp
		},
	}
	return command
}
