package tasks

type Source interface {
	FetchTasks(prefix string) ([]Task, error)
}

type Task struct {
	Key          string
	Descriptions string
	Summary      string
}

func GetSources() ([]Source, error) {
	js, err := NewJiraSource()
	if err != nil {
		return nil, err
	}
	return []Source{NewTogglSource(), js}, nil
}
