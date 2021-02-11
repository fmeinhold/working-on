package workingon

import (
	"github.com/fefeme/workingon/toggl"
	"strconv"
)

type TogglSource struct {
	client *toggl.Toggl
	wid    int
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

func (t *TogglSource) GetTasks() ([]Task, error) {
	taskList, err := t.client.TaskClient.List(t.wid)
	if err != nil {
		return nil, err
	}

	var tasks []Task

	for _, task := range taskList.Tasks {
		tasks = append(tasks, Task{
			Key:     strconv.Itoa(task.Id),
			Summary: task.Name,
			Project: Project{
				Key: strconv.Itoa(task.Pid),
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
	return nil
}

func (t *TogglSource) GetTask(key string) (*Task, error) {
	tid, err := strconv.Atoi(key)
	if err != nil {
		return nil, err
	}
	task, err := t.client.TaskClient.Get(tid)
	if err != nil {
		return nil, err
	}
	return &Task{
		Key:     strconv.Itoa(task.Id),
		Summary: task.Name,
		Project: Project{
			Key: strconv.Itoa(task.Pid),
		},
		TogglTask: task.Id,
	}, nil
}
