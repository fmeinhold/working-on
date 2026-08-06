package cmd

import (
	"fmt"
	"time"

	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

func NewContinueCommand(cfg *workingon.Config) *cobra.Command {
	var (
		dry      bool
		appendTo bool
	)

	command := &cobra.Command{
		Use:   "continue",
		Short: "Continue the last time entry",
		Long: `Continue the last time entry.

Opens a new running timer with the same description, project and task as the
most recent entry. The earlier block keeps its own record - this does not
reopen it.

Shorthand for "wo start --continue".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var start time.Time

			if appendTo {
				var err error
				start, err = workingon.AppendStartTime(cfg)
				if err != nil {
					return err
				}
			}

			timeEntry, err := workingon.ContinueLast(cfg, start, dry)
			if err != nil {
				return err
			}

			fmt.Printf("Continuing: %s \n",
				timeEntry.Format(cfg.Settings.DateTimeLayout, &cfg.Settings.Location))

			return nil
		},
	}

	command.Flags().BoolVarP(&dry, "dry", "d", false, "Do not create anything in toggl")
	command.Flags().BoolVarP(&appendTo, "append", "a", false,
		"Start where the last entry stopped instead of now")

	return command
}
