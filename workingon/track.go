package workingon

import (
	"errors"
	"fmt"
	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/util"
	"github.com/spf13/cobra"
	"strconv"
	"time"
)

var (
	ErrorPidRequired        = errors.New("no project id found for toggl but TogglPidRequired is set to true")
	ErrorPidNotSetInMapping = errors.New("project pid not set in mapping")
)

// AppendStartTime is the moment the most recent time entry finished, for
// --append: the gap since you last stopped is attributed to the new entry
// rather than lost.
func AppendStartTime(cfg *Config) (time.Time, error) {
	return appendStartTimeFrom(toggl.NewToggl(cfg.Settings.ToggleApiToken))
}

func appendStartTimeFrom(client *toggl.Toggl) (time.Time, error) {
	previous, err := client.TimeEntries.MostRecent()
	if err != nil {
		return time.Time{}, err
	}

	if previous == nil {
		return time.Time{}, errors.New("there is no previous time entry to append to")
	}
	if previous.Stop == nil {
		// Nothing has finished, so there is no gap to fill.
		return time.Time{}, fmt.Errorf(
			"the most recent entry (%q) is still running - stop it before appending",
			previous.Description)
	}

	return *previous.Stop, nil
}

// ContinueLast opens a new running timer carrying the same description,
// project and task as the most recent entry.
//
// It starts a fresh entry rather than reopening the old one, so the earlier
// block of time keeps its own record.
func ContinueLast(cfg *Config, start time.Time, dryRun bool) (*toggl.TimeEntry, error) {
	client := toggl.NewToggl(cfg.Settings.ToggleApiToken)

	timeEntry, err := continuationOf(client)
	if err != nil {
		return nil, err
	}

	if start.IsZero() {
		start = time.Now()
	}
	timeEntry.Start = &start
	timeEntry.Duration = toggl.RunningDuration

	if err := timeEntry.Validate(); err != nil {
		return nil, err
	}

	if dryRun {
		return timeEntry, nil
	}

	return client.TimeEntries.Add(timeEntry)
}

// continuationOf builds an unstarted copy of the most recent entry.
func continuationOf(client *toggl.Toggl) (*toggl.TimeEntry, error) {
	previous, err := client.TimeEntries.MostRecent()
	if err != nil {
		return nil, err
	}

	if previous == nil {
		return nil, errors.New("there is no previous time entry to continue")
	}
	if previous.Stop == nil {
		return nil, fmt.Errorf("%q is already running", previous.Description)
	}

	return &toggl.TimeEntry{
		Description: previous.Description,
		WorkspaceId: previous.WorkspaceId,
		ProjectId:   previous.ProjectId,
		TaskId:      previous.TaskId,
		Billable:    previous.Billable,
		Tags:        previous.Tags,
		CreatedWith: toggl.CreatedWith,
	}, nil
}

// describeTask builds the time entry description for a resolved task.
//
// A toggl task key is just its numeric id, which the entry already links to
// via TaskId, so prefixing it only adds noise. Any other source keys its tasks
// meaningfully, and that key is worth carrying into the description.
func describeTask(task *Task) string {
	if task.TogglTask != 0 {
		return task.Summary
	}
	return fmt.Sprintf("%s: %s", task.Key, task.Summary)
}

func NewTimeEntry(cfg *Config, project string, wid int, summaryOrKey string, templateArgs map[string]string) (*toggl.TimeEntry, error) {
	var timeEntry *toggl.TimeEntry

	// Is this as key for a task in a source?
	task, err := Registry.GetTask(summaryOrKey)

	if task != nil {
		timeEntry = &toggl.TimeEntry{
			Description: describeTask(task),
			// A toggl-native task can be linked to directly. Sources that are
			// not toggl leave this zero.
			TaskId: task.TogglTask,
		}
	} else {
		// Maybe it's an alias for a template
		tpl, _ := Configuration.GetTemplate(summaryOrKey)
		switch {
		case tpl != nil:
			// We need to overwrite the startime and stoptime from the commandline
			// for example: wo add ds 21.01.2021 should work
			timeEntry, err = tpl.CreateTimeEntryFromTemplate(templateArgs)
			if err != nil {
				return nil, err
			}

		case err != nil && !errors.Is(err, ErrNoSourceClaimsKey):
			// A source recognised the key and could not resolve it. Quietly
			// booking that as a description would hide a typo'd issue key, or
			// an outage, behind an entry named after the key.
			return nil, err

		default:
			// It is just a Summary / Description
			timeEntry = &toggl.TimeEntry{
				Description: summaryOrKey,
			}
		}
	}

	if project != "" {
		// Overwrite Project pid with command line project parameter
		pid, err := strconv.Atoi(project)
		if err != nil {
			pm, err := cfg.GetMapping(project)
			if err == nil {
				pid = pm.TogglePid
			}
		}
		timeEntry.ProjectId = pid
	} else {
		if task != nil {
			pm, _ := Configuration.GetMapping(task.Project.Key)
			if pm != nil {
				if pm.TogglePid == 0 {
					return nil, ErrorPidNotSetInMapping
				}
				timeEntry.ProjectId = pm.TogglePid
			} else if task.Project.TogglProject != 0 {
				// A toggl-native task already belongs to a toggl project, so
				// there is nothing to map it through.
				timeEntry.ProjectId = task.Project.TogglProject
			}
		}
	}
	// Nothing has named a project yet, so fall back: the repository we are
	// standing in, then the configured default. Whether ending up with
	// neither is fatal is up to toggl_pid_required.
	if timeEntry.TaskId == 0 && timeEntry.ProjectId == 0 {
		pid := FindProjectByGitRepositoryUrl(cfg)
		if pid == 0 {
			pid = cfg.Settings.ToggleDefaultPid
		}
		if pid == 0 && cfg.Settings.TogglePidRequired {
			return nil, ErrorPidRequired
		}
		timeEntry.ProjectId = pid
	}

	timeEntry.WorkspaceId = wid
	timeEntry.CreatedWith = toggl.CreatedWith

	return timeEntry, nil
}

func setDuration(cfg *Config,
	timeEntry *toggl.TimeEntry, startTime time.Time, stopTime time.Time, duration time.Duration, running bool) error {

	now := time.Now()

	if !running {
		if timeEntry.Start == nil || timeEntry.Start.IsZero() {
			if startTime.IsZero() {
				return errors.New("no start time given")
			}
			timeEntry.Start = &startTime
		}
		if timeEntry.Stop == nil || timeEntry.Stop.IsZero() {
			if duration == 0 {
				if stopTime.IsZero() {
					return errors.New("no stop time or duration given")
				}
				timeEntry.Stop = &stopTime
			}
			timeEntry.Duration = int64(duration.Seconds())
		}

	} else {
		if startTime.IsZero() {
			timeEntry.Start = &now
		} else {
			timeEntry.Start = &startTime
		}
		// v9 flags a running entry with duration -1; v8 used the negative unix
		// start time. Stop must be absent while the timer runs.
		timeEntry.Duration = toggl.RunningDuration
		timeEntry.Stop = nil
	}

	err := timeEntry.Validate()

	return err

}

func AddOrStart(cmd *cobra.Command, cfg *Config,
	wid int, project string, summaryOrKey string,
	startTime time.Time, duration time.Duration,
	templateArgs map[string]string, running bool) (*toggl.TimeEntry, error) {

	timeEntry, err := NewTimeEntry(cfg, project, wid, summaryOrKey, templateArgs)
	if err != nil {
		return nil, fmt.Errorf("timeEntry: %s", err)
	}

	var stopTime time.Time
	s, err := cmd.Flags().GetString("stop")
	if err == nil && s != "" {
		stopTime, err = util.ParseTimeUTCE(s, cfg.Settings.DateLayout, cfg.Settings.DateTimeLayout, &cfg.Settings.Location)
		if err != nil {
			return nil, fmt.Errorf("unable to parse stop time: %s", err)
		}
	}

	err = setDuration(cfg, timeEntry, startTime, stopTime, duration, running)
	if err != nil {
		return nil, err
	}

	dryRun, _ := cmd.Flags().GetBool("dry")

	if !dryRun {
		cl := toggl.NewToggl(cfg.Settings.ToggleApiToken)
		timeEntry, err = cl.TimeEntries.Add(timeEntry)
		if err != nil {
			return nil, err
		}
	}

	return timeEntry, nil
}
