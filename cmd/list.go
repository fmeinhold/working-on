package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
	"time"
)

func newListCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {

			spinner, err := yacspin.New(yacspin.Config{
				Frequency:     100 * time.Millisecond,
				CharSet:       yacspin.CharSets[11],
				Suffix:        " retrieving projects ...",
				StopCharacter: "✓",
				StopColors:    []string{"fgGreen"},
			})

			client := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))
			me, err := client.Me.Get()
			if err != nil {
				return err
			}

			spinner.Message("Loading")
			spinner.Start()
			projects, err := client.Projects.List(me.DefaultWorkspaceId)
			spinner.Stop()

			for _, project := range projects {
				fmt.Println(project.Id, project.Name)
			}

			if err != nil {
				return err
			}

			return nil
		},
	}

	return command
}
