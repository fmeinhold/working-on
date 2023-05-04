package tasks

type TogglSource struct{}

func NewTogglSource() *TogglSource {
	return &TogglSource{}
}

func (ts *TogglSource) FetchTasks(prefix string) ([]Task, error) {
	//TODO implement me
	panic("implement me")
}
