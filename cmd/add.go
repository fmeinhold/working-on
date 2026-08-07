package cmd

import (
	"fmt"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"

	//"strconv"
	"time"
)

var (
	UnableToParseArgs = fmt.Errorf("unable to make sense of the arguments")
	DurationRequired  = fmt.Errorf("a duration is required")
	//NoProject         = fmt.Errorf("unable to figure out project")
)

type CommandArgs struct {
	Duration     time.Duration
	StartTime    time.Time
	SummaryOrKey string
}

type TaskDef struct {
	Name      string `mapstructure:"name"`
	TogglTask int64  `mapstructure:"toggl_task"`
	Start     string `mapstructure:"start"`
	Stop      string `mapstructure:"stop"`
	Alias     string `mapstructure:"alias"`
}

func NewAddCommand(cfg *workingon.Config) *cobra.Command {
	var (
		duration   time.Duration
		start      time.Time
		dryRun     bool
		tail       []string
		addCommand = &cobra.Command{
			Use:   "add <summary|task|template alias> <times>",
			Short: "Add a time entry",
			Long: `Add a time entry.

The first argument says what the entry is about - a description, a task by
name or by id, or a template alias:

  wo add "fixing the parser" 9:00 1h
  wo add "ATD Conference" 9:00 1h
  wo add 241929955 9:00 1h
  wo add ds

A task name is matched within the project the entry lands in, so a description
is never mistaken for one. ` + "`wo tasks`" + ` lists what you can book against, and
` + "`wo templates`" + ` the aliases you have set up.

The times that follow are a start and either a stop or a duration, and a date
can lead them:

  wo add "fixing the parser" 9:00-10:00
  wo add "fixing the parser" yesterday 9:00 1h`,

			Args: func(cmd *cobra.Command, args []string) error {
				var err error
				start, duration, tail, err = ParseArgs(newParseArgsConfig(cfg), args)
				return err
			},
			RunE: func(cmd *cobra.Command, args []string) error {

				templateArgs, err := cmd.Flags().GetStringToString("templateArgs")
				if err != nil {
					return err
				}

				wid, err := cmd.Flags().GetInt("wid")
				if err != nil {
					return err
				}

				project, err := cmd.Flags().GetString("project")
				if err != nil {
					return err
				}

				append_, err := cmd.Flags().GetBool("append")
				if err != nil {
					return err
				}

				if append_ {
					start, err = workingon.AppendStartTime(cfg)
					if err != nil {
						return err
					}
				}

				task, err := cmd.Flags().GetString("task")
				if err != nil {
					return err
				}

				stop, err := cmd.Flags().GetString("stop")
				if err != nil {
					return err
				}

				pickTask, err := cmd.Flags().GetBool("pick-task")
				if err != nil {
					return err
				}

				timeEntry, err := workingon.AddOrStart(cfg, workingon.EntryRequest{
					Wid:            wid,
					Project:        project,
					Task:           task,
					SummaryOrKey:   strings.Join(tail, " "),
					Start:          start,
					Stop:           stop,
					Duration:       duration,
					TemplateArgs:   templateArgs,
					DryRun:         dryRun,
					Describe:       describer(cfg),
					ChooseTask:     taskChooser(interactive()),
					AskTemplateArg: templateArgAsker(interactive()),
					PickTask:       pickTask,
				})
				if err != nil {
					return err
				}
				fmt.Println(timeEntry.Format(cfg.Settings.DateTimeLayout, &cfg.Settings.Location))

				return nil
			},
		}
	)

	// Flags
	addCommand.Flags().StringP("stop", "s", "", "Stop Time")
	addCommand.Flags().StringP("project", "p", viper.GetString("TOGGL_PROJECT"),
		"Set the toggl project, by id or by name")
	addCommand.Flags().String("task", viper.GetString("TOGGL_TASK"),
		"Set the toggl task, by id or by name")
	addCommand.Flags().Bool("pick-task", false,
		"Choose the task from this project's tasks, even where the workspace does not require one")
	addCommand.Flags().BoolVarP(&dryRun, "dry", "d", false, "Do not create anything in toggl")
	addCommand.Flags().BoolP("append", "a", false, "Append to last time entry")
	addCommand.Flags().BoolP("fuzzy", "f", false, "Add some fuzziness to the start and stop time")
	addCommand.Flags().IntP("wid", "w", cfg.Settings.ToggleWid, "Toggle track workspace id")

	addCommand.Flags().StringToStringP("templateArgs", "t", nil, "List of named template args")

	return addCommand
}
