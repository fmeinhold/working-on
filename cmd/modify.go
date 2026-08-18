package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

func NewModifyCommand(cfg *workingon.Config) *cobra.Command {
	var (
		id          int
		start       string
		stop        string
		project     string
		task        string
		description string
		dry         bool
	)

	modifyCommand := &cobra.Command{
		Use:     "modify",
		Aliases: []string{"edit"},
		Short:   "Change an entry that is already there",
		Long: `Change an entry you have already tracked.

With no --id this is about the timer running now, or the last entry there was
where nothing is running. Any other entry is named by id, which
` + "`wo show <date> -l`" + ` lists:

  wo modify --stop 17:00
  wo modify --start 9:00 --stop 10:30
  wo modify --project "LaunchCycle 3.0" --task "05 Front End Development"
  wo modify --id 4520482208 --stop 17:30

What you leave out is left alone. A time is a time of day on the day the entry
belongs to, so --stop 17:00 needs no date; give one - "yesterday 9:00", or a
date in your configured layout - where the entry runs somewhere else. A --stop
that falls before the start is read as the following morning, which is what a
shift over midnight is.

Moving a start leaves the stop where it was, so the entry gets longer or
shorter rather than sliding along the day. A --stop on a running timer ends it
there.

A task belongs to its project, so changing the project without naming a task
leaves the entry without one rather than carrying a task across.`,
		Args: cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := workingon.EntryToModify(cfg, id)
			if err != nil {
				return err
			}

			req := workingon.ModifyRequest{
				Id:          entry.Id,
				Project:     project,
				Task:        task,
				Description: description,
				DryRun:      dry,
			}

			// The entry is what the times are read against: "17:00" is that
			// hour on the day this entry belongs to, not on today.
			on := entryDay(entry, &cfg.Settings.Location)

			if cmd.Flags().Changed("start") {
				at, err := parseMoment(cfg, start, on)
				if err != nil {
					return fmt.Errorf("--start %q: %w", start, err)
				}
				req.Start = &at
			}

			if cmd.Flags().Changed("stop") {
				at, err := parseMoment(cfg, stop, on)
				if err != nil {
					return fmt.Errorf("--stop %q: %w", stop, err)
				}

				// A shift that ran past midnight stops on the following day,
				// and saying so with a date every time would be tedious.
				if from := startingAt(req.Start, entry, &cfg.Settings.Location); !at.After(from) &&
					!hasDate(cfg, stop) {
					at = at.AddDate(0, 0, 1)
				}

				req.Stop = &at
			}

			change, err := workingon.Modify(cfg, req)
			if err != nil {
				return err
			}

			if jsonOutput {
				return emitChange(change, cfg, !dry)
			}

			fmt.Print(renderChange(change, cfg, nameResolver, dry))

			return nil
		},
	}

	modifyCommand.Flags().IntVar(&id, "id", 0,
		"The entry to change, by id (default the running one, or the last there was)")
	modifyCommand.Flags().StringVar(&start, "start", "", "Move the start, as \"9:00\"")
	modifyCommand.Flags().StringVarP(&stop, "stop", "s", "", "Move the stop, as \"17:00\"")
	modifyCommand.Flags().StringVarP(&project, "project", "p", "",
		"File it under another project, by id or by name")
	modifyCommand.Flags().StringVar(&task, "task", "", "Attach another task, by id or by name")
	modifyCommand.Flags().StringVarP(&description, "description", "m", "", "Say what it was about")
	modifyCommand.Flags().BoolVarP(&dry, "dry", "d", false, "Show what would change and stop")

	return modifyCommand
}

// entryDay is the day an entry belongs to, which is where a bare time of day
// lands. A running timer that began yesterday still belongs to yesterday.
func entryDay(entry *toggl.TimeEntry, loc *time.Location) time.Time {
	if entry == nil || entry.Start == nil {
		return time.Now().In(loc)
	}
	return entry.Start.In(loc)
}

// startingAt is the start a --stop is measured against: the new one where it
// was given in the same breath, the entry's own otherwise.
func startingAt(start *time.Time, entry *toggl.TimeEntry, loc *time.Location) time.Time {
	if start != nil {
		return start.In(loc)
	}
	return entryDay(entry, loc)
}

// parseMoment reads a flag that names a time, against the day the entry it
// belongs to already sits on.
//
// The pieces may come in any order and either may be left out, as everywhere
// else in wo: "17:00" keeps the entry's date, "yesterday" keeps its time of
// day, and "yesterday 17:00" says both.
func parseMoment(cfg *workingon.Config, spec string, on time.Time) (time.Time, error) {
	config := newParseArgsConfig(cfg)
	loc := config.location()
	on = on.In(loc)

	year, month, day := on.Date()
	clock := clockTime{hour: on.Hour(), minute: on.Minute()}

	for _, field := range strings.Fields(spec) {
		if at, matched, err := tryClock(field); matched {
			if err != nil {
				return time.Time{}, err
			}
			clock = at
			continue
		}

		if date, matched, err := tryDate(field, config); matched {
			if err != nil {
				return time.Time{}, err
			}
			year, month, day = date.Date()
			continue
		}

		return time.Time{}, fmt.Errorf(
			"%q is not a time - give one as \"17:00\", or with a day as \"yesterday 17:00\"", field)
	}

	return time.Date(year, month, day, clock.hour, clock.minute, 0, 0, loc), nil
}

// hasDate reports whether a spec said which day it meant. One that did is
// taken at its word, rather than being rolled forward over midnight.
func hasDate(cfg *workingon.Config, spec string) bool {
	config := newParseArgsConfig(cfg)

	for _, field := range strings.Fields(spec) {
		if _, matched, _ := tryDate(field, config); matched {
			return true
		}
	}

	return false
}

func renderChange(change *workingon.Change, cfg *workingon.Config,
	resolve func(*toggl.TimeEntry) entryNames, dry bool) string {

	before, after := change.Before, change.After
	loc := &cfg.Settings.Location

	// Times are read against the day the entry belongs to: a bare clock is
	// enough for that day, and anything that landed on another one has to say
	// so or a stop after midnight reads as an entry running backwards.
	show := momentTextOn(entryDay(&before, loc), loc, timeLayout(cfg), cfg.Settings.DateTimeLayout)

	heading := "Modified"
	if dry {
		heading = "Would modify"
	}

	out := fmt.Sprintf("%s  \"%s\"\n", heading, after.Description)
	if before.Description != after.Description {
		out = fmt.Sprintf("%s  \"%s\" -> \"%s\"\n", heading, before.Description, after.Description)
	}

	out += changedLine("start", show(before.Start), show(after.Start))
	out += changedLine("stop", show(before.Stop), show(after.Stop))
	out += changedLine("length", lengthText(&before), lengthText(&after))

	beforeNames, afterNames := resolve(&before), resolve(&after)
	out += changedLine("project", whereText(before.ProjectId, beforeNames.project),
		whereText(after.ProjectId, afterNames.project))
	out += changedLine("task", whereText(before.TaskId, beforeNames.task),
		whereText(after.TaskId, afterNames.task))

	return out
}

// Only the fields that moved are printed. A list of every field, most of them
// unchanged, would bury the one thing the reader is checking.
func changedLine(label, was, now string) string {
	if was == now {
		return ""
	}
	return fmt.Sprintf("  %-8s %s -> %s\n", label, was, now)
}

// momentTextOn writes times as a clock reading on the day the entry belongs
// to, and with the date on any other day.
func momentTextOn(day time.Time, loc *time.Location, clock, dated string) func(*time.Time) string {
	year, month, date := day.Date()

	return func(moment *time.Time) string {
		if moment == nil {
			return "still running"
		}

		at := moment.In(loc)
		if atYear, atMonth, atDate := at.Date(); atYear == year && atMonth == month && atDate == date {
			return at.Format(clock)
		}

		return at.Format(dated)
	}
}

func lengthText(entry *toggl.TimeEntry) string {
	if entry.IsRunning() {
		return "running"
	}
	return humanDuration(time.Duration(entry.Duration) * time.Second)
}

func whereText(id int, name string) string {
	switch {
	case id == 0:
		return "none"
	case name == "" || strings.HasPrefix(name, "#"):
		return strconv.Itoa(id)
	default:
		return fmt.Sprintf("%s (%d)", name, id)
	}
}

// The entry is given as it now stands with the one it replaced beside it, so a
// caller can report the change without having read the entry beforehand.
func emitChange(change *workingon.Change, cfg *workingon.Config, saved bool) error {
	before, after := change.Before, change.After

	return emit(struct {
		Action  string     `json:"action"`
		Entry   *entryJSON `json:"entry"`
		Was     *entryJSON `json:"was"`
		Changed []string   `json:"changed"`
		Saved   bool       `json:"saved"`
	}{"modified", entryWith(&after, cfg), entryWith(&before, cfg), change.Notes, saved})
}
