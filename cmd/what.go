package cmd

import (
	"fmt"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

func NewWhatCommand(cfg *workingon.Config) *cobra.Command {
	whatCommand := &cobra.Command{
		Use:   "what",
		Short: "What are you working on?",
		Long: `What are you working on?

Shows the timer that is running right now, the same as ` + "`wo now`" + `. For a day's
worth of entries, see ` + "`wo show`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := toggl.NewToggl(cfg.Settings.ToggleApiToken)

			current, err := cl.TimeEntries.Current()
			if err != nil {
				return err
			}

			if jsonOutput {
				return emitCurrent(current, cfg)
			}

			prompt, _ := cmd.Flags().GetBool("prompt")
			fmt.Print(RenderCurrent(current, cfg, prompt))

			return nil
		},
	}
	whatCommand.Flags().BoolP("prompt", "p", false, "Output an indicator for usage in a shell prompt")

	return whatCommand
}
