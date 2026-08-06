package workingon

import (
	"github.com/fefeme/workingon/toggl"
	"strconv"
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

	for _, task := range cached {
		tasks = append(tasks, Task{
			Key:     strconv.Itoa(task.Id),
			Summary: task.Name,
			Project: Project{
				Key:          strconv.Itoa(task.ProjectId),
				Name:         task.ProjectName,
				TogglProject: task.ProjectId,
			},
			TogglTask: task.Id,
		})
	}

	return tasks, nil
}

func (t *TogglSource) GetProjects() ([]Project, error) {
	projectList, err := t.client.WorkspaceClient.ListProjects(t.wid)
	if err != nil {
		return nil, err
	}

	var projects []Project
	for _, project := range projectList.Projects {
		projects = append(projects, Project{
			Key:  strconv.Itoa(project.Id),
			Name: project.Name,
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
	// v9 addresses tasks under their project, so an id-only lookup has to go
	// through the workspace listing. The cache keeps that off the hot path.
	task, err := t.cache.Find(tid)
	if err != nil {
		return nil, err
	}
	return &Task{
		Key:     strconv.Itoa(task.Id),
		Summary: task.Name,
		Project: Project{
			Key:          strconv.Itoa(task.ProjectId),
			Name:         task.ProjectName,
			TogglProject: task.ProjectId,
		},
		TogglTask: task.Id,
	}, nil
}
