package workingon

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
)

// These exercise the real Toggl v9 API and are skipped unless WO_LIVE_TEST is
// set, so a plain `go test ./...` and CI stay hermetic:
//
//	WO_LIVE_TEST=1 go test ./workingon/ -run TestLive -v
//
// They are strictly read only - nothing here creates, modifies or stops a time
// entry. The token comes from the same config file the binary uses.
func liveClient(t *testing.T) (*toggl.Toggl, *Config) {
	t.Helper()

	if os.Getenv("WO_LIVE_TEST") == "" {
		t.Skip("set WO_LIVE_TEST=1 to run tests against the real Toggl API")
	}

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("unable to load config: %v", err)
	}
	if cfg.Settings.ToggleApiToken == "" || strings.HasPrefix(cfg.Settings.ToggleApiToken, "<") {
		t.Skip("no usable toggl api token in config")
	}

	return toggl.NewToggl(cfg.Settings.ToggleApiToken), cfg
}

// The v8 base url returns 404; this is the canary for the whole port.
func TestLiveApiReachable(t *testing.T) {
	client, cfg := liveClient(t)

	workspaces, err := client.WorkspaceClient.GetWorkspaces()
	if err != nil {
		t.Fatalf("GET /me/workspaces: %v", err)
	}
	if workspaces.Count == 0 {
		t.Fatal("no workspaces returned")
	}

	var found bool
	for _, workspace := range workspaces.Workspaces {
		if workspace.Id == cfg.Settings.ToggleWid {
			found = true
			t.Logf("workspace %d %q", workspace.Id, workspace.Name)
		}
	}
	if !found {
		t.Errorf("configured toggl_wid %d is not among the returned workspaces", cfg.Settings.ToggleWid)
	}
}

func TestLiveCurrentEntry(t *testing.T) {
	client, _ := liveClient(t)

	entry, err := client.TimeEntries.Current()
	if err != nil {
		t.Fatalf("GET /me/time_entries/current: %v", err)
	}

	if entry == nil {
		t.Log("no timer running")
		return
	}

	t.Logf("running: %q project=%d duration=%d", entry.Description, entry.ProjectId, entry.Duration)
	if !entry.IsRunning() {
		t.Errorf("duration = %d, want negative for a running entry", entry.Duration)
	}
	if entry.WorkspaceId == 0 {
		t.Error("workspace_id did not decode; v9 field names may have changed")
	}
}

func TestLiveListTodaysEntries(t *testing.T) {
	client, cfg := liveClient(t)

	year, month, day := time.Now().Date()
	start := time.Date(year, month, day, 0, 0, 0, 0, &cfg.Settings.Location)
	end := time.Now()

	list, err := client.TimeEntries.List(&start, &end)
	if err != nil {
		t.Fatalf("GET /me/time_entries: %v", err)
	}

	t.Logf("%d entries today", list.Count)
	for _, entry := range list.TimeEntries {
		if entry.Start == nil {
			t.Error("an entry decoded without a start time")
		}
	}
}

// Projects come back as a bare array while tasks use a paginated envelope;
// both must decode.
func TestLiveProjectsDecode(t *testing.T) {
	client, cfg := liveClient(t)

	projects, err := client.WorkspaceClient.ListProjects(cfg.Settings.ToggleWid)
	if err != nil {
		t.Fatalf("GET /workspaces/%d/projects: %v", cfg.Settings.ToggleWid, err)
	}
	if projects.Count == 0 {
		t.Skip("no projects in this workspace")
	}

	t.Logf("%d projects", projects.Count)
	for _, project := range projects.Projects {
		if project.Id == 0 || project.Name == "" {
			t.Errorf("project decoded with empty id or name: %+v", project)
			break
		}
	}
}

// The task endpoint pages at 50 by default. Without pagination this silently
// returns a fraction of a real workspace.
func TestLiveTasksArePaginatedInFull(t *testing.T) {
	client, cfg := liveClient(t)

	list, err := client.TaskClient.List(cfg.Settings.ToggleWid)
	if err != nil {
		t.Fatalf("GET /workspaces/%d/tasks: %v", cfg.Settings.ToggleWid, err)
	}

	t.Logf("%d tasks", list.Count)
	if list.Count == 50 {
		t.Error("got exactly 50 tasks - the default page size, so pagination is not being followed")
	}
	if list.Count == 0 {
		t.Skip("no tasks in this workspace")
	}

	// A task from the far end is the one an unpaginated walk would miss.
	last := list.Tasks[list.Count-1]
	found, err := client.TaskClient.Find(cfg.Settings.ToggleWid, last.Id)
	if err != nil {
		t.Fatalf("Find(%d) failed for a task in the listing: %v", last.Id, err)
	}
	if found.Id != last.Id {
		t.Errorf("Find returned task %d, want %d", found.Id, last.Id)
	}
}
