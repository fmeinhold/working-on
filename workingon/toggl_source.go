package workingon

import (
	"fmt"
	"strconv"

	"github.com/fefeme/workingon/toggl"
)

type TogglSource struct {
	client *toggl.Toggl
	wid    int
	cache  *TaskCache
}

func init() {
	err := Registry.Register(&TogglSource{})
	if err != nil {
		panic(err)
	}
}

func (t *TogglSource) GetName() string {
	return "toggl"
}

// Handles reports whether key is a toggl task id, which is always a positive
// integer.
func (t *TogglSource) Handles(key string) bool {
	id, err := strconv.Atoi(key)
	return err == nil && id > 0
}

func (t *TogglSource) GetTasks() ([]Task, error) {
	cached, err := t.cache.Tasks()
	if err != nil {
		return nil, err
	}

	var tasks []Task

	for i := range cached {
		tasks = append(tasks, *togglTask(&cached[i]))
	}

	return tasks, nil
}

func (t *TogglSource) GetProjects(includeArchived bool) ([]Project, error) {
	if t.client == nil {
		return nil, fmt.Errorf("toggl source is not configured")
	}

	projectList, err := t.client.WorkspaceClient.ListProjectsWhere(t.wid,
		toggl.ProjectQuery{ActiveOnly: !includeArchived})
	if err != nil {
		return nil, err
	}

	var projects []Project
	for _, project := range projectList.Projects {
		projects = append(projects, Project{
			Key:      strconv.Itoa(project.Id),
			Name:     project.Name,
			Archived: !project.Active,
		})
	}

	return projects, nil

}

func (t *TogglSource) Configure(cfg *Config) error {
	t.client = toggl.NewToggl(cfg.Settings.ToggleApiToken)
	t.wid = cfg.Settings.ToggleWid
	t.cache = NewTaskCache(t.client, t.wid, cfg.Settings.ToggleApiToken)
	return nil
}

// LookupTaskByName resolves a task name from the local cache only.
func (t *TogglSource) LookupTaskByName(name string, projectId int) *Task {
	if t.cache == nil {
		return nil
	}
	task := t.cache.LookupByName(name, projectId)
	if task == nil {
		return nil
	}
	return togglTask(task)
}

// FindTaskByName resolves a task name, refreshing the cache if it misses.
func (t *TogglSource) FindTaskByName(name string, projectId int) (*Task, error) {
	if t.cache == nil {
		return nil, fmt.Errorf("%w: no task named %q", ErrTaskNotFound, name)
	}

	task, err := t.cache.FindByName(name, projectId)
	if err != nil {
		return nil, err
	}
	return togglTask(task), nil
}

// togglTask converts a cached toggl task into the shape sources return.
func togglTask(task *toggl.Task) *Task {
	return &Task{
		Key:     strconv.Itoa(task.Id),
		Summary: task.Name,
		Project: Project{
			Key:          strconv.Itoa(task.ProjectId),
			Name:         task.ProjectName,
			TogglProject: task.ProjectId,
		},
		TogglTask: task.Id,
	}
}

// RefreshCache rebuilds the local task cache from scratch.
func (t *TogglSource) RefreshCache() error {
	if t.cache == nil {
		return nil
	}
	return t.cache.Refresh()
}

// ClearCache removes the local task cache.
func (t *TogglSource) ClearCache() error {
	if t.cache == nil {
		return nil
	}
	return t.cache.Clear()
}

// CachePath is where this source keeps its task cache, empty if it has none.
func (t *TogglSource) CachePath() string {
	if t.cache == nil {
		return ""
	}
	return t.cache.path
}

func (t *TogglSource) GetTask(key string) (*Task, error) {
	tid, err := strconv.Atoi(key)
	if err != nil {
		return nil, err
	}
	if t.cache == nil {
		return nil, fmt.Errorf("%w: task %s", ErrTaskNotFound, key)
	}
	// v9 addresses tasks under their project, so an id-only lookup has to go
	// through the workspace listing. The cache keeps that off the hot path.
	task, err := t.cache.Find(tid)
	if err != nil {
		return nil, err
	}
	return togglTask(task), nil
}
