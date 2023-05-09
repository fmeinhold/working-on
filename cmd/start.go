package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/tasks"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/spf13/cobra"
	"regexp"
)

func newStartCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "start",
		Short: "Start a time entry",
		Long: `Start a time entry

Either from a template set in your cfg file 
or by description/key, start time and duration`,

		Args: func(cmd *cobra.Command, args []string) error {
			err := cfg.InitProjectConfig(false)
			if err != nil {
				return err
			}

			localPid, err := cfg.GetDefaultProject()

			if Pid == 0 && localPid == 0 {
				return fmt.Errorf("no pid set in config file or --pid/-p")
			} else if localPid != 0 {
				Pid = localPid
			}

			toggl := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))

			current, err := toggl.TimeEntries.Current()
			if err != nil {
				return err
			}

			if current.IsSet() {
				current, err = toggl.TimeEntries.Stop(current)
				if err != nil {
					return err
				}
			}

			timeEntry, err := newTimeEntryFromArgs(args)
			if err != nil {
				return err
			}

			timeEntry.WorkspaceID = Wid
			timeEntry.ProjectID = Pid
			timeEntry.Duration = timeEntry.Duration * -1

			timeEntry, err = toggl.TimeEntries.Start(timeEntry)
			if err != nil {
				return err
			}

			fmt.Println(timeEntry)

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {

			js, err := tasks.NewJiraSource()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			re := regexp.MustCompile(`([0-9A-Z]+)-`)

			matches := re.FindStringSubmatch(toComplete)

			if matches != nil {
				result, err := js.FetchTasks(matches[1])

				if err != nil {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}
				var keys []string
				for _, task := range result {
					keys = append(keys, fmt.Sprintf("%s: %s", task.Key, task.Summary))
					//keys = append(keys, task.Key)
				}
				return keys, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}

	/*	err := command.RegisterFlagCompletionFunc("task", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {

			toggl := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))
			result, err := toggl.Tasks.FetchAll()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var keys []string
			for _, task := range result {
				keys = append(keys, fmt.Sprintf("%s: %s", task.Id, task.Name))
				//keys = append(keys, task.Key)
			}
			return keys, cobra.ShellCompDirectiveNoFileComp
		})

		if err != nil {
			return nil
		}
	*/return command
}
