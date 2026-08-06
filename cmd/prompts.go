package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"

	"github.com/mattn/go-isatty"
)

// untitled is what an entry is called when there is nobody to ask and no
// default configured. Toggl workspaces can require a description, and a placeholder
// that says the entry was never named beats failing to track the time at all.
const untitled = "Untitled"

// describer names entries that carry no description of their own: the one being
// created, and the timer already running that starting or stopping it saves.
//
// toggl_default_description settles the question outright. Without it, the
// answer is asked for, and only a run with nowhere to ask - a script, a cron
// job - falls back to untitled.
func describer(cfg *workingon.Config) workingon.Describer {
	return describerFor(cfg, os.Stdin, os.Stdout, interactive())
}

// interactive reports whether there is anybody at the other end to answer a
// question.
func interactive() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
}

func describerFor(cfg *workingon.Config, in io.Reader, out io.Writer, interactive bool) workingon.Describer {
	fallback := cfg.Settings.ToggleDefaultDescription
	if fallback == "" {
		fallback = untitled
	}

	if cfg.Settings.ToggleDefaultDescription != "" || !interactive {
		return func(*toggl.TimeEntry) (string, error) { return fallback, nil }
	}

	prompt := &prompter{reader: bufio.NewReader(in), out: out}

	return func(entry *toggl.TimeEntry) (string, error) {
		fmt.Fprintf(out, "\n%s has no description, and toggl needs one to save it.\n",
			describeSubject(entry))

		return prompt.line("What was it", fallback), nil
	}
}

// taskChooser offers a project's tasks and returns the one picked, or nil when
// the question was left unanswered.
//
// A run with nobody to ask - a script, a cron job - answers nothing at all, so
// a workspace that requires a task reports that plainly instead of hanging on a
// prompt no one will see.
func taskChooser(interactive bool) workingon.TaskChooser {
	if !interactive {
		return nil
	}
	return chooseTaskFrom(os.Stdin, os.Stdout)
}

func chooseTaskFrom(in io.Reader, out io.Writer) workingon.TaskChooser {
	prompt := &prompter{reader: bufio.NewReader(in), out: out}

	return func(projectId int, tasks []workingon.Task) (*workingon.Task, error) {
		names := make([]string, len(tasks))
		for i, task := range tasks {
			names[i] = task.Summary
		}

		fmt.Fprintf(out, "\nTasks in project %d:\n", projectId)

		index := pickOne(prompt, "Which task", names)
		if index < 0 {
			return nil, nil
		}

		return &tasks[index], nil
	}
}

// describeSubject says which entry is being asked about, since the answer is
// usually wanted for the timer that was already running rather than the one
// just asked for.
func describeSubject(entry *toggl.TimeEntry) string {
	if entry.Id != 0 {
		return "The running entry"
	}
	return "This entry"
}
