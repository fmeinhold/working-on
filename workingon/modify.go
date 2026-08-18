package workingon

import (
	"errors"
	"fmt"
	"time"

	"github.com/fefeme/workingon/toggl"
)

// ModifyRequest is a change to an entry that is already there.
//
// Every field is optional, and what is left out is left alone: an entry keeps
// whatever nobody asked about. That is the whole contract of a modify - one
// that quietly reset the fields you did not mention would be a rewrite, and
// these are hours somebody already worked.
type ModifyRequest struct {
	// Id names the entry to change. Zero means the timer running now, or the
	// last entry there was where nothing is running.
	Id int

	Start *time.Time
	Stop  *time.Time

	// Project and Task are an id or a name, read the same way as they are
	// everywhere else.
	Project string
	Task    string

	Description string

	DryRun bool
}

// Change is an entry as it was and as it would be, with a word on each field
// that moved.
//
// Both sides are carried because the caller has something to show rather than
// something to assert: "start 09:12 -> 09:00" can be checked by the person
// reading it, where "modified" cannot.
type Change struct {
	Before toggl.TimeEntry
	After  toggl.TimeEntry
	Notes  []string
}

func (c Change) Note() string {
	out := ""
	for i, note := range c.Notes {
		if i > 0 {
			out += ", "
		}
		out += note
	}
	return out
}

func Modify(cfg *Config, req ModifyRequest) (*Change, error) {
	return modify(toggl.NewToggl(cfg.Settings.ToggleApiToken), cfg, req)
}

func modify(client *toggl.Toggl, cfg *Config, req ModifyRequest) (*Change, error) {
	entry, err := EntryToModifyFrom(client, req.Id)
	if err != nil {
		return nil, err
	}

	after := *entry
	var notes []string

	if req.Description != "" && req.Description != after.Description {
		after.Description = req.Description
		notes = append(notes, "description")
	}

	if req.Project != "" {
		pid, err := resolveProject(cfg, req.Project)
		if err != nil {
			return nil, err
		}

		if pid != after.ProjectId {
			after.ProjectId = pid
			notes = append(notes, "project")

			// A task belongs to the project it was made in and cannot come
			// along to another one. Where no task was named to replace it,
			// leaving the entry without one is the only honest answer - the
			// alternative is an entry filed under a task from somewhere else.
			if req.Task == "" && after.TaskId != 0 {
				after.TaskId = 0
				notes = append(notes, "task cleared with the project")
			}
		}
	}

	if req.Task != "" {
		was := after.TaskId
		if err := applyTask(cfg, &after, req.Task); err != nil {
			return nil, err
		}
		if after.TaskId != was {
			notes = append(notes, "task")
		}
	}

	if req.Start != nil && !sameMoment(req.Start, after.Start) {
		start := *req.Start
		after.Start = &start
		notes = append(notes, "start")
	}

	if req.Stop != nil && !sameMoment(req.Stop, after.Stop) {
		stop := *req.Stop
		after.Stop = &stop

		// An entry that was running has been given an end, which is a thing
		// worth saying out loud rather than reporting as a changed field.
		if entry.IsRunning() {
			notes = append(notes, "stopped")
		} else {
			notes = append(notes, "stop")
		}
	}

	if err := settleDuration(&after); err != nil {
		return nil, err
	}

	if len(notes) == 0 {
		return nil, errors.New(
			"nothing to change - give a start, a stop, a project, a task or a description")
	}

	change := &Change{Before: *entry, After: after, Notes: notes}

	if req.DryRun {
		return change, nil
	}

	saved, err := client.TimeEntries.Update(&after)
	if err != nil {
		return nil, err
	}
	change.After = *saved

	return change, nil
}

// settleDuration keeps the length in step with the two ends, since toggl is
// told all three and would otherwise believe the stale one.
//
// Moving a start leaves the stop where it was: "start it at nine" is a
// statement about when work began, not a request to slide the whole entry an
// hour later.
func settleDuration(entry *toggl.TimeEntry) error {
	if entry.Start == nil || entry.Start.IsZero() {
		return errors.New("this entry has no start time, so there is nothing to measure from")
	}

	if entry.Stop == nil {
		entry.Duration = toggl.RunningDuration
		return nil
	}

	if !entry.Stop.After(*entry.Start) {
		return fmt.Errorf("an entry cannot stop at or before it starts - %s to %s",
			entry.Start.Format(time.RFC3339), entry.Stop.Format(time.RFC3339))
	}

	entry.Duration = int64(entry.Stop.Sub(*entry.Start).Seconds())

	return nil
}

func sameMoment(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// EntryToModify is the entry a modify with no id is about, and is exported so
// a caller can read the times off it before saying what to change them to: a
// stop of "17:00" means seventeen hundred on the day that entry belongs to,
// which cannot be worked out until the entry is in hand.
func EntryToModify(cfg *Config, id int) (*toggl.TimeEntry, error) {
	return EntryToModifyFrom(toggl.NewToggl(cfg.Settings.ToggleApiToken), id)
}

// The timer running now, or the last entry there was where nothing is running.
// Anything else has to be named by id: guessing which of a day's entries
// somebody meant is not a thing to do with a PUT.
func EntryToModifyFrom(client *toggl.Toggl, id int) (*toggl.TimeEntry, error) {
	if id != 0 {
		return client.TimeEntries.Find(id)
	}

	running, err := client.TimeEntries.Current()
	if err != nil {
		return nil, err
	}
	if running != nil {
		return running, nil
	}

	recent, err := client.TimeEntries.MostRecent()
	if err != nil {
		return nil, err
	}
	if recent == nil {
		return nil, errors.New(
			"there is no entry to modify - nothing is running, and nothing has been tracked")
	}

	return recent, nil
}
