package workingon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/fefeme/workingon/toggl"
)

const (
	// taskCacheVersion is bumped whenever the on-disk layout changes, so an
	// older file is discarded rather than misread.
	taskCacheVersion = 1

	// taskCacheMaxAge is how stale the watermark may get before the cache is
	// rebuilt from scratch instead of topped up. The API rejects a `since`
	// older than three months, so this leaves a wide margin.
	taskCacheMaxAge = 30 * 24 * time.Hour
)

type taskCacheData struct {
	Version     int          `json:"version"`
	WorkspaceId int          `json:"workspace_id"`
	Account     string       `json:"account"`
	SyncedAt    time.Time    `json:"synced_at"`
	Tasks       []toggl.Task `json:"tasks"`
}

// TaskCache is a local mirror of a workspace's tasks.
//
// v9 has no id-only task route, so resolving a single task means walking the
// whole workspace listing - eight requests for a workspace of any size, on
// every `wo add`. The cache turns that into a map lookup, kept current with
// cheap `since` deltas rather than periodic full rebuilds.
type TaskCache struct {
	client      *toggl.Toggl
	workspaceId int
	account     string
	path        string

	data taskCacheData
	byId map[int]*toggl.Task

	loaded bool
	// synced guards against refreshing more than once per process, so a run
	// that asks for several unknown ids does not resync for each of them.
	synced bool
}

func NewTaskCache(client *toggl.Toggl, workspaceId int, apiToken string) *TaskCache {
	cache := &TaskCache{
		client:      client,
		workspaceId: workspaceId,
		account:     accountFingerprint(apiToken),
	}

	// A cache we cannot place on disk is not fatal: reads simply fall back to
	// hitting the API every time, which is what happened before it existed.
	if path, err := taskCachePath(workspaceId); err == nil {
		cache.path = path
	}

	return cache
}

// Tasks returns every task in the workspace, bringing the cache up to date
// first. The delta is small enough that a listing command can always pay it.
func (c *TaskCache) Tasks() ([]toggl.Task, error) {
	if err := c.sync(); err != nil {
		return nil, err
	}
	return c.data.Tasks, nil
}

// Find returns a task by id.
//
// A miss is treated as "possibly stale" rather than "no such task": the cache
// refreshes and looks again before giving up, so a task created moments ago
// resolves without the user needing to know a cache exists.
func (c *TaskCache) Find(id int) (*toggl.Task, error) {
	c.load()

	if task, ok := c.byId[id]; ok {
		return task, nil
	}

	if err := c.sync(); err != nil {
		return nil, err
	}

	if task, ok := c.byId[id]; ok {
		return task, nil
	}

	return nil, fmt.Errorf("%w: task %d in workspace %d", ErrTaskNotFound, id, c.workspaceId)
}

// Refresh discards what is cached and rebuilds it from scratch.
func (c *TaskCache) Refresh() error {
	c.data = taskCacheData{}
	c.byId = nil
	c.loaded = true
	c.synced = false

	return c.sync()
}

// Clear removes the cache file.
func (c *TaskCache) Clear() error {
	c.data = taskCacheData{}
	c.byId = nil
	c.loaded = true
	c.synced = false

	if c.path == "" {
		return nil
	}
	if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// sync brings the cache up to date, as a delta where possible.
func (c *TaskCache) sync() error {
	if c.synced {
		return nil
	}
	c.load()

	// Take the watermark before the request: anything that changes while the
	// fetch is in flight is then picked up next time rather than missed. The
	// small overlap is harmless because merging is idempotent.
	watermark := time.Now()

	if c.needsRebuild() {
		if err := c.rebuild(); err != nil {
			return err
		}
	} else if err := c.applyDelta(c.data.SyncedAt); err != nil {
		// A rejected or failed delta most likely means we drifted outside the
		// three month window; fall back to a full rebuild rather than serving
		// something we know is stale.
		if err := c.rebuild(); err != nil {
			return err
		}
	}

	c.data.Version = taskCacheVersion
	c.data.WorkspaceId = c.workspaceId
	c.data.Account = c.account
	c.data.SyncedAt = watermark
	c.synced = true

	c.reindex()

	return c.save()
}

func (c *TaskCache) needsRebuild() bool {
	return c.data.SyncedAt.IsZero() ||
		len(c.data.Tasks) == 0 ||
		time.Since(c.data.SyncedAt) > taskCacheMaxAge
}

func (c *TaskCache) rebuild() error {
	list, err := c.client.TaskClient.List(c.workspaceId)
	if err != nil {
		return err
	}

	c.data.Tasks = list.Tasks
	return nil
}

func (c *TaskCache) applyDelta(since time.Time) error {
	list, err := c.client.TaskClient.ListSince(c.workspaceId, since)
	if err != nil {
		return err
	}

	c.merge(list.Tasks)
	return nil
}

// merge applies a delta: deleted tasks drop out, everything else is upserted.
func (c *TaskCache) merge(delta []toggl.Task) {
	if len(delta) == 0 {
		return
	}

	byId := make(map[int]toggl.Task, len(c.data.Tasks)+len(delta))
	for _, task := range c.data.Tasks {
		byId[task.Id] = task
	}

	for _, task := range delta {
		if task.IsDeleted() {
			delete(byId, task.Id)
			continue
		}
		byId[task.Id] = task
	}

	tasks := make([]toggl.Task, 0, len(byId))
	for _, task := range byId {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Id < tasks[j].Id })

	c.data.Tasks = tasks
}

func (c *TaskCache) reindex() {
	c.byId = make(map[int]*toggl.Task, len(c.data.Tasks))
	for i := range c.data.Tasks {
		c.byId[c.data.Tasks[i].Id] = &c.data.Tasks[i]
	}
}

// load reads the cache file. Anything unreadable, corrupt or belonging to a
// different account or layout is treated as an empty cache and rebuilt, since
// a wrong answer is worse than a slow one.
func (c *TaskCache) load() {
	if c.loaded {
		return
	}
	c.loaded = true

	if c.path == "" {
		return
	}

	raw, err := os.ReadFile(c.path)
	if err != nil {
		return
	}

	var data taskCacheData
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}

	if data.Version != taskCacheVersion ||
		data.WorkspaceId != c.workspaceId ||
		data.Account != c.account {
		return
	}

	c.data = data
	c.reindex()
}

// save writes the cache out atomically, so a concurrent reader never sees a
// half-written file and an interrupted run cannot leave one behind.
func (c *TaskCache) save() error {
	if c.path == "" {
		return nil
	}

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	raw, err := json.Marshal(c.data)
	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(dir, "tasks-*.json.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temp.Name(), 0o600); err != nil {
		return err
	}

	return os.Rename(temp.Name(), c.path)
}

// taskCachePath is per workspace, so switching workspaces selects a different
// cache instead of invalidating the one already built.
func taskCachePath(workspaceId int) (string, error) {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".cache")
	}

	return filepath.Join(dir, "working-on", fmt.Sprintf("tasks-%d.json", workspaceId)), nil
}

// accountFingerprint identifies the credential a cache was built with, without
// storing the token itself.
func accountFingerprint(apiToken string) string {
	sum := sha256.Sum256([]byte(apiToken))
	return hex.EncodeToString(sum[:8])
}
