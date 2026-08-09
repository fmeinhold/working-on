package toggl

import "time"

// RunningDuration is the Duration a time entry carries while it is running.
// API v8 encoded this as the negative unix start time; v9 uses a plain -1.
const RunningDuration = -1

type TaskList struct {
	Count int
	Tasks []Task
}

// A toggl task
type Task struct {
	Id               int        `json:"id"`
	Name             string     `json:"name"`
	ProjectId        int        `json:"project_id"`
	ProjectName      string     `json:"project_name,omitempty"`
	WorkspaceId      int        `json:"workspace_id"`
	UserId           int        `json:"user_id"`
	EstimatedSeconds int        `json:"estimated_seconds"`
	TrackedSeconds   int        `json:"tracked_seconds"`
	Active           bool       `json:"active"`
	Recurring        bool       `json:"recurring"`
	At               time.Time  `json:"at"`
	ServerDeletedAt  *time.Time `json:"server_deleted_at,omitempty"`
}

// Deleted tasks only appear in a listing filtered with `since`.
func (t *Task) IsDeleted() bool {
	return t.ServerDeletedAt != nil
}

// A toggl time entry
//
// A running entry has Duration == RunningDuration and a nil Stop; a completed
// entry has a positive Duration and, usually, a Stop.
type TimeEntry struct {
	Id          int        `json:"id,omitempty"`
	Description string     `json:"description"`
	WorkspaceId int        `json:"workspace_id"`
	ProjectId   int        `json:"project_id,omitempty"`
	TaskId      int        `json:"task_id,omitempty"`
	UserId      int        `json:"user_id,omitempty"`
	Billable    bool       `json:"billable"`
	Start       *time.Time `json:"start"`
	Stop        *time.Time `json:"stop,omitempty"`
	Duration    int64      `json:"duration"`
	CreatedWith string     `json:"created_with"`
	Tags        []string   `json:"tags,omitempty"`
	At          *time.Time `json:"at,omitempty"`
}

// A toggl workspace
type Workspace struct {
	Id             int    `json:"id"`
	Name           string `json:"name"`
	OrganizationId int    `json:"organization_id"`
	Premium        bool   `json:"premium"`
	Admin          bool   `json:"admin"`
}

// A toggl project
type Project struct {
	Id             int       `json:"id"`
	Name           string    `json:"name"`
	WorkspaceId    int       `json:"workspace_id"`
	ClientId       int       `json:"client_id"`
	ClientName     string    `json:"client_name"`
	Active         bool      `json:"active"`
	IsPrivate      bool      `json:"is_private"`
	Template       bool      `json:"template"`
	TemplateId     int       `json:"template_id"`
	Billable       bool      `json:"billable"`
	AutoEstimates  bool      `json:"auto_estimates"`
	EstimatedHours int       `json:"estimated_hours"`
	At             time.Time `json:"at"`
	Color          string    `json:"color"`
	Rate           float32   `json:"rate"`
	CreatedAt      time.Time `json:"created_at"`
}
