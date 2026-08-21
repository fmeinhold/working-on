package workingon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
)

// modifyServer answers the lookup with one entry and records what was sent
// back, which is the only way to tell a modify that left a field alone from
// one that reset it to the same value by luck.
type modifyServer struct {
	*httptest.Server
	saved   *toggl.TimeEntry
	updates int
}

func newModifyServer(t *testing.T, entry string) *modifyServer {
	t.Helper()

	recorder := &modifyServer{}
	recorder.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			recorder.updates++

			var sent toggl.TimeEntry
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				t.Errorf("decoding the update: %v", err)
			}
			recorder.saved = &sent

			out, _ := json.Marshal(sent)
			w.Write(out)
			return
		}

		fmt.Fprint(w, entry)
	}))
	t.Cleanup(recorder.Close)

	return recorder
}

func (m *modifyServer) client() *toggl.Toggl {
	return toggl.NewTogglAt("test-token", m.URL)
}

// A finished entry, 09:00 to 10:30 on a project with a task.
const trackedEntry = `{"id":7,"description":"parser review","workspace_id":1562374,
	"project_id":188362780,"task_id":87708632,"start":"2026-08-17T09:00:00Z",
	"stop":"2026-08-17T10:30:00Z","duration":5400}`

const runningEntry = `{"id":8,"description":"parser review","workspace_id":1562374,
	"project_id":188362780,"task_id":87708632,"start":"2026-08-17T09:00:00Z","duration":-1}`

func moment(t *testing.T, value string) *time.Time {
	t.Helper()

	moment, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return &moment
}

// The whole contract: what was not asked about comes back as it was. v9 takes
// the entire entry on a PUT, so every field not named here is one this could
// have quietly dropped.
func TestModifyLeavesUnmentionedFieldsAlone(t *testing.T) {
	server := newModifyServer(t, trackedEntry)

	change, err := modify(server.client(), &Config{}, ModifyRequest{
		Id:   7,
		Stop: moment(t, "2026-08-17T11:00:00Z"),
	})
	if err != nil {
		t.Fatalf("modify: %v", err)
	}

	sent := server.saved
	if sent == nil {
		t.Fatal("nothing was sent to toggl")
	}

	if sent.Description != "parser review" {
		t.Errorf("description = %q, want it untouched", sent.Description)
	}
	if sent.ProjectId != 188362780 || sent.TaskId != 87708632 {
		t.Errorf("project/task = %d/%d, want them untouched", sent.ProjectId, sent.TaskId)
	}
	if !sent.Start.Equal(*moment(t, "2026-08-17T09:00:00Z")) {
		t.Errorf("start = %s, want it untouched", sent.Start)
	}
	if change.Note() != "stop" {
		t.Errorf("notes = %q, want just the stop", change.Note())
	}
}

// The length is sent alongside the two ends, so it has to be worked out again
// or toggl is told a duration that contradicts them.
func TestModifyKeepsTheLengthInStepWithTheEnds(t *testing.T) {
	for name, tc := range map[string]struct {
		req  ModifyRequest
		want int64
	}{
		"a later stop makes it longer": {
			req:  ModifyRequest{Id: 7, Stop: moment(t, "2026-08-17T11:00:00Z")},
			want: 7200,
		},
		"an earlier start makes it longer": {
			req:  ModifyRequest{Id: 7, Start: moment(t, "2026-08-17T08:30:00Z")},
			want: 7200,
		},
		"both at once": {
			req: ModifyRequest{Id: 7,
				Start: moment(t, "2026-08-17T09:30:00Z"), Stop: moment(t, "2026-08-17T10:00:00Z")},
			want: 1800,
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := newModifyServer(t, trackedEntry)

			if _, err := modify(server.client(), &Config{}, tc.req); err != nil {
				t.Fatalf("modify: %v", err)
			}

			if server.saved.Duration != tc.want {
				t.Errorf("duration = %d, want %d", server.saved.Duration, tc.want)
			}
		})
	}
}

// Moving a start is a statement about when work began. Sliding the stop along
// with it would keep a length nobody asked to keep.
func TestModifyingTheStartLeavesTheStopWhereItWas(t *testing.T) {
	server := newModifyServer(t, trackedEntry)

	if _, err := modify(server.client(), &Config{}, ModifyRequest{
		Id:    7,
		Start: moment(t, "2026-08-17T08:00:00Z"),
	}); err != nil {
		t.Fatalf("modify: %v", err)
	}

	if !server.saved.Stop.Equal(*moment(t, "2026-08-17T10:30:00Z")) {
		t.Errorf("stop = %s, want it left at 10:30", server.saved.Stop)
	}
}

// A task belongs to the project it was made in, so it cannot follow the entry
// to another one. Left attached it would file the entry under a task from a
// project it is no longer in.
func TestModifyClearsATaskThatCannotFollowTheProject(t *testing.T) {
	server := newModifyServer(t, trackedEntry)

	change, err := modify(server.client(), &Config{}, ModifyRequest{Id: 7, Project: "178178172"})
	if err != nil {
		t.Fatalf("modify: %v", err)
	}

	if server.saved.ProjectId != 178178172 {
		t.Errorf("project = %d, want the new one", server.saved.ProjectId)
	}
	if server.saved.TaskId != 0 {
		t.Errorf("task = %d, want it cleared", server.saved.TaskId)
	}
	if !strings.Contains(change.Note(), "task cleared") {
		t.Errorf("notes = %q, want them to say the task was cleared", change.Note())
	}
}

// Naming a task in the same breath is saying where it should land instead, so
// there is nothing to clear.
func TestModifyKeepsATaskGivenWithTheProject(t *testing.T) {
	server := newModifyServer(t, trackedEntry)

	if _, err := modify(server.client(), &Config{}, ModifyRequest{
		Id: 7, Project: "178178172", Task: "77728379",
	}); err != nil {
		t.Fatalf("modify: %v", err)
	}

	if server.saved.TaskId != 77728379 {
		t.Errorf("task = %d, want the one that was named", server.saved.TaskId)
	}
}

// Giving a running timer an end is how you correct a day you forgot to stop.
func TestModifyStopsARunningEntry(t *testing.T) {
	server := newModifyServer(t, runningEntry)

	change, err := modify(server.client(), &Config{}, ModifyRequest{
		Id:   8,
		Stop: moment(t, "2026-08-17T17:30:00Z"),
	})
	if err != nil {
		t.Fatalf("modify: %v", err)
	}

	if server.saved.Duration != 30600 {
		t.Errorf("duration = %d, want the 8h30m it ran", server.saved.Duration)
	}
	if server.saved.IsRunning() {
		t.Error("the entry is still running")
	}
	if change.Note() != "stopped" {
		t.Errorf("notes = %q, want it to say the entry was stopped", change.Note())
	}
}

// One that keeps running keeps its negative duration and no stop - the two are
// how toggl tells a running entry from a finished one.
func TestModifyLeavesARunningEntryRunning(t *testing.T) {
	server := newModifyServer(t, runningEntry)

	if _, err := modify(server.client(), &Config{}, ModifyRequest{
		Id:    8,
		Start: moment(t, "2026-08-17T08:00:00Z"),
	}); err != nil {
		t.Fatalf("modify: %v", err)
	}

	if server.saved.Stop != nil {
		t.Errorf("stop = %s, want none while it runs", server.saved.Stop)
	}
	if !server.saved.IsRunning() {
		t.Errorf("duration = %d, want it still negative", server.saved.Duration)
	}
}

func TestModifyRefusesAnEntryThatWouldRunBackwards(t *testing.T) {
	server := newModifyServer(t, trackedEntry)

	_, err := modify(server.client(), &Config{}, ModifyRequest{
		Id:   7,
		Stop: moment(t, "2026-08-17T08:00:00Z"),
	})

	if err == nil {
		t.Fatal("a stop before the start was accepted")
	}
	if !strings.Contains(err.Error(), "before it starts") {
		t.Errorf("error = %q, want it to say the entry cannot stop before it starts", err)
	}
	if server.updates != 0 {
		t.Error("the entry was saved anyway")
	}
}

// A modify that changes nothing is a mistake worth reporting, not a PUT worth
// making.
func TestModifyRefusesAChangeThatChangesNothing(t *testing.T) {
	for name, req := range map[string]ModifyRequest{
		"nothing was given":     {Id: 7},
		"the same values again": {Id: 7, Description: "parser review", Project: "188362780"},
	} {
		t.Run(name, func(t *testing.T) {
			server := newModifyServer(t, trackedEntry)

			_, err := modify(server.client(), &Config{}, req)
			if err == nil {
				t.Fatal("a no-op modify was accepted")
			}
			if !strings.Contains(err.Error(), "nothing to change") {
				t.Errorf("error = %q, want it to say there is nothing to change", err)
			}
			if server.updates != 0 {
				t.Error("something was saved anyway")
			}
		})
	}
}

func TestModifyDryRunSavesNothing(t *testing.T) {
	server := newModifyServer(t, trackedEntry)

	change, err := modify(server.client(), &Config{}, ModifyRequest{
		Id:     7,
		Stop:   moment(t, "2026-08-17T11:00:00Z"),
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("modify: %v", err)
	}

	if server.updates != 0 {
		t.Error("a dry run reached toggl")
	}
	if !change.After.Stop.Equal(*moment(t, "2026-08-17T11:00:00Z")) {
		t.Errorf("stop = %s, want the change it would have made", change.After.Stop)
	}
	if change.Before.Stop.Equal(*change.After.Stop) {
		t.Error("before and after are the same entry")
	}
}

// Nothing running and nothing tracked is an answer, not a crash.
func TestEntryToModifyWithNothingToGoOn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/current") {
			fmt.Fprint(w, "null")
			return
		}
		fmt.Fprint(w, "[]")
	}))
	defer server.Close()

	_, err := EntryToModifyFrom(toggl.NewTogglAt("test-token", server.URL), 0)

	if err == nil {
		t.Fatal("an entry was found where there is none")
	}
	if !strings.Contains(err.Error(), "no entry to modify") {
		t.Errorf("error = %q, want it to say there is nothing to modify", err)
	}
}

// The running timer is what "the entry" means while one is running, whatever
// was tracked before it.
func TestEntryToModifyPrefersTheRunningTimer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/current") {
			fmt.Fprint(w, runningEntry)
			return
		}
		fmt.Fprint(w, "["+trackedEntry+"]")
	}))
	defer server.Close()

	entry, err := EntryToModifyFrom(toggl.NewTogglAt("test-token", server.URL), 0)
	if err != nil {
		t.Fatal(err)
	}

	if entry.Id != 8 {
		t.Errorf("entry = %d, want the running one", entry.Id)
	}
}

// An error is output too. Told the entry cannot stop before it starts, the
// person reading has to recognise the times in it as the ones they typed.
func TestModifyReportsTimesTheWayTheUserReadsThem(t *testing.T) {
	server := newModifyServer(t, trackedEntry)

	cfg := &Config{}
	cfg.Settings.DateTimeLayout = "2.1.2006 15:04"
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Settings.Location = *berlin

	_, err = modify(server.client(), cfg, ModifyRequest{
		Id:   7,
		Stop: moment(t, "2026-08-17T08:00:00Z"),
	})
	if err == nil {
		t.Fatal("a stop before the start was accepted")
	}

	// The entry runs 09:00 to 10:30 UTC, which is 11:00 to 12:30 in Berlin.
	if !strings.Contains(err.Error(), "17.8.2026 11:00 to 17.8.2026 10:00") {
		t.Errorf("error = %q, want the times in the user's zone and layout", err)
	}
	if strings.Contains(err.Error(), "Z") || strings.Contains(err.Error(), "+02:00") {
		t.Errorf("error = %q, want no technical timestamps in it", err)
	}
}
