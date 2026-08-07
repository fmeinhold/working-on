package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
)

func describerWith(defaultDescription string, answer string, interactive bool) (workingon.Describer, *bytes.Buffer) {
	cfg := &workingon.Config{}
	cfg.Settings.ToggleDefaultDescription = defaultDescription

	out := &bytes.Buffer{}

	return describerFor(cfg, strings.NewReader(answer), out, interactive), out
}

func TestDescriberAsksWhenThereIsSomebodyToAsk(t *testing.T) {
	describe, out := describerWith("", "invoice review\n", true)

	got, err := describe(&toggl.TimeEntry{Id: 42})
	if err != nil {
		t.Fatal(err)
	}

	if got != "invoice review" {
		t.Errorf("description = %q, want the typed answer", got)
	}
	if !strings.Contains(out.String(), "The running entry has no description") {
		t.Errorf("question did not say which entry it was about:\n%s", out)
	}
}

// Pressing enter takes the placeholder rather than leaving toggl to refuse the
// entry a second time.
func TestDescriberFallsBackOnABlankAnswer(t *testing.T) {
	describe, _ := describerWith("", "\n", true)

	if got, _ := describe(&toggl.TimeEntry{Id: 42}); got != untitled {
		t.Errorf("description = %q, want %q", got, untitled)
	}
}

func TestDescriberUsesTheConfiguredDefaultWithoutAsking(t *testing.T) {
	describe, out := describerWith("Development", "typed instead\n", true)

	got, err := describe(&toggl.TimeEntry{Id: 42})
	if err != nil {
		t.Fatal(err)
	}

	if got != "Development" {
		t.Errorf("description = %q, want the configured default", got)
	}
	if out.Len() != 0 {
		t.Errorf("asked anyway:\n%s", out)
	}
}

// A cron job or a pipe has nobody to answer, and failing to track the time is
// worse than tracking it under a placeholder.
func TestDescriberDoesNotAskWithoutATerminal(t *testing.T) {
	describe, out := describerWith("", "", false)

	got, err := describe(&toggl.TimeEntry{Id: 42})
	if err != nil {
		t.Fatal(err)
	}

	if got != untitled {
		t.Errorf("description = %q, want %q", got, untitled)
	}
	if out.Len() != 0 {
		t.Errorf("asked with nobody there:\n%s", out)
	}
}

// The entry being created has no id yet, and calling it "the running entry"
// would point at the wrong one.
func TestDescriberNamesTheEntryItIsAskingAbout(t *testing.T) {
	describe, out := describerWith("", "\n", true)

	if _, err := describe(&toggl.TimeEntry{}); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out.String(), "running") {
		t.Errorf("a new entry was described as the running one:\n%s", out)
	}
}

func projectTasks() []workingon.Task {
	return []workingon.Task{
		{Key: "1", Summary: "Development", TogglTask: 1},
		{Key: "2", Summary: "ATD Conference", TogglTask: 2},
		{Key: "3", Summary: "DevLearn Conference", TogglTask: 3},
	}
}

func TestTaskChooserPicksByTyping(t *testing.T) {
	out := &bytes.Buffer{}

	// The listing is numbered, but a name is what anyone actually remembers.
	choose := chooseTaskFrom(strings.NewReader("atd\n1\n"), out)

	task, err := choose(91210706, projectTasks())
	if err != nil {
		t.Fatal(err)
	}

	if task == nil || task.TogglTask != 2 {
		t.Fatalf("chose %+v, want ATD Conference", task)
	}
	if !strings.Contains(out.String(), "Tasks in project 91210706") {
		t.Errorf("listing did not say which project it was offering:\n%s", out)
	}
}

func TestTaskChooserTakesABlankAnswerAsNone(t *testing.T) {
	choose := chooseTaskFrom(strings.NewReader("\n"), &bytes.Buffer{})

	task, err := choose(91210706, projectTasks())
	if err != nil {
		t.Fatal(err)
	}
	if task != nil {
		t.Errorf("chose %+v, want nothing", task)
	}
}

// With nobody to ask there is no chooser at all, which is what turns a required
// task into a plain error rather than a prompt into the void.
func TestTaskChooserIsAbsentWithoutATerminal(t *testing.T) {
	if taskChooser(false) != nil {
		t.Error("got a chooser for a run with nobody to answer it")
	}
	if taskChooser(true) == nil {
		t.Error("got no chooser for an interactive run")
	}
}

func TestTemplateArgAskerAsksForEachPlaceholder(t *testing.T) {
	out := &bytes.Buffer{}
	ask := askTemplateArgsFrom(strings.NewReader("a review\nSam\n"), out)

	answers, err := ask("call", []string{"what", "caller"})
	if err != nil {
		t.Fatal(err)
	}

	if answers["what"] != "a review" || answers["caller"] != "Sam" {
		t.Errorf("answers = %v, want each question answered in turn", answers)
	}
	if !strings.Contains(out.String(), `Template "call" asks for 2 arguments`) {
		t.Errorf("the questions did not say what was asking:\n%s", out)
	}
	if !strings.Contains(out.String(), "caller: ") {
		t.Errorf("a placeholder was not asked for by name:\n%s", out)
	}
}

// With nobody to ask there is no asker at all, so a scripted run books the
// entry with the placeholder as it stands rather than waiting on an answer.
func TestTemplateArgAskerIsAbsentWithoutATerminal(t *testing.T) {
	if templateArgAsker(false) != nil {
		t.Error("got an asker for a run with nobody to answer it")
	}
	if templateArgAsker(true) == nil {
		t.Error("got no asker for an interactive run")
	}
}
