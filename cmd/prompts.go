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

const untitled = "Untitled"

func describer(cfg *workingon.Config) workingon.Describer {
	return describerFor(cfg, os.Stdin, os.Stdout, interactive())
}

// A run that asked for JSON is a program reading the output, and a program
// cannot answer a prompt - it would sit waiting on an answer that never comes,
// having already been given a document it could not parse.
func interactive() bool {
	return !jsonOutput && isatty.IsTerminal(os.Stdin.Fd())
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

// A run with nobody to ask - a script, a cron job - leaves the placeholders as
// they were, so a scripted `wo add call` still books something rather than
// stopping on a question no one will see.
func templateArgAsker(interactive bool) workingon.TemplateArgAsker {
	if !interactive {
		return nil
	}
	return askTemplateArgsFrom(os.Stdin, os.Stdout)
}

// An answer left blank is no answer: the placeholder renders as <no value>, as
// it would have without asking.
func askTemplateArgsFrom(in io.Reader, out io.Writer) workingon.TemplateArgAsker {
	prompt := &prompter{reader: bufio.NewReader(in), out: out}

	return func(alias string, names []string) (map[string]string, error) {
		fmt.Fprintf(out, "\nTemplate %q asks for %s:\n", alias, countOfArgs(len(names)))

		answers := make(map[string]string, len(names))
		for _, name := range names {
			answers[name] = prompt.line(name, "")
		}

		return answers, nil
	}
}

func countOfArgs(count int) string {
	if count == 1 {
		return "1 argument"
	}
	return fmt.Sprintf("%d arguments", count)
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
