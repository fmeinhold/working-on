package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/spf13/cobra"
)

func newAddCommand() *cobra.Command {

	command := &cobra.Command{
		Use:   "add",
		Short: "Add a time entry",
		Long:  `Add a time entry`,

		RunE: func(cmd *cobra.Command, args []string) error {

			err := cfg.InitProjectConfig(false)
			if err != nil {
				panic(err)
			}

			if Pid == 0 {
				return fmt.Errorf("no pid set in config file or --pid/-p")
			}

			timeEntry, err := newTimeEntryFromArgs(args)

			timeEntry.ProjectID = Pid
			timeEntry.WorkspaceID = cfg.GlobalConfig.GetInt(cfg.TogglDefaultWid)

			if err != nil {
				return err
			}

			toggl := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))

			timeEntry, err = toggl.TimeEntries.Create(timeEntry)

			if err != nil {
				return err
			}

			fmt.Println(timeEntry)

			return nil
		},
	}

	return command

}
