package workingon

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/util"
)

var (
	ErrorPidRequired  = errors.New("no project id found for toggl but TogglPidRequired is set to true")
	ErrorTaskRequired = errors.New("this workspace wants a task on every entry (toggl_task_required)")
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
func ContinueLast(cfg *Config, req EntryRequest) (*toggl.TimeEntry, error) {
	client := toggl.NewToggl(cfg.Settings.ToggleApiToken)

	timeEntry, err := continuationOf(client)
	if err != nil {
		return nil, err
	}

	start := req.Start
	if start.IsZero() {
		start = time.Now()
	}
	timeEntry.Start = &start
	timeEntry.Duration = toggl.RunningDuration

	if err := timeEntry.Validate(); err != nil {
		return nil, err
	}

	// The entry this copies may have had neither a task nor a description of
	// its own, and this one has to stand on its own feet.
	if err := requireTask(cfg, timeEntry, req); err != nil {
		return nil, err
	}

	if _, err := describe(timeEntry, req.Describe); err != nil {
		return nil, err
	}

	if req.DryRun {
		return timeEntry, nil
	}

	return client.TimeEntries.Add(timeEntry)
}

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

// A toggl task key is just its numeric id, which the entry already links to
// via TaskId, so prefixing it only adds noise. Any other source keys its tasks
// meaningfully, and that key is worth carrying into the description.
func describeTask(task *Task) string {
	if task.TogglTask != 0 {
		return task.Summary
	}
	return fmt.Sprintf("%s: %s", task.Key, task.Summary)
}

// TaskNamer is the optional ability of a source to resolve a task by name.
// Toggl task ids are unmemorable, so a name is how you actually reach one.
type TaskNamer interface {
	// LookupTaskByName answers from local state only. It runs against every
	// free text description, so it must never call out.
	LookupTaskByName(name string, projectId int) *Task

	// FindTaskByName may go to the source, for a task the user named
	// explicitly and therefore expects to exist.
	FindTaskByName(name string, projectId int) (*Task, error)
}

// lookupTaskByName asks the sources that can, without calling out.
func lookupTaskByName(name string, projectId int) *Task {
	if name == "" {
		return nil
	}

	for _, source := range Registry.RegisteredSources {
		namer, ok := source.(TaskNamer)
		if !ok {
			continue
		}
		if task := namer.LookupTaskByName(name, projectId); task != nil {
			return task
		}
	}

	return nil
}

func findTaskByName(name string, projectId int) (*Task, error) {
	var lastErr error

	for _, source := range Registry.RegisteredSources {
		namer, ok := source.(TaskNamer)
		if !ok {
			continue
		}
		task, err := namer.FindTaskByName(name, projectId)
		if err != nil {
			lastErr = err
			continue
		}
		if task != nil {
			return task, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%w: no task named %q", ErrTaskNotFound, name)
}

func applyTask(cfg *Config, timeEntry *toggl.TimeEntry, taskRef string) error {
	if taskRef != "" {
		// A --task wins over anything inferred, and may be an id or a name.
		if id, err := strconv.Atoi(taskRef); err == nil {
			timeEntry.TaskId = id
			return nil
		}

		task, err := findTaskByName(taskRef, timeEntry.ProjectId)
		if err != nil {
			return err
		}
		timeEntry.TaskId = task.TogglTask
		if timeEntry.ProjectId == 0 {
			timeEntry.ProjectId = task.Project.TogglProject
		}
		return nil
	}

	// The default task belongs to the default project, so it is only right for
	// an entry that landed in that project - anywhere else it would attach a
	// task from a project the entry is not in.
	if timeEntry.TaskId == 0 && cfg.Settings.ToggleDefaultTask != 0 &&
		timeEntry.ProjectId != 0 && timeEntry.ProjectId == cfg.Settings.ToggleDefaultPid {
		timeEntry.TaskId = cfg.Settings.ToggleDefaultTask
	}

	return nil
}

// Order: the --project flag, then the configured default - which a
// .workingon.yaml beside a checkout sets per repository. It runs before the
// task is resolved, because a task name is only unambiguous within a project.
//
// A --project is either an id or the name of a project the workspace has.
// An unresolvable one is an error rather than a silent fall back to the
// default: filing time under the wrong project is worse than not filing it.
func resolveProject(cfg *Config, project string) (int, error) {
	if project == "" {
		return cfg.Settings.ToggleDefaultPid, nil
	}

	if pid, err := strconv.Atoi(project); err == nil {
		return pid, nil
	}

	return FindProjectByName(project)
}

// CurrentProject is the project an entry started here and now would be filed
// under with no --project flag.
//
// It defers to resolveProject rather than repeating the precedence, so a
// listing that points at the current project cannot drift from the one an
// entry actually lands in.
func CurrentProject(cfg *Config) int {
	pid, _ := resolveProject(cfg, "")
	return pid
}

// resolveEntry turns the summary argument into a time entry: a task key, a
// template alias, a task name in this project, or plain description text.
func resolveEntry(summaryOrKey string, pid int, req EntryRequest) (*toggl.TimeEntry, error) {
	task, err := Registry.GetTask(summaryOrKey)
	if task != nil {
		return entryForTask(task), nil
	}

	if tpl, _ := Configuration.GetTemplate(summaryOrKey); tpl != nil {
		args, askErr := tpl.answerOpenArgs(req.TemplateArgs, req.AskTemplateArg)
		if askErr != nil {
			return nil, askErr
		}
		return tpl.CreateTimeEntryFromTemplate(args)
	}

	if err != nil && !errors.Is(err, ErrNoSourceClaimsKey) {
		// A source recognised the key and could not resolve it. Quietly
		// booking that as a description would hide a typo'd issue key, or
		// an outage, behind an entry named after the key.
		return nil, err
	}

	// The name of a task in this project is a reference to it, not a
	// description that happens to read the same way.
	if named := lookupTaskByName(summaryOrKey, pid); named != nil {
		return entryForTask(named), nil
	}

	return &toggl.TimeEntry{Description: summaryOrKey}, nil
}

// A task belongs to exactly one project, and toggl refuses an entry that files
// it under another - so of the two, only the task can be honoured. Whatever
// chose the project, be it the --project flag, a template's toggl_pid or the
// checkout's toggl_default_pid, the task the entry ended up with settles it.
//
// A task that cannot be looked up leaves the project alone: an entry that may
// be wrong still beats refusing to book time over a cold cache.
func settleProjectAgainstTask(timeEntry *toggl.TimeEntry) {
	if timeEntry.TaskId == 0 {
		return
	}

	if projectId := projectOfTask(timeEntry.TaskId); projectId != 0 {
		timeEntry.ProjectId = projectId
	}
}

// Zero when the task cannot be looked up.
func projectOfTask(taskId int) int {
	task, err := Registry.GetTask(strconv.Itoa(taskId))
	if err != nil || task == nil {
		return 0
	}
	return task.Project.TogglProject
}

func entryForTask(task *Task) *toggl.TimeEntry {
	return &toggl.TimeEntry{
		Description: describeTask(task),
		// A toggl-native task can be linked to directly, and already belongs to
		// a toggl project. A task from any other source has neither, so the
		// entry keeps whatever the flag or the default chose.
		TaskId:    task.TogglTask,
		ProjectId: task.Project.TogglProject,
	}
}

// NewTimeEntry is the plain form of newTimeEntry, for a caller holding the
// parts of an entry rather than a whole EntryRequest.
func NewTimeEntry(cfg *Config, project string, wid int, summaryOrKey string,
	templateArgs map[string]string, taskRef string) (*toggl.TimeEntry, error) {

	return newTimeEntry(cfg, EntryRequest{
		Wid:          wid,
		Project:      project,
		Task:         taskRef,
		SummaryOrKey: summaryOrKey,
		TemplateArgs: templateArgs,
	})
}

func newTimeEntry(cfg *Config, req EntryRequest) (*toggl.TimeEntry, error) {

	pid, err := resolveProject(cfg, req.Project)
	if err != nil {
		return nil, err
	}

	timeEntry, err := resolveEntry(req.SummaryOrKey, pid, req)
	if err != nil {
		return nil, err
	}

	switch {
	case req.Project != "":
		// An explicit --project wins over what the entry brought with it, for
		// an entry that carries no task to settle it below.
		timeEntry.ProjectId = pid
	case timeEntry.ProjectId == 0:
		timeEntry.ProjectId = pid
	}

	if err := applyTask(cfg, timeEntry, req.Task); err != nil {
		return nil, err
	}

	settleProjectAgainstTask(timeEntry)

	if timeEntry.TaskId == 0 && timeEntry.ProjectId == 0 && cfg.Settings.TogglePidRequired {
		return nil, ErrorPidRequired
	}

	timeEntry.WorkspaceId = req.Wid
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

// TaskChooser picks one of a project's tasks, returning nil to leave the entry
// without one. It is what makes a required task answerable rather than merely
// refused.
type TaskChooser func(projectId int, tasks []Task) (*Task, error)

// Only toggl-native tasks are offered: a task from another source is carried in
// the description, so there is nothing to attach and nothing that would satisfy
// a workspace asking for a task.
func TasksInProject(projectId int) ([]Task, error) {
	if projectId == 0 {
		return nil, nil
	}

	var found []Task

	for _, source := range Registry.RegisteredSources {
		tasks, err := source.GetTasks()
		if err != nil {
			return nil, err
		}

		for _, task := range tasks {
			if task.TogglTask != 0 && task.Project.TogglProject == projectId {
				found = append(found, task)
			}
		}
	}

	return found, nil
}

// requireTask settles the task of an entry that resolved to none: it is asked
// for when the workspace requires one, or when --pick-task asked to choose.
//
// Toggl refuses an entry that breaks the workspace's rule, and it refuses it
// after the fact - so the question is worth asking here, where the answer can
// still go into the entry.
func requireTask(cfg *Config, timeEntry *toggl.TimeEntry, req EntryRequest) error {
	required := cfg.Settings.ToggleTaskRequired

	// --pick-task asks even about a task that was inherited - from the default,
	// or from the entry being continued - since that is the only reason to pass
	// it. A --task names one outright, and settles it.
	asked := req.PickTask && req.Task == ""

	if !asked && (timeEntry.TaskId != 0 || !required) {
		return nil
	}

	// An entry that already has a task meets the rule whatever comes of the
	// asking, so from here on only a bare entry can fail.
	unmet := required && timeEntry.TaskId == 0

	if timeEntry.ProjectId == 0 {
		if !unmet {
			return nil
		}
		return fmt.Errorf("%w, and there is no project to take one from", ErrorTaskRequired)
	}

	tasks, err := TasksInProject(timeEntry.ProjectId)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		if !unmet {
			return nil
		}
		return fmt.Errorf("%w, but project %d has none", ErrorTaskRequired, timeEntry.ProjectId)
	}

	var chosen *Task
	if req.ChooseTask != nil {
		if chosen, err = req.ChooseTask(timeEntry.ProjectId, tasks); err != nil {
			return err
		}
	}

	if chosen == nil {
		if !unmet {
			return nil
		}
		return fmt.Errorf("%w - name one with --task, or run this where you can be asked",
			ErrorTaskRequired)
	}

	timeEntry.TaskId = chosen.TogglTask

	// An entry that was never given a summary is about the task that was just
	// chosen, exactly as it would be had the task been named as the summary.
	if timeEntry.Description == "" {
		timeEntry.Description = describeTask(chosen)
	}

	return nil
}

// EntryRequest is everything the commands can say about a new time entry.
type EntryRequest struct {
	Wid          int
	Project      string
	Task         string
	SummaryOrKey string
	Start        time.Time
	Stop         string
	Duration     time.Duration
	TemplateArgs map[string]string
	Running      bool
	DryRun       bool
	// Describe names an entry that has none. Leave it nil to let a nameless
	// entry through as it stands.
	Describe Describer
	// ChooseTask answers for an entry that resolved to no task, when the
	// workspace requires one or PickTask asked to choose.
	ChooseTask TaskChooser
	// AskTemplateArg answers for the placeholders in a template's description
	// that TemplateArgs left open. Leave it nil to render them as <no value>.
	AskTemplateArg TemplateArgAsker
	// PickTask asks for the task to be chosen even where the workspace does
	// not insist on one.
	PickTask bool
}

// Describer supplies a description for an entry that carries none, returning
// "" to leave it that way. It is given the entry so it can say which one it is
// asking about.
type Describer func(entry *toggl.TimeEntry) (string, error)

func describe(timeEntry *toggl.TimeEntry, describer Describer) (bool, error) {
	if describer == nil || timeEntry.Description != "" {
		return false, nil
	}

	description, err := describer(timeEntry)
	if err != nil || description == "" {
		return false, err
	}

	timeEntry.Description = description

	return true, nil
}

// Toggl saves the running entry as a side effect of starting or stopping one,
// and a workspace that requires descriptions refuses to save it without one.
// That refusal arrives as a failure of whatever you were trying to do, naming a
// requirement of an entry you were not thinking about - so settle it first.
func NameRunningEntry(cfg *Config, describer Describer) (*toggl.TimeEntry, error) {
	return nameRunningEntry(toggl.NewToggl(cfg.Settings.ToggleApiToken), describer)
}

func nameRunningEntry(client *toggl.Toggl, describer Describer) (*toggl.TimeEntry, error) {
	if describer == nil {
		return nil, nil
	}

	running, err := client.TimeEntries.Current()
	if err != nil || running == nil {
		return nil, err
	}

	named, err := describe(running, describer)
	if err != nil || !named {
		return nil, err
	}

	return client.TimeEntries.Update(running)
}

func AddOrStart(cfg *Config, req EntryRequest) (*toggl.TimeEntry, error) {
	timeEntry, err := newTimeEntry(cfg, req)
	if err != nil {
		return nil, fmt.Errorf("timeEntry: %s", err)
	}

	var stopTime time.Time
	if req.Stop != "" {
		stopTime, err = util.ParseTimeUTCE(req.Stop, cfg.Settings.DateLayout,
			cfg.Settings.DateTimeLayout, &cfg.Settings.Location)
		if err != nil {
			return nil, fmt.Errorf("unable to parse stop time: %s", err)
		}
	}

	err = setDuration(cfg, timeEntry, req.Start, stopTime, req.Duration, req.Running)
	if err != nil {
		return nil, err
	}

	// Before the description, so that choosing a task can answer for both.
	if err := requireTask(cfg, timeEntry, req); err != nil {
		return nil, err
	}

	if _, err := describe(timeEntry, req.Describe); err != nil {
		return nil, err
	}

	if req.DryRun {
		return timeEntry, nil
	}

	return createEntry(toggl.NewToggl(cfg.Settings.ToggleApiToken), timeEntry, req)
}

func createEntry(client *toggl.Toggl, timeEntry *toggl.TimeEntry, req EntryRequest) (*toggl.TimeEntry, error) {
	// Starting a timer stops whatever was running, so that entry has to be
	// acceptable to toggl before this one can be created - otherwise its
	// missing description is reported as a failure to start this one.
	if req.Running {
		if _, err := nameRunningEntry(client, req.Describe); err != nil {
			return nil, err
		}
	}

	return client.TimeEntries.Add(timeEntry)
}
