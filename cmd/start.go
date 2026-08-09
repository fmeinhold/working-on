package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewStartCommand(cfg *workingon.Config) *cobra.Command {
	var (
		appendTo bool
		dry      bool
		cont     bool
		project  string
		start    time.Time
		duration time.Duration
		tail     []string
	)

	startCommand := &cobra.Command{
		Use:   "start <summary|task|template alias> <start time>",
		Short: "Start working on a task",
		Long: `Start a running timer.

The first argument says what the entry is about - a description, a task by
name or by id, or a template alias:

  wo start "fixing the parser"
  wo start "ATD Conference"
  wo start 241929955
  wo start ds

A task name is matched within the project the entry lands in, so a description
is never mistaken for one. ` + "`wo tasks`" + ` lists what you can book against, and
` + "`wo templates`" + ` the aliases you have set up.

The timer starts now unless you say when, and a date can lead the time:

  wo start "fixing the parser" 9:00
  wo start "fixing the parser" yesterday 9:00

--append starts it where the last entry stopped, so the gap since then belongs
to this one. --continue starts a fresh entry carrying the last one's
description, project and task, and takes no summary of its own.

A template's own start and stop are not used here - a timer runs from when it
starts until you stop it.`,
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

			if appendTo {
				// Back-date the timer to where the last entry ended, so the
				// gap since then belongs to this task.
				start, err = workingon.AppendStartTime(cfg)
				if err != nil {
					return err
				}
			}

			pickTask, err := cmd.Flags().GetBool("pick-task")
			if err != nil {
				return err
			}

			if cont {
				timeEntry, err := workingon.ContinueLast(cfg, workingon.EntryRequest{
					Start:      start,
					DryRun:     dry,
					Describe:   describer(cfg),
					ChooseTask: taskChooser(interactive()),
					PickTask:   pickTask,
				})
				if err != nil {
					return err
				}
				if jsonOutput {
					return emitEntry("continued", timeEntry, cfg)
				}
				fmt.Printf("Continuing: %s \n",
					timeEntry.Format(cfg.Settings.DateTimeLayout, &cfg.Settings.Location))
				return nil
			}

			wid, err := cmd.Flags().GetInt("wid")
			if err != nil {
				return err
			}

			project, err := cmd.Flags().GetString("project")
			if err != nil {
				return err
			}

			task, err := cmd.Flags().GetString("task")
			if err != nil {
				return err
			}

			timeEntry, err := workingon.AddOrStart(cfg, workingon.EntryRequest{
				Wid:            wid,
				Project:        project,
				Task:           task,
				SummaryOrKey:   strings.Join(tail, " "),
				Start:          start,
				Duration:       duration,
				TemplateArgs:   templateArgs,
				Running:        true,
				DryRun:         dry,
				Describe:       describer(cfg),
				ChooseTask:     taskChooser(interactive()),
				AskTemplateArg: templateArgAsker(interactive()),
				PickTask:       pickTask,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitEntry("started", timeEntry, cfg)
			}
			fmt.Printf("Started tracking for: %s \n", timeEntry.Format(cfg.Settings.DateTimeLayout, &cfg.Settings.Location))

			return nil

		},
	}

	startCommand.Flags().BoolVarP(&appendTo, "append", "a", false, "Use stop time of last time entry as start time for this task")
	startCommand.Flags().BoolVarP(&cont, "continue", "c", false, "Continue last task")
	startCommand.Flags().BoolVarP(&dry, "dry", "d", false, "Do not create anything in toggl")
	startCommand.Flags().StringVarP(&project, "project", "p", viper.GetString("TOGGL_PROJECT"),
		"Set the toggl project, by id or by name")
	startCommand.Flags().String("task", viper.GetString("TOGGL_TASK"),
		"Set the toggl task, by id or by name")
	startCommand.Flags().Bool("pick-task", false,
		"Choose the task from this project's tasks, even where the workspace does not require one")
	startCommand.Flags().StringToStringP("templateArgs", "t", nil, "List of named template args")
	startCommand.Flags().IntP("wid", "w", cfg.Settings.ToggleWid, "Toggle track workspace id")

	return startCommand
}
