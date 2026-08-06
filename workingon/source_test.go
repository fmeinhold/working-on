package workingon

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Toggl is the only real source, so the registry's ability to route a key to
// the right source is exercised against a second, made up one: an issue
// tracker keying its tasks "ABC-123".
var trackerIssueKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*-\d+$`)

func numericKey(key string) bool {
	id, err := strconv.Atoi(key)
	return err == nil && id > 0
}

func issueKey(key string) bool {
	return trackerIssueKey.MatchString(key)
}

// registryWith installs the given sources for the duration of a test.
func registryWith(t *testing.T, sources ...Source) {
	t.Helper()

	previous := Registry.RegisteredSources
	t.Cleanup(func() { Registry.RegisteredSources = previous })

	Registry.RegisteredSources = sources
}

// The point of the whole exercise: a key one source owns must not cost a
// round trip to another.
func TestRegistryOnlyAsksSourcesThatClaimTheKey(t *testing.T) {
	cases := []struct {
		name        string
		key         string
		wantTracker int
		wantToggl   int
		wantResult  string
	}{
		{"numeric id goes to toggl only", "30422198", 0, 1, "Testing"},
		{"issue key goes to the tracker only", "MOET-297", 1, 0, "Fix the thing"},
		{"free text goes nowhere", "fixed the build", 0, 0, ""},
		{"multi word text goes nowhere", "call with Sam", 0, 0, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := &stubSource{
				name:    "tracker",
				handles: issueKey,
				tasks:   map[string]*Task{"MOET-297": {Key: "MOET-297", Summary: "Fix the thing"}},
			}
			toggl := &stubSource{
				name:    "toggl",
				handles: numericKey,
				tasks:   map[string]*Task{"30422198": {Key: "30422198", Summary: "Testing", TogglTask: 30422198}},
			}

			registryWith(t, tracker, toggl)

			task, err := Registry.GetTask(tc.key)

			if tracker.calls != tc.wantTracker {
				t.Errorf("tracker consulted %d time(s), want %d", tracker.calls, tc.wantTracker)
			}
			if toggl.calls != tc.wantToggl {
				t.Errorf("toggl consulted %d time(s), want %d", toggl.calls, tc.wantToggl)
			}

			if tc.wantResult == "" {
				if err == nil {
					t.Fatal("expected an error for a key no source claims")
				}
				if task != nil {
					t.Errorf("got task %+v, want nil", task)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetTask(%q): %v", tc.key, err)
			}
			if task.Summary != tc.wantResult {
				t.Errorf("summary = %q, want %q", task.Summary, tc.wantResult)
			}
		})
	}
}

// Free text is how a plain description reaches the time entry, so it must fail
// without touching the network at all.
func TestRegistryUnclaimedKeySaysSo(t *testing.T) {
	tracker := &stubSource{name: "tracker", handles: issueKey}
	toggl := &stubSource{name: "toggl", handles: numericKey}

	registryWith(t, tracker, toggl)

	_, err := Registry.GetTask("refactored the parser")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not a task key") {
		t.Errorf("error = %q, want it to say the key belongs to no source", err)
	}
}

// An unreachable source must not be reported as "no such task".
func TestRegistryDistinguishesFailureFromNotFound(t *testing.T) {
	tracker := &stubSource{
		name:    "tracker",
		handles: issueKey,
		err:     errors.New("connection refused"),
	}
	registryWith(t, tracker)

	_, err := Registry.GetTask("MOET-297")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %q, want it to carry the underlying failure", err)
	}
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, should not claim the task does not exist", err)
	}
}

func TestRegistryReportsNotFoundWhenSourceAnswersCleanly(t *testing.T) {
	toggl := &stubSource{name: "toggl", handles: numericKey}
	registryWith(t, toggl)

	_, err := Registry.GetTask("12345")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not found in toggl") {
		t.Errorf("error = %q, want it to name the source that was asked", err)
	}
}

// nilSource answers with neither a task nor an error, which must not be taken
// as a final answer.
type nilSource struct{ stubSource }

func (n *nilSource) GetTask(string) (*Task, error) { return nil, nil }

// A source returning (nil, nil) must not end the search.
func TestRegistryKeepsLookingPastAnEmptyAnswer(t *testing.T) {
	empty := &nilSource{stubSource{name: "empty", handles: func(string) bool { return true }}}
	real := &stubSource{
		name:    "toggl",
		handles: numericKey,
		tasks:   map[string]*Task{"1": {Key: "1", Summary: "found me"}},
	}

	registryWith(t, empty, real)

	task, err := Registry.GetTask("1")
	if err != nil {
		t.Fatal(err)
	}
	if task == nil || task.Summary != "found me" {
		t.Fatalf("got %+v, want the task from the second source", task)
	}
}

// Toggl task ids are always positive integers. Anything else must be left for
// another source, or for the description.
func TestTogglSourceKeyPattern(t *testing.T) {
	source := &TogglSource{}

	cases := map[string]bool{
		"30422198":        true,
		"1":               true,
		"0":               false,
		"-5":              false,
		"3.14":            false,
		"":                false,
		"MOET-297":        false,
		"fixed the build": false,
	}

	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			if got := source.Handles(key); got != want {
				t.Errorf("TogglSource.Handles(%q) = %v, want %v", key, got, want)
			}
		})
	}
}
