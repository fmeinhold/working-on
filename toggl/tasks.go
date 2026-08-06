package toggl

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// tasksPerPage is the page size used when walking the workspace task list.
// The endpoint defaults to 50, which silently truncates any real workspace.
const tasksPerPage = 1000

type TaskClient struct {
	client *Client
}

// taskPage is the paginated envelope returned by the workspace task listing.
type taskPage struct {
	Data       []Task `json:"data"`
	Page       int    `json:"page"`
	PerPage    int    `json:"per_page"`
	TotalCount int    `json:"total_count"`
}

// List returns every task in the workspace, following pagination to the end.
func (t *TaskClient) List(wid int) (*TaskList, error) {
	return t.list(wid, time.Time{})
}

// ListSince returns only the tasks created, modified or deleted since the
// given time. Deleted tasks appear with ServerDeletedAt set, and only through
// this call - a plain listing omits them.
//
// The API rejects a `since` older than three months with a 400.
func (t *TaskClient) ListSince(wid int, since time.Time) (*TaskList, error) {
	return t.list(wid, since)
}

func (t *TaskClient) list(wid int, since time.Time) (*TaskList, error) {
	var tasks []Task

	err := t.eachPage(wid, since, func(page []Task) bool {
		tasks = append(tasks, page...)
		return true
	})
	if err != nil {
		return nil, err
	}

	return &TaskList{
		Tasks: tasks,
		Count: len(tasks),
	}, nil
}

// eachPage walks the workspace task list, handing each page to visit. visit
// returns false to stop early.
func (t *TaskClient) eachPage(wid int, since time.Time, visit func([]Task) bool) error {
	seen := 0

	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("per_page", strconv.Itoa(tasksPerPage))
		if !since.IsZero() {
			params.Set("since", strconv.FormatInt(since.Unix(), 10))
		}

		message, err := t.client.NewMessage("GET",
			fmt.Sprintf("workspaces/%d/tasks?%s", wid, params.Encode()), nil)
		if err != nil {
			return err
		}

		data, err := t.client.SendRequest(message)
		if err != nil {
			return err
		}

		var p taskPage
		if err := json.Unmarshal(*data, &p); err != nil {
			// Not the paginated envelope - fall back to a plain collection.
			var rows []Task
			if err := decodeList(*data, &rows); err != nil {
				return err
			}
			visit(rows)
			return nil
		}

		if len(p.Data) == 0 {
			return nil
		}

		seen += len(p.Data)

		if !visit(p.Data) {
			return nil
		}
		if seen >= p.TotalCount {
			return nil
		}
	}
}

// Get fetches a single task. v9 scopes tasks under their project, so unlike v8
// the task id alone is not enough to address one.
func (t *TaskClient) Get(wid int, pid int, tid int) (*Task, error) {
	message, err := t.client.NewMessage("GET",
		fmt.Sprintf("workspaces/%d/projects/%d/tasks/%d", wid, pid, tid), nil)
	if err != nil {
		return nil, err
	}

	data, err := t.client.SendRequest(message)
	if err != nil {
		return nil, err
	}

	var task Task
	if err := json.Unmarshal(*data, &task); err != nil {
		return nil, err
	}

	return &task, nil
}

// Find looks a task up by id alone, for callers that do not know its project.
//
// v9 dropped v8's /tasks/{id} route and offers no id filter on the listing, so
// this walks the workspace tasks and stops at the first match. That is a lot of
// work for one lookup - prefer Get whenever the project id is known.
func (t *TaskClient) Find(wid int, tid int) (*Task, error) {
	var found *Task

	err := t.eachPage(wid, time.Time{}, func(page []Task) bool {
		for i := range page {
			if page[i].Id == tid {
				task := page[i]
				found = &task
				return false
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}

	if found == nil {
		return nil, fmt.Errorf("task %d not found in workspace %d", tid, wid)
	}
	return found, nil
}
