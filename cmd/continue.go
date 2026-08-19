package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

// recentShown is how many of the recent entries are offered at once. It is the
// length of a list somebody reads rather than scrolls, and a query narrows the
// way to anything below it.
const recentShown = 10

func NewContinueCommand(cfg *workingon.Config) *cobra.Command {
	var (
		dry      bool
		appendTo bool
		recent   bool
	)

	command := &cobra.Command{
		Use:   "continue [query]",
		Short: "Continue the last time entry, or an earlier one",
		Long: `Continue the last time entry, or an earlier one.

Opens a new running timer with the same description, project and task as the
most recent entry. The earlier block keeps its own record - this does not
reopen it.

--recent offers the last ` + fmt.Sprint(recentShown) + ` things you worked on instead, newest first, and
starts the one you pick:

  wo continue --recent

The same work booked several times is one line rather than several, and a timer
still running is left out - there is nothing to continue about work that has not
stopped.

A query narrows that listing, matching letters in order rather than as a run,
so it is the shape of the thing you want rather than its spelling:

  wo continue --recent oauth
  wo continue --recent "lp3 dbq"

With nobody to ask - a script, or --json - a query that leaves exactly one
entry starts it, and anything else is an error saying what it found.

Shorthand for "wo start --continue".`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.TrimSpace(strings.Join(args, " "))

			if query != "" && !recent {
				return fmt.Errorf(
					"a query only narrows the recent listing - did you mean `wo continue --recent %s`?", query)
			}

			var start time.Time

			if appendTo {
				var err error
				start, err = workingon.AppendStartTime(cfg)
				if err != nil {
					return err
				}
			}

			pickTask, err := cmd.Flags().GetBool("pick-task")
			if err != nil {
				return err
			}

			req := workingon.EntryRequest{
				Start:      start,
				DryRun:     dry,
				Describe:   describer(cfg),
				ChooseTask: taskChooser(interactive()),
				PickTask:   pickTask,
			}

			var timeEntry *toggl.TimeEntry

			if recent {
				chosen, err := chooseRecent(cfg, query, interactive())
				if err != nil {
					return err
				}
				if chosen == nil {
					fmt.Println("Nothing chosen, so nothing was started.")
					return nil
				}

				timeEntry, err = workingon.ContinueEntry(cfg, chosen, req)
				if err != nil {
					return err
				}
			} else {
				timeEntry, err = workingon.ContinueLast(cfg, req)
				if err != nil {
					return err
				}
			}

			if jsonOutput {
				return emitEntry("continued", timeEntry, cfg)
			}
			fmt.Printf("Continuing: %s \n",
				timeEntry.Format(cfg.Settings.DateTimeLayout, &cfg.Settings.Location))

			return nil
		},
	}

	command.Flags().BoolVarP(&dry, "dry", "d", false, "Do not create anything in toggl")
	command.Flags().BoolVarP(&appendTo, "append", "a", false,
		"Start where the last entry stopped instead of now")
	command.Flags().BoolVarP(&recent, "recent", "r", false,
		"Pick from the last few things you worked on rather than the last one")
	command.Flags().Bool("pick-task", false,
		"Choose the task rather than carrying over the one the last entry had")

	return command
}

// chooseRecent answers with the entry to continue, or nil where the listing was
// offered and nothing was picked.
func chooseRecent(cfg *workingon.Config, query string, ask bool) (*toggl.TimeEntry, error) {
	entries, err := workingon.RecentEntries(cfg)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("nothing has been tracked in the last %d days, so there is nothing to continue",
			workingon.RecentDays)
	}

	labels := make([]string, len(entries))
	for i := range entries {
		labels[i] = sanitizeLabel(&entries[i], nameResolver(&entries[i]))
	}

	// The query is matched against what the entry was, not when it was: a
	// listing of times is not something anybody searches by.
	matched := fuzzyMatches(labels, query)

	if len(matched) == 0 {
		return nil, fmt.Errorf("nothing in the last %d days matches %q", workingon.RecentDays, query)
	}

	if !ask {
		return theOnlyOne(entries, labels, matched, query)
	}

	if len(matched) > recentShown {
		matched = matched[:recentShown]
	}

	choices := make([]string, len(matched))
	for i, index := range matched {
		choices[i] = recentChoice(&entries[index], labels[index], cfg)
	}

	fmt.Printf("\nThe last %s you worked on:\n", countOfEntries(len(choices)))

	prompt := &prompter{reader: bufio.NewReader(os.Stdin), out: os.Stdout}

	picked := pickOneMatching(prompt, "Which one", choices, fuzzyMatches)
	if picked < 0 {
		return nil, nil
	}

	return &entries[matched[picked]], nil
}

// With nobody to ask, one match is an answer and anything else is a question -
// which is the same rule the rest of wo follows under --json.
func theOnlyOne(entries []toggl.TimeEntry, labels []string, matched []int, query string) (*toggl.TimeEntry, error) {
	if len(matched) == 1 {
		return &entries[matched[0]], nil
	}

	var found []string
	for _, index := range matched {
		if len(found) == 3 {
			found = append(found, fmt.Sprintf("and %d more", len(matched)-3))
			break
		}
		found = append(found, fmt.Sprintf("%q", labels[index]))
	}

	if query == "" {
		return nil, fmt.Errorf(
			"there is nobody to ask which of %d recent entries to continue - give a query that names one: %s",
			len(matched), strings.Join(found, ", "))
	}

	return nil, fmt.Errorf("%d recent entries match %q - narrow it down: %s",
		len(matched), query, strings.Join(found, ", "))
}

// recentChoice is one row of the listing: what the work was, and when it was
// last done - the second is how you tell two days of the same thing apart.
func recentChoice(entry *toggl.TimeEntry, label string, cfg *workingon.Config) string {
	if entry.Start == nil {
		return label
	}

	when := entry.Start.In(&cfg.Settings.Location).Format(cfg.Settings.DateTimeLayout)

	return fmt.Sprintf("%s  (%s)", label, when)
}

func countOfEntries(count int) string {
	if count == 1 {
		return "1 thing"
	}
	return fmt.Sprintf("%d things", count)
}
