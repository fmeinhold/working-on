package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"

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

// endOfDayAsker is who `wo sanitize` puts an entry that ran past midnight to.
// A run with nobody to ask leaves the question unasked, and such an entry is
// capped at day_ends as it always was.
func endOfDayAsker(cfg *workingon.Config) workingon.EndOfDayAsker {
	if !interactive() {
		return nil
	}
	return endOfDayAskerFor(cfg, os.Stdin, os.Stdout)
}

func endOfDayAskerFor(cfg *workingon.Config, in io.Reader, out io.Writer) workingon.EndOfDayAsker {
	prompt := &prompter{reader: bufio.NewReader(in), out: out}
	loc := &cfg.Settings.Location
	clock := timeLayout(cfg)

	return func(ask workingon.EndOfDay) time.Time {
		began, ranUntil := ask.Began.In(loc), ask.RanUntil.In(loc)

		// The times are on two different dates by now, so the one that is not
		// the entry's own has to say which day it is on.
		show := momentTextOn(began, loc, clock, cfg.Settings.DateTimeLayout)

		fmt.Fprintf(out, "\n%q %s past midnight - it began at %s and has %s on the clock.\n",
			describedAs(ask.Entry.Description), ranOnOrWasLeft(ask.Running),
			began.Format(clock), tableDuration(ranUntil.Sub(began)))

		var offered string
		if !ask.Suggested.IsZero() {
			offered = ask.Suggested.In(loc).Format(clock)
		}

		for {
			answer := prompt.line("When did it stop", offered)
			if answer == "" {
				fmt.Fprintln(out, "Left as it was tracked.")
				return time.Time{}
			}

			ended, err := parseMoment(cfg, answer, began)
			if err != nil {
				fmt.Fprintf(out, "%v\n", err)
				continue
			}

			// A time of day that lands before the start is the small hours of
			// the next morning, which is how `wo modify --stop` reads one too.
			if !ended.After(began) && !hasDate(cfg, answer) {
				ended = ended.AddDate(0, 0, 1)
			}

			switch {
			case !ended.After(began):
				fmt.Fprintf(out, "It began at %s, so it cannot have stopped by then.\n",
					show(&began))
			case ended.After(ranUntil):
				fmt.Fprintf(out, "%s is as far as it got, so it cannot have run past that.\n",
					show(&ranUntil))
			default:
				return ended
			}
		}
	}
}

// An entry with a stop ran on; one still going was left running, which is a
// thing that happened to it rather than something it did.
func ranOnOrWasLeft(running bool) string {
	if running {
		return "was left running"
	}
	return "ran on"
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
