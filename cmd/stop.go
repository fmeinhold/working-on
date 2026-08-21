package cmd

import (
	"fmt"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

func NewStopCommand(cfg *workingon.Config) *cobra.Command {
	var stopCommand = &cobra.Command{
		Use:   "stop",
		Short: "Stop currently running timer",
		Long:  `Stop currently running timer`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Stopping saves the entry, which toggl can refuse while it has no
			// description.
			if _, err := workingon.NameRunningEntry(cfg, describer(cfg)); err != nil {
				return err
			}

			cl := toggl.NewToggl(cfg.Settings.ToggleApiToken)

			timeEntry, err := cl.TimeEntries.StopCurrent()
			if err != nil {
				return err
			}

			if jsonOutput {
				return emitEntry("stopped", timeEntry, cfg)
			}
			fmt.Print(renderStopped(timeEntry, cfg))

			return nil
		},
	}
	return stopCommand
}

// renderStopped says what was booked, in the zone and layout the user reads.
//
// Not "%s" of the entry: its String is Go's idea of a time - a UTC timestamp
// with the offset spelled out on the end - which is no way to tell somebody
// what afternoon they just booked.
func renderStopped(entry *toggl.TimeEntry, cfg *workingon.Config) string {
	return fmt.Sprintf("Stopped %s\n",
		entry.Format(cfg.Settings.DateTimeLayout, &cfg.Settings.Location))
}
