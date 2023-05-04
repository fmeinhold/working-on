package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/tasks"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/spf13/cobra"
	"regexp"
	"strings"
	"time"
)

var (
	Pid int
)

func newStartCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "start",
		Short: "Start a time entry",
		Long: `Start a time entry

Either from a template set in your cfg file 
or by description/key, start time and duration`,

		Args: func(cmd *cobra.Command, args []string) error {
			times, description := GuessTypes(args)

			err := cfg.InitProjectConfig(false)
			if err != nil {
				panic(err)
			}

			defaultPid, err := cfg.GetDefaultProject()
			if err != nil {
				panic(err)
			}

			fmt.Println(defaultPid)

			cmd.Flags().IntVarP(&Pid, "pid", "p", defaultPid, "toggl_api project pid")

			if Pid == 0 {
				return fmt.Errorf("no pid set in config file or --pid/-p")
			}

			var start time.Time

			if len(times) == 1 {
				start = times[0]
			} else {
				start = time.Now()

			}

			toggl := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))

			timeEntry := toggl_api.NewTimeEntryRunning(cfg.GlobalConfig.GetInt(cfg.TogglDefaultWid), Pid, strings.Join(description, " "), true, true, &start)

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

	return command
}
