package workingon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
)

const testToken = "test-token"

// fakeWorkspace serves the paginated task listing, recording every request so
// tests can assert on how often - and with what `since` - the cache calls out.
type fakeWorkspace struct {
	tasks    map[int]toggl.Task
	requests []string
	sinces   []string
	fail     bool
}

func newFakeWorkspace(ids ...int) *fakeWorkspace {
	workspace := &fakeWorkspace{tasks: map[int]toggl.Task{}}
	for _, id := range ids {
		workspace.add(id, fmt.Sprintf("task %d", id))
	}
	return workspace
}

func (f *fakeWorkspace) add(id int, name string) {
	f.tasks[id] = toggl.Task{Id: id, Name: name, ProjectId: 91, WorkspaceId: 5, Active: true}
}

func (f *fakeWorkspace) delete(id int, at time.Time) {
	task := f.tasks[id]
	task.ServerDeletedAt = &at
	f.tasks[id] = task
}

// start returns a cache pointed at this fake, using dir for its cache file.
func (f *fakeWorkspace) start(t *testing.T, dir string) *TaskCache {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.fail {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "boom")
			return
		}

		since := r.URL.Query().Get("since")
		f.requests = append(f.requests, r.URL.Path)
		f.sinces = append(f.sinces, since)

		var rows []toggl.Task
		for _, task := range f.tasks {
			// A plain listing hides deleted tasks; only a `since` query
			// reports them, which is how the real API behaves.
			if task.IsDeleted() && since == "" {
				continue
			}
			rows = append(rows, task)
		}

		payload, err := json.Marshal(rows)
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Fprintf(w, `{"data":%s,"page":1,"per_page":1000,"total_count":%d}`, payload, len(rows))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("XDG_CACHE_HOME", dir)

	return NewTaskCache(toggl.NewTogglAt(testToken, srv.URL), 5, testToken)
}

func TestTaskCacheFirstUseFetchesEverythingAndPersists(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2, 3)
	cache := workspace.start(t, dir)

	tasks, err := cache.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(tasks))
	}
	if workspace.sinces[0] != "" {
		t.Errorf("first fetch sent since=%q; a cold cache must be a full rebuild", workspace.sinces[0])
	}

	path := filepath.Join(dir, "working-on", "tasks-5.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file was not written: %v", err)
	}
}

// A warm cache tops up with `since` rather than rebuilding.
func TestTaskCacheSecondRunUsesDelta(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2, 3)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	workspace.add(4, "brand new")

	second := workspace.start(t, dir)
	tasks, err := second.Tasks()
	if err != nil {
		t.Fatal(err)
	}

	if len(tasks) != 4 {
		t.Errorf("got %d tasks, want 4 after the delta", len(tasks))
	}
	if last := workspace.sinces[len(workspace.sinces)-1]; last == "" {
		t.Error("second run did not send a since parameter")
	}
}

// The case the whole design turns on: a task created after the cache was built
// still resolves, because a miss refreshes instead of failing.
func TestTaskCacheMissTriggersRefresh(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	workspace.add(99, "created moments ago")

	second := workspace.start(t, dir)
	found, err := second.Find(99)
	if err != nil {
		t.Fatalf("Find(99) failed even though a refresh would have found it: %v", err)
	}
	if found.Name != "created moments ago" {
		t.Errorf("got %q, want the new task", found.Name)
	}
}

// A cached task is served without any network call at all.
func TestTaskCacheHitDoesNotCallOut(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2, 3)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	second := workspace.start(t, dir)
	before := len(workspace.requests)

	if _, err := second.Find(2); err != nil {
		t.Fatal(err)
	}
	if len(workspace.requests) != before {
		t.Errorf("a cache hit made %d request(s); it should make none",
			len(workspace.requests)-before)
	}
}

// An id that genuinely does not exist refreshes once, then gives up.
func TestTaskCacheUnknownIdRefreshesOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	second := workspace.start(t, dir)
	before := len(workspace.requests)

	for i := 0; i < 3; i++ {
		if _, err := second.Find(999); err == nil {
			t.Fatal("expected an error for an unknown task id")
		}
	}

	if calls := len(workspace.requests) - before; calls != 1 {
		t.Errorf("made %d refreshes for repeated unknown lookups, want 1", calls)
	}
}

// A task deleted upstream must drop out of the cache, not linger forever.
func TestTaskCacheDeltaRemovesDeletedTasks(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2, 3)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	workspace.delete(2, time.Now())

	second := workspace.start(t, dir)
	tasks, err := second.Tasks()
	if err != nil {
		t.Fatal(err)
	}

	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2 after a deletion", len(tasks))
	}
	for _, task := range tasks {
		if task.Id == 2 {
			t.Error("deleted task 2 is still cached")
		}
	}
}

// Past the resync window the delta is abandoned in favour of a rebuild, so we
// never ask for a `since` the API would reject.
func TestTaskCacheRebuildsWhenWatermarkIsTooOld(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	agePersistedCache(t, dir, time.Now().Add(-taskCacheMaxAge-24*time.Hour))

	second := workspace.start(t, dir)
	if _, err := second.Tasks(); err != nil {
		t.Fatal(err)
	}

	if last := workspace.sinces[len(workspace.sinces)-1]; last != "" {
		t.Errorf("sent since=%q for a cache past the resync window; want a full rebuild", last)
	}
}

func TestTaskCacheRecoversFromCorruptFile(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2)

	path := filepath.Join(dir, "working-on", "tasks-5.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	cache := workspace.start(t, dir)
	tasks, err := cache.Tasks()
	if err != nil {
		t.Fatalf("a corrupt cache should rebuild, not fail: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2", len(tasks))
	}
}

// A cache built with a different credential must not be served.
func TestTaskCacheIgnoresAnotherAccountsFile(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2, 3)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	rewritePersistedCache(t, dir, func(data *taskCacheData) {
		data.Account = accountFingerprint("a-different-token")
	})

	second := workspace.start(t, dir)
	before := len(workspace.requests)

	if _, err := second.Find(2); err != nil {
		t.Fatal(err)
	}
	if len(workspace.requests) == before {
		t.Error("served a cache built with another token without refetching")
	}
}

func TestTaskCacheIgnoresOlderLayoutVersion(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2, 3)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	rewritePersistedCache(t, dir, func(data *taskCacheData) {
		data.Version = taskCacheVersion - 1
	})

	second := workspace.start(t, dir)
	before := len(workspace.requests)

	if _, err := second.Find(2); err != nil {
		t.Fatal(err)
	}
	if len(workspace.requests) == before {
		t.Error("served a cache written by an older layout")
	}
}

// Each workspace gets its own file, so alternating between them does not force
// a rebuild every time.
func TestTaskCachePathIsPerWorkspace(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/cache")

	first, err := taskCachePath(5)
	if err != nil {
		t.Fatal(err)
	}
	second, err := taskCachePath(6)
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatalf("both workspaces resolved to %s", first)
	}
	if want := "/tmp/cache/working-on/tasks-5.json"; first != want {
		t.Errorf("path = %s, want %s", first, want)
	}
}

func TestTaskCacheRefreshForcesFullRebuild(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	second := workspace.start(t, dir)
	if err := second.Refresh(); err != nil {
		t.Fatal(err)
	}

	if last := workspace.sinces[len(workspace.sinces)-1]; last != "" {
		t.Errorf("Refresh sent since=%q; it must rebuild from scratch", last)
	}
}

func TestTaskCacheClearRemovesFile(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2)

	cache := workspace.start(t, dir)
	if _, err := cache.Tasks(); err != nil {
		t.Fatal(err)
	}

	if err := cache.Clear(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "working-on", "tasks-5.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cache file still present after Clear: %v", err)
	}

	// Clearing twice is not an error.
	if err := cache.Clear(); err != nil {
		t.Errorf("second Clear returned %v", err)
	}
}

// A failing API surfaces as an error rather than an empty task list, which
// would read as "this task does not exist".
func TestTaskCacheReportsApiFailure(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2)
	workspace.fail = true

	cache := workspace.start(t, dir)
	if _, err := cache.Tasks(); err == nil {
		t.Fatal("expected an error when the API fails")
	}
}

// The cache file is written atomically, so it is either absent or complete.
func TestTaskCacheFileIsValidAndPrivate(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2, 3)

	cache := workspace.start(t, dir)
	if _, err := cache.Tasks(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "working-on", "tasks-5.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data taskCacheData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("cache file is not valid json: %v", err)
	}
	if data.Version != taskCacheVersion || data.WorkspaceId != 5 || len(data.Tasks) != 3 {
		t.Errorf("unexpected cache contents: %+v", data)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("cache file mode = %o, want 600", mode)
	}

	// No temporary files left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("left a temporary file behind: %s", entry.Name())
		}
	}
}

// A cold rebuild walks the workspace in several requests, so a single hiccup
// part way through used to lose the whole thing. The client's retry should
// absorb it.
func TestTaskCacheSurvivesTransientFailureDuringRebuild(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++

		// Fail the second request the way a flaky gateway would.
		if requests == 2 {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, "bad gateway")
			return
		}

		fmt.Fprint(w, `{"data":[{"id":1,"name":"task 1","workspace_id":5}],"page":1,"per_page":1000,"total_count":1}`)
	}))
	t.Cleanup(srv.Close)

	cache := NewTaskCache(toggl.NewTogglAt(testToken, srv.URL), 5, testToken)

	first, err := cache.Tasks()
	if err != nil {
		t.Fatalf("a transient 502 should have been retried: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("got %d tasks, want 1", len(first))
	}

	// The delta on the next run is the request that actually failed once.
	second := NewTaskCache(toggl.NewTogglAt(testToken, srv.URL), 5, testToken)
	if _, err := second.Tasks(); err != nil {
		t.Fatalf("delta sync failed: %v", err)
	}
	if requests < 3 {
		t.Errorf("server saw %d requests; the failure should have been retried", requests)
	}
}

// namedTaskServer serves a fixed set of tasks with names and projects.
func namedTaskServer(t *testing.T, dir string, rows string) (*TaskCache, *int) {
	t.Helper()

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprintf(w, `{"data":[%s],"page":1,"per_page":1000,"total_count":1}`, rows)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("XDG_CACHE_HOME", dir)

	return NewTaskCache(toggl.NewTogglAt(testToken, srv.URL), 5, testToken), &requests
}

const namedTasks = `
{"id":241929955,"name":"ATD Conference","project_id":91210706,"workspace_id":5},
{"id":77918943,"name":"000 Conferences","project_id":91210706,"workspace_id":5},
{"id":999,"name":"ATD Conference","project_id":164014679,"workspace_id":5}`

func TestTaskCacheFindByName(t *testing.T) {
	cache, _ := namedTaskServer(t, t.TempDir(), namedTasks)

	if _, err := cache.Tasks(); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		projectId int
		want      int
	}{
		{"ATD Conference", 91210706, 241929955},
		{"atd conference", 91210706, 241929955},
		{"ATD CONFERENCE", 91210706, 241929955},
		{"ATD Conference", 164014679, 999},
		{"000 Conferences", 91210706, 77918943},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s in %d", tc.name, tc.projectId), func(t *testing.T) {
			task := cache.LookupByName(tc.name, tc.projectId)
			if task == nil {
				t.Fatalf("LookupByName(%q, %d) found nothing", tc.name, tc.projectId)
			}
			if task.Id != tc.want {
				t.Errorf("got task %d, want %d", task.Id, tc.want)
			}
		})
	}
}

// The same name exists in two projects, so a workspace wide lookup cannot pick
// one and must decline rather than guess.
func TestTaskCacheDeclinesAnAmbiguousName(t *testing.T) {
	cache, _ := namedTaskServer(t, t.TempDir(), namedTasks)

	if _, err := cache.Tasks(); err != nil {
		t.Fatal(err)
	}

	if task := cache.LookupByName("ATD Conference", 0); task != nil {
		t.Errorf("got task %d for a name in two projects, want nothing", task.Id)
	}
}

func TestTaskCacheLookupByNameMisses(t *testing.T) {
	cache, _ := namedTaskServer(t, t.TempDir(), namedTasks)

	if _, err := cache.Tasks(); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"fixing the parser", "ATD", "Conference", ""} {
		if task := cache.LookupByName(name, 91210706); task != nil {
			t.Errorf("LookupByName(%q) matched task %d; the match must be exact", name, task.Id)
		}
	}
}

// LookupByName runs against every free text description, so paying a request
// to discover a description is not a task name would undo the cache.
func TestTaskCacheLookupByNameNeverCallsOut(t *testing.T) {
	dir := t.TempDir()
	cache, requests := namedTaskServer(t, dir, namedTasks)

	if _, err := cache.Tasks(); err != nil {
		t.Fatal(err)
	}

	warm := NewTaskCache(cache.client, 5, testToken)
	before := *requests

	for i := 0; i < 5; i++ {
		warm.LookupByName("definitely not a task name", 91210706)
	}

	if *requests != before {
		t.Errorf("made %d request(s) for cache-only lookups, want none", *requests-before)
	}
}

// FindByName is for a task the user named, so a miss is worth one refresh.
func TestTaskCacheFindByNameRefreshesOnMiss(t *testing.T) {
	dir := t.TempDir()
	cache, requests := namedTaskServer(t, dir, namedTasks)

	if _, err := cache.Tasks(); err != nil {
		t.Fatal(err)
	}

	warm := NewTaskCache(cache.client, 5, testToken)
	before := *requests

	if _, err := warm.FindByName("no such task", 91210706); err == nil {
		t.Fatal("expected an error for a name that does not exist")
	}
	if *requests == before {
		t.Error("FindByName did not refresh before giving up")
	}
}

// A cache that cannot be placed on disk still works, it just never persists.
func TestTaskCacheWithoutAPathStillServes(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1, 2)

	cache := workspace.start(t, dir)
	cache.path = ""

	tasks, err := cache.Tasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2", len(tasks))
	}
}

func persistedCachePath(dir string) string {
	return filepath.Join(dir, "working-on", "tasks-5.json")
}

func rewritePersistedCache(t *testing.T, dir string, edit func(*taskCacheData)) {
	t.Helper()

	path := persistedCachePath(dir)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data taskCacheData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}

	edit(&data)

	updated, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatal(err)
	}
}

func agePersistedCache(t *testing.T, dir string, syncedAt time.Time) {
	t.Helper()
	rewritePersistedCache(t, dir, func(data *taskCacheData) {
		data.SyncedAt = syncedAt
	})
}

// Guard the assumption the delta rests on: the watermark is sent as a unix
// timestamp, which is what the API expects.
func TestTaskCacheSendsUnixWatermark(t *testing.T) {
	dir := t.TempDir()
	workspace := newFakeWorkspace(1)

	first := workspace.start(t, dir)
	if _, err := first.Tasks(); err != nil {
		t.Fatal(err)
	}

	second := workspace.start(t, dir)
	if _, err := second.Tasks(); err != nil {
		t.Fatal(err)
	}

	last := workspace.sinces[len(workspace.sinces)-1]
	seconds, err := strconv.ParseInt(last, 10, 64)
	if err != nil {
		t.Fatalf("since=%q is not a unix timestamp: %v", last, err)
	}
	if age := time.Since(time.Unix(seconds, 0)); age < 0 || age > time.Hour {
		t.Errorf("watermark is %s old, want a recent timestamp", age)
	}
}
