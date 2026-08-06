package workingon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
)

// stubSource stands in for a real source so these tests never touch the network.
// It counts lookups, so tests can assert a source was never consulted.
type stubSource struct {
	name    string
	tasks   map[string]*Task
	names   map[string]*Task
	handles func(string) bool
	err     error
	calls   int
	lookups int
}

// LookupTaskByName and FindTaskByName make the stub a TaskNamer, matching a
// name within a project the way the toggl cache does.
func (s *stubSource) LookupTaskByName(name string, projectId int) *Task {
	s.lookups++

	task, ok := s.names[strings.ToLower(name)]
	if !ok {
		return nil
	}
	if projectId != 0 && task.Project.TogglProject != projectId {
		return nil
	}
	return task
}

func (s *stubSource) FindTaskByName(name string, projectId int) (*Task, error) {
	if task := s.LookupTaskByName(name, projectId); task != nil {
		return task, nil
	}
	return nil, fmt.Errorf("%w: no task named %q", ErrTaskNotFound, name)
}

func (s *stubSource) Configure(*Config) error         { return nil }
func (s *stubSource) GetName() string                 { return s.name }
func (s *stubSource) GetTasks() ([]Task, error)       { return nil, nil }
func (s *stubSource) GetProjects() ([]Project, error) { return nil, nil }

func (s *stubSource) Handles(key string) bool {
	if s.handles != nil {
		return s.handles(key)
	}
	// Without an explicit pattern, claim only what this source actually has,
	// so free text stays unclaimed the way it would in practice.
	_, known := s.tasks[key]
	return known
}

func (s *stubSource) GetTask(key string) (*Task, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if task, ok := s.tasks[key]; ok {
		return task, nil
	}
	// Real sources signal a clean miss this way, as distinct from a failure.
	return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, key)
}

// withSources swaps the global registry and config for the duration of a test.
func withSources(t *testing.T, cfg Config, sources ...Source) {
	t.Helper()

	registry, configuration := Registry.RegisteredSources, Configuration
	t.Cleanup(func() {
		Registry.RegisteredSources, Configuration = registry, configuration
	})

	Registry.RegisteredSources = sources
	Configuration = cfg
}

// A toggl-native task knows its own ids, so an entry built from one must link
// to the task and land on its project without needing a config mapping.
func TestNewTimeEntryLinksTogglTask(t *testing.T) {
	togglTask := &Task{
		Key:       "30422198",
		Summary:   "Testing",
		TogglTask: 30422198,
		Project:   Project{Key: "158249179", Name: "SW", TogglProject: 158249179},
	}

	cfg := Config{Settings: Settings{TogglePidRequired: true}}
	withSources(t, cfg, &stubSource{name: "toggl", tasks: map[string]*Task{"30422198": togglTask}})

	entry, err := NewTimeEntry(&cfg, "", 5, "30422198", nil, "")
	if err != nil {
		t.Fatalf("NewTimeEntry: %v", err)
	}

	if entry.TaskId != 30422198 {
		t.Errorf("task_id = %d, want 30422198", entry.TaskId)
	}
	if entry.ProjectId != 158249179 {
		t.Errorf("project_id = %d, want 158249179", entry.ProjectId)
	}
	// The numeric key adds nothing once the entry links to the task.
	if entry.Description != "Testing" {
		t.Errorf("description = %q, want %q", entry.Description, "Testing")
	}
}

// A task from a non-toggl source has no toggl ids of its own and resolves
// through the configured mapping, keeping its key in the description.
func TestNewTimeEntryMapsNonTogglTask(t *testing.T) {
	trackerTask := &Task{
		Key:     "MOET-297",
		Summary: "Fix the thing",
		Project: Project{Key: "MOET", Name: "Moet"},
	}

	cfg := Config{
		Settings: Settings{TogglePidRequired: true},
		Projects: []ProjectMapping{{Name: "MOET", TogglePid: 164014679}},
	}
	withSources(t, cfg, &stubSource{name: "tracker", tasks: map[string]*Task{"MOET-297": trackerTask}})

	entry, err := NewTimeEntry(&cfg, "", 5, "MOET-297", nil, "")
	if err != nil {
		t.Fatalf("NewTimeEntry: %v", err)
	}

	if entry.ProjectId != 164014679 {
		t.Errorf("project_id = %d, want 164014679 from the mapping", entry.ProjectId)
	}
	if entry.TaskId != 0 {
		t.Errorf("task_id = %d, want 0 for a non-toggl task", entry.TaskId)
	}
	if entry.Description != "MOET-297: Fix the thing" {
		t.Errorf("description = %q, want the issue key prefixed", entry.Description)
	}
}

// An explicit --project always wins over whatever the task says.
func TestNewTimeEntryProjectFlagOverridesTask(t *testing.T) {
	togglTask := &Task{
		Key:       "1",
		Summary:   "Testing",
		TogglTask: 1,
		Project:   Project{Key: "158249179", TogglProject: 158249179},
	}

	cfg := Config{Settings: Settings{TogglePidRequired: true}}
	withSources(t, cfg, &stubSource{name: "toggl", tasks: map[string]*Task{"1": togglTask}})

	entry, err := NewTimeEntry(&cfg, "999", 5, "1", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.ProjectId != 999 {
		t.Errorf("project_id = %d, want the flag value 999", entry.ProjectId)
	}
}

// A plain description still needs a project from somewhere.
func TestNewTimeEntryRequiresProjectForFreeText(t *testing.T) {
	cfg := Config{Settings: Settings{TogglePidRequired: true}}
	withSources(t, cfg, &stubSource{name: "toggl", handles: numericKey})

	_, err := NewTimeEntry(&cfg, "", 5, "some free text", nil, "")
	if err != ErrorPidRequired {
		t.Fatalf("err = %v, want ErrorPidRequired", err)
	}
}

// A key a source claimed but could not resolve must not quietly become the
// description - that hides a typo'd issue key behind an entry named after it.
func TestNewTimeEntryReportsAClaimedKeyThatFailedToResolve(t *testing.T) {
	cases := map[string]*stubSource{
		"source has no such task": {
			name:    "toggl",
			handles: numericKey,
			tasks:   map[string]*Task{},
		},
		"source could not answer": {
			name:    "tracker",
			handles: issueKey,
			err:     errors.New("connection refused"),
		},
	}

	keys := map[string]string{
		"source has no such task": "30422198",
		"source could not answer": "MOET-297",
	}

	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Settings: Settings{TogglePidRequired: true}}
			withSources(t, cfg, source)

			entry, err := NewTimeEntry(&cfg, "91", 5, keys[name], nil, "")
			if err == nil {
				t.Fatalf("got entry %+v, want an error rather than a description", entry)
			}
			if err == ErrorPidRequired {
				t.Fatal("reported a missing project; the lookup failure should surface instead")
			}
		})
	}
}

// A template alias still wins, even if a source happens to claim the same
// shape of key and fail on it.
func TestNewTimeEntryPrefersATemplateOverAFailedLookup(t *testing.T) {
	cfg := Config{
		Settings:  Settings{TogglePidRequired: true},
		Templates: []TemplateConfig{{Alias: "40819208", Description: "Daily Standup"}},
	}
	withSources(t, cfg, &stubSource{name: "toggl", handles: numericKey, tasks: map[string]*Task{}})

	entry, err := NewTimeEntry(&cfg, "91", 5, "40819208", nil, "")
	if err != nil {
		t.Fatalf("NewTimeEntry: %v", err)
	}
	if entry.Description != "Daily Standup" {
		t.Errorf("description = %q, want the template's", entry.Description)
	}
}

// toggl_default_pid is the last resort before giving up, so a description with
// no other project lands somewhere rather than being refused.
func TestNewTimeEntryFallsBackToTheDefaultProject(t *testing.T) {
	cfg := Config{Settings: Settings{TogglePidRequired: true, ToggleDefaultPid: 91210706}}
	withSources(t, cfg, &stubSource{name: "toggl", handles: numericKey})

	entry, err := NewTimeEntry(&cfg, "", 5, "some free text", nil, "")
	if err != nil {
		t.Fatalf("NewTimeEntry: %v", err)
	}
	if entry.ProjectId != 91210706 {
		t.Errorf("project_id = %d, want the default 91210706", entry.ProjectId)
	}
}

// The default is a floor, not a ceiling: anything more specific wins.
func TestNewTimeEntryPrefersASpecificProjectOverTheDefault(t *testing.T) {
	togglTask := &Task{
		Key:       "1",
		Summary:   "Testing",
		TogglTask: 1,
		Project:   Project{Key: "158249179", TogglProject: 158249179},
	}

	cfg := Config{Settings: Settings{TogglePidRequired: true, ToggleDefaultPid: 91210706}}
	withSources(t, cfg, &stubSource{name: "toggl", tasks: map[string]*Task{"1": togglTask}})

	t.Run("the --project flag", func(t *testing.T) {
		entry, err := NewTimeEntry(&cfg, "999", 5, "some free text", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if entry.ProjectId != 999 {
			t.Errorf("project_id = %d, want the flag's 999", entry.ProjectId)
		}
	})

	t.Run("the task's own project", func(t *testing.T) {
		entry, err := NewTimeEntry(&cfg, "", 5, "1", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if entry.ProjectId != 158249179 {
			t.Errorf("project_id = %d, want the task's 158249179", entry.ProjectId)
		}
	})
}

// Without a default, a description with no project is still refused when one
// is required.
func TestNewTimeEntryWithoutADefaultStillRequiresAProject(t *testing.T) {
	cfg := Config{Settings: Settings{TogglePidRequired: true}}
	withSources(t, cfg, &stubSource{name: "toggl", handles: numericKey})

	if _, err := NewTimeEntry(&cfg, "", 5, "some free text", nil, ""); err != ErrorPidRequired {
		t.Fatalf("err = %v, want ErrorPidRequired", err)
	}
}

// With no default and no requirement, an entry may simply have no project.
func TestNewTimeEntryAllowsNoProjectWhenNotRequired(t *testing.T) {
	cfg := Config{Settings: Settings{TogglePidRequired: false}}
	withSources(t, cfg, &stubSource{name: "toggl", handles: numericKey})

	entry, err := NewTimeEntry(&cfg, "", 5, "some free text", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.ProjectId != 0 {
		t.Errorf("project_id = %d, want none", entry.ProjectId)
	}
}

func TestNewTimeEntrySetsWorkspaceAndCreatedWith(t *testing.T) {
	cfg := Config{}
	withSources(t, cfg, &stubSource{name: "toggl", handles: numericKey})

	entry, err := NewTimeEntry(&cfg, "91", 1562374, "writing tests", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.WorkspaceId != 1562374 {
		t.Errorf("workspace_id = %d, want 1562374", entry.WorkspaceId)
	}
	if entry.CreatedWith != toggl.CreatedWith {
		t.Errorf("created_with = %q, want %q", entry.CreatedWith, toggl.CreatedWith)
	}
}

// atdConference is a task in project 91210706, the shape this repository's
// mapping resolves to.
func atdConference() *Task {
	return &Task{
		Key:       "241929955",
		Summary:   "ATD Conference",
		TogglTask: 241929955,
		Project:   Project{Key: "91210706", TogglProject: 91210706},
	}
}

func namingSource() *stubSource {
	return &stubSource{
		name:    "toggl",
		handles: numericKey,
		names:   map[string]*Task{"atd conference": atdConference()},
	}
}

// The name of a task in this project is a reference to it, not a description
// that happens to read the same way.
func TestNewTimeEntryResolvesATaskByName(t *testing.T) {
	cfg := Config{Settings: Settings{TogglePidRequired: true, ToggleDefaultPid: 91210706}}
	withSources(t, cfg, namingSource())

	entry, err := NewTimeEntry(&cfg, "", 5, "ATD Conference", nil, "")
	if err != nil {
		t.Fatalf("NewTimeEntry: %v", err)
	}

	if entry.TaskId != 241929955 {
		t.Errorf("task_id = %d, want 241929955", entry.TaskId)
	}
	if entry.ProjectId != 91210706 {
		t.Errorf("project_id = %d, want 91210706", entry.ProjectId)
	}
	if entry.Description != "ATD Conference" {
		t.Errorf("description = %q, want the task name", entry.Description)
	}
}

func TestNewTimeEntryMatchesATaskNameCaseInsensitively(t *testing.T) {
	cfg := Config{Settings: Settings{ToggleDefaultPid: 91210706}}
	withSources(t, cfg, namingSource())

	entry, err := NewTimeEntry(&cfg, "", 5, "atd conference", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.TaskId != 241929955 {
		t.Errorf("task_id = %d, want the task matched regardless of case", entry.TaskId)
	}
}

// Text that is not a task name stays a description.
func TestNewTimeEntryLeavesUnmatchedTextAsDescription(t *testing.T) {
	cfg := Config{Settings: Settings{ToggleDefaultPid: 91210706}}
	withSources(t, cfg, namingSource())

	entry, err := NewTimeEntry(&cfg, "", 5, "fixing the parser", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.TaskId != 0 {
		t.Errorf("task_id = %d, want none for free text", entry.TaskId)
	}
	if entry.Description != "fixing the parser" {
		t.Errorf("description = %q, want it left alone", entry.Description)
	}
}

// A name is only unambiguous inside a project, so the lookup is scoped to the
// one already resolved.
func TestNewTimeEntryScopesTaskNamesToTheProject(t *testing.T) {
	cfg := Config{Settings: Settings{ToggleDefaultPid: 164014679}}
	withSources(t, cfg, namingSource())

	entry, err := NewTimeEntry(&cfg, "", 5, "ATD Conference", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.TaskId != 0 {
		t.Errorf("task_id = %d, want none: the task is in another project", entry.TaskId)
	}
	if entry.Description != "ATD Conference" {
		t.Errorf("description = %q, want it treated as text", entry.Description)
	}
}

// --task takes an id.
func TestNewTimeEntryTaskFlagById(t *testing.T) {
	cfg := Config{Settings: Settings{ToggleDefaultPid: 91210706}}
	withSources(t, cfg, namingSource())

	entry, err := NewTimeEntry(&cfg, "", 5, "some work", nil, "241929955")
	if err != nil {
		t.Fatal(err)
	}
	if entry.TaskId != 241929955 {
		t.Errorf("task_id = %d, want 241929955", entry.TaskId)
	}
	if entry.Description != "some work" {
		t.Errorf("description = %q, want the description left alone", entry.Description)
	}
}

// --task also takes a name.
func TestNewTimeEntryTaskFlagByName(t *testing.T) {
	cfg := Config{Settings: Settings{ToggleDefaultPid: 91210706}}
	withSources(t, cfg, namingSource())

	entry, err := NewTimeEntry(&cfg, "", 5, "some work", nil, "ATD Conference")
	if err != nil {
		t.Fatal(err)
	}
	if entry.TaskId != 241929955 {
		t.Errorf("task_id = %d, want the named task", entry.TaskId)
	}
}

// A --task that resolves to nothing is an error, not a silent omission.
func TestNewTimeEntryTaskFlagReportsAnUnknownName(t *testing.T) {
	cfg := Config{Settings: Settings{ToggleDefaultPid: 91210706}}
	withSources(t, cfg, namingSource())

	_, err := NewTimeEntry(&cfg, "", 5, "some work", nil, "No Such Task")
	if err == nil {
		t.Fatal("expected an error for a task name that does not exist")
	}
	if !strings.Contains(err.Error(), "No Such Task") {
		t.Errorf("error = %q, want it to name the task", err)
	}
}

// A mapping may pin a task, so work in that repository lands on it without
// anything extra on the command line.
func TestNewTimeEntryUsesTheMappingsTask(t *testing.T) {
	cfg := Config{
		Settings: Settings{TogglePidRequired: true},
		Projects: []ProjectMapping{{Name: "SW", TogglePid: 91210706, TogglTask: 241929955}},
	}
	withSources(t, cfg, namingSource())

	entry, err := NewTimeEntry(&cfg, "SW", 5, "fixing the parser", nil, "")
	if err != nil {
		t.Fatalf("NewTimeEntry: %v", err)
	}

	if entry.ProjectId != 91210706 {
		t.Errorf("project_id = %d, want the mapping's", entry.ProjectId)
	}
	if entry.TaskId != 241929955 {
		t.Errorf("task_id = %d, want the mapping's task", entry.TaskId)
	}
}

// An explicit --task beats the one the mapping pins.
func TestNewTimeEntryTaskFlagOverridesTheMapping(t *testing.T) {
	cfg := Config{
		Settings: Settings{TogglePidRequired: true},
		Projects: []ProjectMapping{{Name: "SW", TogglePid: 91210706, TogglTask: 241929955}},
	}
	withSources(t, cfg, namingSource())

	entry, err := NewTimeEntry(&cfg, "SW", 5, "fixing the parser", nil, "77918943")
	if err != nil {
		t.Fatal(err)
	}
	if entry.TaskId != 77918943 {
		t.Errorf("task_id = %d, want the flag's 77918943", entry.TaskId)
	}
}

// A task resolved from the summary keeps its own task, not the mapping's.
func TestNewTimeEntryResolvedTaskBeatsTheMapping(t *testing.T) {
	cfg := Config{
		Settings: Settings{TogglePidRequired: true},
		Projects: []ProjectMapping{{Name: "SW", TogglePid: 91210706, TogglTask: 77918943}},
	}
	source := namingSource()
	source.tasks = map[string]*Task{"241929955": atdConference()}
	withSources(t, cfg, source)

	entry, err := NewTimeEntry(&cfg, "SW", 5, "241929955", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if entry.TaskId != 241929955 {
		t.Errorf("task_id = %d, want the resolved task", entry.TaskId)
	}
}

// --append picks up where the last entry stopped, so the gap since then is
// attributed to the new entry rather than lost.
func TestAppendStartTime(t *testing.T) {
	stop := time.Date(2026, 8, 6, 11, 30, 0, 0, time.UTC)

	cases := []struct {
		name    string
		body    string
		want    time.Time
		wantErr string
	}{
		{
			name: "uses the stop time of the most recent entry",
			body: `[{"id":1,"start":"2026-08-06T09:00:00Z","stop":"2026-08-06T10:00:00Z","duration":3600},
			        {"id":2,"start":"2026-08-06T10:30:00Z","stop":"2026-08-06T11:30:00Z","duration":3600}]`,
			want: stop,
		},
		{
			name: "picks the latest entry regardless of listing order",
			body: `[{"id":2,"start":"2026-08-06T10:30:00Z","stop":"2026-08-06T11:30:00Z","duration":3600},
			        {"id":1,"start":"2026-08-06T09:00:00Z","stop":"2026-08-06T10:00:00Z","duration":3600}]`,
			want: stop,
		},
		{
			name:    "refuses when nothing has been tracked yet",
			body:    `[]`,
			wantErr: "no previous time entry",
		},
		{
			name:    "refuses while a timer is still running",
			body:    `[{"id":3,"description":"still going","start":"2026-08-06T09:00:00Z","duration":-1}]`,
			wantErr: "still running",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()

			cfg := &Config{}
			cfg.Settings.ToggleApiToken = "test-token"

			got, err := appendStartTimeFrom(toggl.NewTogglAt(cfg.Settings.ToggleApiToken, srv.URL))

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("got %s, want an error mentioning %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %q, want it to mention %q", err, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("start = %s, want %s", got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

// Continuing carries the previous entry's identity onto a fresh timer.
func TestContinuationOfCopiesTheLastEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":7,"description":"05 Front End Development","workspace_id":1562374,
		                "project_id":188362780,"task_id":40819208,"billable":true,
		                "tags":["dev"],"start":"2026-08-06T09:00:00Z",
		                "stop":"2026-08-06T11:30:00Z","duration":9000}]`)
	}))
	defer srv.Close()

	entry, err := continuationOf(toggl.NewTogglAt("test-token", srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	if entry.Description != "05 Front End Development" {
		t.Errorf("description = %q", entry.Description)
	}
	if entry.WorkspaceId != 1562374 || entry.ProjectId != 188362780 || entry.TaskId != 40819208 {
		t.Errorf("identity not carried over: %+v", entry)
	}
	if !entry.Billable {
		t.Error("billable was not carried over")
	}
	if len(entry.Tags) != 1 || entry.Tags[0] != "dev" {
		t.Errorf("tags = %v, want [dev]", entry.Tags)
	}
	if entry.CreatedWith != toggl.CreatedWith {
		t.Errorf("created_with = %q", entry.CreatedWith)
	}

	// A fresh entry, not a reopened one: the old block keeps its record.
	if entry.Id != 0 {
		t.Errorf("id = %d, want a new entry rather than the old one", entry.Id)
	}
	if entry.Stop != nil {
		t.Errorf("stop = %v, want nil on a continuation", entry.Stop)
	}
}

func TestContinuationRefusesWhenThereIsNothingToContinue(t *testing.T) {
	cases := map[string]struct {
		body    string
		wantErr string
	}{
		"nothing tracked yet": {
			body:    `[]`,
			wantErr: "no previous time entry",
		},
		"already running": {
			body:    `[{"id":3,"description":"still going","start":"2026-08-06T09:00:00Z","duration":-1}]`,
			wantErr: "already running",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()

			_, err := continuationOf(toggl.NewTogglAt("test-token", srv.URL))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// v9 marks a running entry with -1; v8 used the negative unix start time, which
// v9 would read as an entry tens of thousands of hours long.
func TestSetDurationRunningEntry(t *testing.T) {
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	entry := &toggl.TimeEntry{Description: "live", WorkspaceId: 5}

	if err := setDuration(&Config{}, entry, start, time.Time{}, 0, true); err != nil {
		t.Fatal(err)
	}

	if entry.Duration != toggl.RunningDuration {
		t.Errorf("duration = %d, want %d", entry.Duration, toggl.RunningDuration)
	}
	if entry.Stop != nil {
		t.Error("stop must be nil while running")
	}

	body, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if _, present := wire["stop"]; present {
		t.Errorf("stop reached the wire while running: %s", body)
	}
}

func TestSetDurationCompletedEntry(t *testing.T) {
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)
	entry := &toggl.TimeEntry{Description: "done", WorkspaceId: 5}

	if err := setDuration(&Config{}, entry, start, stop, 0, false); err != nil {
		t.Fatal(err)
	}
	if entry.Duration != 5400 {
		t.Errorf("duration = %d, want 5400", entry.Duration)
	}
}

func TestSetDurationFromExplicitDuration(t *testing.T) {
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	entry := &toggl.TimeEntry{Description: "done", WorkspaceId: 5}

	if err := setDuration(&Config{}, entry, start, time.Time{}, 2*time.Hour, false); err != nil {
		t.Fatal(err)
	}
	if entry.Duration != 7200 {
		t.Errorf("duration = %d, want 7200", entry.Duration)
	}
}

func TestSetDurationNeedsStopOrDuration(t *testing.T) {
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	entry := &toggl.TimeEntry{Description: "incomplete", WorkspaceId: 5}

	if err := setDuration(&Config{}, entry, start, time.Time{}, 0, false); err == nil {
		t.Fatal("expected an error with neither a stop time nor a duration")
	}
}
