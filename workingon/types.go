package workingon

// Project is a project as a source sees it.
//
// TogglProject is set only when the source already knows the toggl project id.
// A toggl-native project knows its own; a project from any other source has to
// be resolved through a mapping in the config.
type Project struct {
	Key          string
	Name         string
	TogglProject int
}

// Task is a task as a source sees it.
//
// TogglTask is set only when the task is toggl-native, in which case the time
// entry can be linked to it directly.
type Task struct {
	Key       string
	Summary   string
	Project   Project
	TogglTask int
}
