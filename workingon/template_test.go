package workingon

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// withLayouts gives the parser the settings a template's start and stop are
// read with, since it takes them from the global configuration.
func withLayouts(t *testing.T) {
	t.Helper()
	withSources(t, Config{Settings: Settings{
		DateLayout:     "2.1.2006",
		DateTimeLayout: "2.1.2006 15:04",
		Location:       *time.UTC,
	}})
}

// A template's description is a Go template, filled from --templateArgs.
func TestTemplateRendersItsDescription(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "call", Description: "Call with {{.caller}}"}

	entry, err := tpl.CreateTimeEntryFromTemplate(map[string]string{"caller": "Sam"})
	if err != nil {
		t.Fatalf("CreateTimeEntryFromTemplate: %v", err)
	}

	if entry.Description != "Call with Sam" {
		t.Errorf("description = %q, want %q", entry.Description, "Call with Sam")
	}
}

func TestTemplatePlaceholders(t *testing.T) {
	for name, tc := range map[string]struct {
		description string
		want        []string
	}{
		"a plain description asks for nothing": {"Daily Standup", nil},
		"a placeholder":                        {"Call with {{.caller}}", []string{"caller"}},
		"spaced and piped":                     {"{{ .caller | printf \"%s\" }}", []string{"caller"}},
		"in the order they appear":             {"{{.what}} with {{.caller}}", []string{"what", "caller"}},
		"named twice, asked for once":          {"{{.caller}} and {{.caller}}", []string{"caller"}},
		"a path is asked for by its root":      {"{{.caller.name}}", []string{"caller"}},
		"inside a condition":                   {"Call{{if .caller}} with {{.caller}}{{end}}", []string{"caller"}},
		"a description that does not parse":    {"Call with {{.caller", nil},
	} {
		t.Run(name, func(t *testing.T) {
			if got := placeholders(tc.description); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("placeholders(%q) = %v, want %v", tc.description, got, tc.want)
			}
		})
	}
}

// Only what was left open is asked about - an argument already given is not a
// question.
func TestTemplateAsksOnlyForTheOpenArguments(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "call", Description: "{{.what}} with {{.caller}}"}

	var asked []string
	args, err := tpl.answerOpenArgs(map[string]string{"what": "Call"},
		func(alias string, names []string) (map[string]string, error) {
			if alias != "call" {
				t.Errorf("asked on behalf of %q, want the alias", alias)
			}
			asked = names
			return map[string]string{"caller": "Sam"}, nil
		})
	if err != nil {
		t.Fatalf("answerOpenArgs: %v", err)
	}

	if !reflect.DeepEqual(asked, []string{"caller"}) {
		t.Errorf("asked for %v, want only the unanswered caller", asked)
	}

	entry, err := tpl.CreateTimeEntryFromTemplate(args)
	if err != nil {
		t.Fatalf("CreateTimeEntryFromTemplate: %v", err)
	}
	if entry.Description != "Call with Sam" {
		t.Errorf("description = %q, want %q", entry.Description, "Call with Sam")
	}
}

// The flags belong to the caller, and an answer given here must not follow them
// out of this entry.
func TestTemplateAnsweringLeavesTheGivenArgumentsAlone(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "call", Description: "{{.what}} with {{.caller}}"}
	given := map[string]string{"what": "Call"}

	if _, err := tpl.answerOpenArgs(given, func(string, []string) (map[string]string, error) {
		return map[string]string{"caller": "Sam"}, nil
	}); err != nil {
		t.Fatalf("answerOpenArgs: %v", err)
	}

	if _, added := given["caller"]; added {
		t.Errorf("the answer was written into the caller's arguments: %v", given)
	}
}

// A template whose arguments are all given is not worth interrupting for.
func TestTemplateDoesNotAskWhenNothingIsOpen(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "call", Description: "Call with {{.caller}}"}

	_, err := tpl.answerOpenArgs(map[string]string{"caller": "Sam"},
		func(string, []string) (map[string]string, error) {
			t.Error("asked for an argument that was already given")
			return nil, nil
		})
	if err != nil {
		t.Fatalf("answerOpenArgs: %v", err)
	}
}

// An argument given as empty was answered, however tersely.
func TestTemplateTakesAnEmptyArgumentAsAnAnswer(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "call", Description: "Call with {{.caller}}"}

	if open := tpl.openArgs(map[string]string{"caller": ""}); open != nil {
		t.Errorf("open arguments = %v, want none", open)
	}
}

// With nobody to ask, a placeholder is left as it always was.
func TestTemplateWithoutAnAskerLeavesThePlaceholderOpen(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "call", Description: "Call with {{.caller}}"}

	args, err := tpl.answerOpenArgs(nil, nil)
	if err != nil {
		t.Fatalf("answerOpenArgs: %v", err)
	}

	entry, err := tpl.CreateTimeEntryFromTemplate(args)
	if err != nil {
		t.Fatalf("CreateTimeEntryFromTemplate: %v", err)
	}
	if entry.Description != "Call with <no value>" {
		t.Errorf("description = %q, want the unfilled placeholder", entry.Description)
	}
}

// An answer left blank is no answer, and leaves the placeholder as it was
// rather than booking an entry with a hole in its name.
func TestTemplateBlankAnswerLeavesThePlaceholderOpen(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "call", Description: "Call with {{.caller}}"}

	args, err := tpl.answerOpenArgs(nil, func(string, []string) (map[string]string, error) {
		return map[string]string{"caller": ""}, nil
	})
	if err != nil {
		t.Fatalf("answerOpenArgs: %v", err)
	}

	entry, err := tpl.CreateTimeEntryFromTemplate(args)
	if err != nil {
		t.Fatalf("CreateTimeEntryFromTemplate: %v", err)
	}
	if entry.Description != "Call with <no value>" {
		t.Errorf("description = %q, want the unfilled placeholder", entry.Description)
	}
}

func TestTemplateReportsAFailureToAsk(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "call", Description: "Call with {{.caller}}"}
	failed := errors.New("nobody answered")

	if _, err := tpl.answerOpenArgs(nil, func(string, []string) (map[string]string, error) {
		return nil, failed
	}); !errors.Is(err, failed) {
		t.Errorf("err = %v, want the asker's own", err)
	}
}

// A template may pin the project and task its entries belong to.
func TestTemplatePinsItsProjectAndTask(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "ds", Description: "Daily Standup",
		TogglPid: 91210706, TogglTask: 241929955}

	entry, err := tpl.CreateTimeEntryFromTemplate(nil)
	if err != nil {
		t.Fatalf("CreateTimeEntryFromTemplate: %v", err)
	}

	if entry.ProjectId != 91210706 {
		t.Errorf("project_id = %d, want the template's 91210706", entry.ProjectId)
	}
	if entry.TaskId != 241929955 {
		t.Errorf("task_id = %d, want the template's 241929955", entry.TaskId)
	}
}

// A template that pins neither leaves both for the caller to settle, so the
// usual resolution still applies to an entry booked through it.
func TestTemplateWithoutAProjectLeavesItUnset(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "call", Description: "A call"}

	entry, err := tpl.CreateTimeEntryFromTemplate(nil)
	if err != nil {
		t.Fatalf("CreateTimeEntryFromTemplate: %v", err)
	}

	if entry.ProjectId != 0 || entry.TaskId != 0 {
		t.Errorf("entry = %+v, want no project or task of its own", entry)
	}
}

// The duration follows from the pair of times, so a template need not state it.
func TestTemplateDurationFollowsFromStartAndStop(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "ds", Description: "Daily Standup",
		Start: "17:30", Stop: "17:45"}

	entry, err := tpl.CreateTimeEntryFromTemplate(nil)
	if err != nil {
		t.Fatalf("CreateTimeEntryFromTemplate: %v", err)
	}

	if entry.Duration != 900 {
		t.Errorf("duration = %d, want 900 seconds", entry.Duration)
	}
}

// A stop on its own says nothing about how long the entry ran, and must not
// take the start it does not have.
func TestTemplateWithAStopButNoStartHasNoDuration(t *testing.T) {
	withLayouts(t)
	tpl := TemplateConfig{Alias: "ds", Description: "Daily Standup", Stop: "17:45"}

	entry, err := tpl.CreateTimeEntryFromTemplate(nil)
	if err != nil {
		t.Fatalf("CreateTimeEntryFromTemplate: %v", err)
	}

	if entry.Duration != 0 {
		t.Errorf("duration = %d, want none without a start", entry.Duration)
	}
}
