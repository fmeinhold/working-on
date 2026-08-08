package cmd

import (
	"strings"
	"testing"
)

// The one field a caller checks before deciding whether this checkout is one
// that books time. It has to agree with the overlay actually having been found.
func TestWhereSaysWhetherTheCheckoutIsConfigured(t *testing.T) {
	for name, tc := range map[string]struct {
		local string
		want  bool
	}{
		"an overlay was found": {local: "/src/project/.workingon.yaml", want: true},
		"none was":             {want: false},
	} {
		t.Run(name, func(t *testing.T) {
			where := whereJSON{LocalConfig: tc.local, Configured: tc.local != ""}

			if where.Configured != tc.want {
				t.Errorf("configured = %v, want %v", where.Configured, tc.want)
			}
		})
	}
}

// A checkout with no overlay is not a failure to report, it is an answer - and
// one that should say what to do about it.
func TestWhereSaysWhatToDoWithAnUnconfiguredCheckout(t *testing.T) {
	out := renderWhere(whereJSON{Directory: "/src/project"})

	for _, want := range []string{"no .workingon.yaml", "wo init"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not mention %q:\n%s", want, out)
		}
	}
}

func TestWhereNamesTheProjectEntriesWouldLandIn(t *testing.T) {
	for name, tc := range map[string]struct {
		where whereJSON
		want  string
	}{
		"a project that resolved": {
			where: whereJSON{Project: &ref{Id: 42, Name: "Learning Platform"}},
			want:  "Learning Platform (42)",
		},
		"an id that did not": {
			where: whereJSON{Project: &ref{Id: 42}},
			want:  "42",
		},
		"no project at all": {
			where: whereJSON{},
			want:  "no toggl_default_pid is set",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if out := renderWhere(tc.where); !strings.Contains(out, tc.want) {
				t.Errorf("output does not mention %q:\n%s", tc.want, out)
			}
		})
	}
}

// The repository is where the overlay is, not where you happened to run from -
// the walk goes up, so the two are usually different.
func TestWhereNamesTheRepositoryRatherThanTheDirectory(t *testing.T) {
	out := renderWhere(whereJSON{
		Directory:   "/src/project/deep/nested",
		LocalConfig: "/src/project/.workingon.yaml",
		Configured:  true,
	})

	if !strings.Contains(out, "/src/project\n") {
		t.Errorf("output does not name the repository root:\n%s", out)
	}
	if !strings.Contains(out, "/src/project/deep/nested") {
		t.Errorf("output does not name the directory it was run from:\n%s", out)
	}
}
