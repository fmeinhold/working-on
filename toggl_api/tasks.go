package toggl_api

import (
	"encoding/json"
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"time"
)

type TasksClient struct {
	config *RequestConfig
}

type Task struct {
	Id               int       `json:"id"`
	Name             string    `json:"name"`
	Pid              int       `json:"project_id"`
	WorkspaceID      int       `json:"workspace_id"`
	UserID           int       `json:"user_id"`
	EstimatedSeconds int       `json:"estimated_seconds"`
	TrackedSeconds   int       `json:"tracked_seconds"`
	Active           bool      `json:"active"`
	At               time.Time `json:"at"`
}

func (t *TasksClient) FetchAll() ([]Task, error) {
	pid, err := cfg.GetDefaultProject()
	if err != nil {
		return nil, err
	}

	wid := cfg.GlobalConfig.GetInt(cfg.TogglDefaultWid)

	url := fmt.Sprintf("https://api.track.toggl.com/api/v9/workspaces/%d/projects/%d/tasks", wid, pid)
	fmt.Println(url)

	rawMessage, err := t.config.SendRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	fmt.Println("rawMessage", rawMessage)

	var tasks []Task
	err = json.Unmarshal(*rawMessage, &tasks)

	if err != nil {
		return nil, err
	}

	return tasks, nil

}
