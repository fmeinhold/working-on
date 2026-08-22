package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
)

// togglStub answers the calls init makes: workspaces, projects and tasks.
type togglStub struct {
	workspaces string
	projects   string
	tasks      string
	fail       bool
}

func (s togglStub) session(t *testing.T, answers string, path string) (initSession, *bytes.Buffer) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.fail {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "Incorrect username and/or password")
			return
		}

		if strings.Contains(r.URL.Path, "projects") {
			fmt.Fprint(w, s.projects)
			return
		}
		if strings.Contains(r.URL.Path, "tasks") {
			fmt.Fprint(w, s.tasks)
			return
		}
		fmt.Fprint(w, s.workspaces)
	}))
	t.Cleanup(srv.Close)

	out := &bytes.Buffer{}

	return initSession{
		in:     strings.NewReader(answers),
		out:    out,
		path:   path,
		client: func(apiToken string) *toggl.Toggl { return toggl.NewTogglAt(apiToken, srv.URL) },
	}, out
}

func oneWorkspace() togglStub {
	return togglStub{
		workspaces: `[{"id":1562374,"name":"Sealworks","organization_id":9}]`,
		projects:   `[{"id":91210706,"name":"SW BIZ DEV","workspace_id":1562374,"active":true}]`,
		tasks: `[{"id":241929955,"name":"Development","project_id":91210706,` +
			`"workspace_id":1562374,"active":true},` +
			`{"id":77918943,"name":"Elsewhere","project_id":1,` +
			`"workspace_id":1562374,"active":true}]`,
	}
}

// The whole flow, answering every prompt with a blank line so the defaults
// stand, and reading the config back through the real loader.
func TestInitWritesALoadableConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	session, out := oneWorkspace().session(t, "my-api-token\n\n\n\n\n\n", path)

	if err := runInit(session); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no config written: %v", err)
	}

	for _, want := range []string{"my-api-token", "toggl_wid: 1562374", "date_layout:"} {
		if !strings.Contains(string(written), want) {
			t.Errorf("config is missing %q:\n%s", want, written)
		}
	}

	if !strings.Contains(out.String(), "Sealworks") {
		t.Errorf("output did not name the workspace:\n%s", out)
	}

	// The real proof: the loader accepts what init produced.
	loadConfigFrom(t, dir, func(cfg *workingon.Config) {
		if cfg.Settings.ToggleApiToken != "my-api-token" {
			t.Errorf("token = %q", cfg.Settings.ToggleApiToken)
		}
		if cfg.Settings.ToggleWid != 1562374 {
			t.Errorf("wid = %d, want 1562374", cfg.Settings.ToggleWid)
		}
		if cfg.Settings.Location.String() == "" {
			t.Error("location did not decode")
		}
		if !cfg.Settings.TogglePidRequired {
			t.Error("toggl_pid_required = false, want the default true")
		}

		sanitizer, err := workingon.NewSanitizer(cfg)
		if err != nil {
			t.Errorf("the sanitize section did not load: %v", err)
		}
		if sanitizer.Snap != workingon.DefaultSnap || sanitizer.Short != workingon.DefaultShort {
			t.Errorf("sanitize = %s / %s, want the defaults it wrote out",
				sanitizer.Snap, sanitizer.Short)
		}
	})
}

// The date questions offer what the locale writes, and a blank line takes it.
func TestInitOffersTheLocaleDateDefaults(t *testing.T) {
	for _, tc := range []struct {
		locale   string
		date     string
		dateTime string
	}{
		{"en_US.UTF-8", "1/2/2006", "1/2/2006 3:04 PM"},
		{"de_DE.UTF-8", "2.1.2006", "2.1.2006 15:04"},
		{"sv_SE.UTF-8", "2006-01-02", "2006-01-02 15:04"},
	} {
		t.Run(tc.locale, func(t *testing.T) {
			isolateLocale(t)
			t.Setenv("LANG", tc.locale)

			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")

			session, out := oneWorkspace().session(t, "token\n\n\n\n\n\n", path)

			if err := runInit(session); err != nil {
				t.Fatalf("runInit: %v", err)
			}

			for _, want := range []string{
				"Date format [" + tc.date + "]",
				"Date and time format [" + tc.dateTime + "]",
			} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("prompt did not offer %q:\n%s", want, out)
				}
			}

			loadConfigFrom(t, dir, func(cfg *workingon.Config) {
				if cfg.Settings.DateLayout != tc.date {
					t.Errorf("date_layout = %q, want %q", cfg.Settings.DateLayout, tc.date)
				}
				if cfg.Settings.DateTimeLayout != tc.dateTime {
					t.Errorf("date_time_layout = %q, want %q",
						cfg.Settings.DateTimeLayout, tc.dateTime)
				}
			})
		})
	}
}

// loadConfigFrom runs the real config loader against a directory laid out the
// way the loader expects.
func loadConfigFrom(t *testing.T, dir string, check func(*workingon.Config)) {
	t.Helper()

	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "working_on")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), written, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	cfg, err := workingon.InitConfig()
	if err != nil {
		t.Fatalf("the generated config does not load: %v", err)
	}

	check(cfg)
}

func TestInitAnswersOverrideTheDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// timezone, date, datetime, week start, pid required, task required, pick
	// a default, which one
	answers := "token\nUTC\n2006-01-02\n2006-01-02 15:04\nSun\nn\ny\ny\n1\n"

	session, _ := oneWorkspace().session(t, answers, path)

	if err := runInit(session); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	loadConfigFrom(t, dir, func(cfg *workingon.Config) {
		if cfg.Settings.Location.String() != "UTC" {
			t.Errorf("location = %q, want UTC", cfg.Settings.Location.String())
		}
		if cfg.Settings.DateLayout != "2006-01-02" {
			t.Errorf("date_layout = %q", cfg.Settings.DateLayout)
		}
		if cfg.Settings.TogglePidRequired {
			t.Error("toggl_pid_required = true, want the answered false")
		}
		if !cfg.Settings.ToggleTaskRequired {
			t.Error("toggl_task_required = false, want the answered true")
		}
		if cfg.Settings.ToggleDefaultPid != 91210706 {
			t.Errorf("toggl_default_pid = %d, want the chosen project", cfg.Settings.ToggleDefaultPid)
		}
		if cfg.Settings.DayFirst {
			t.Error("day_first = true, want false for a year first layout")
		}
		if cfg.Settings.WeekStarts != "sunday" {
			t.Errorf("week_starts = %q, want the answered sunday", cfg.Settings.WeekStarts)
		}
	})
}

// A bad token should be caught before the user answers anything else.
func TestInitRejectsABadTokenBeforeAskingMore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	stub := oneWorkspace()
	stub.fail = true

	session, out := stub.session(t, "bad-token\n", path)

	err := runInit(session)
	if err == nil {
		t.Fatal("expected an error for a token toggl rejects")
	}
	if !strings.Contains(err.Error(), "unable to reach toggl") {
		t.Errorf("error = %q, want it to say the token did not work", err)
	}

	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("a config was written despite the token failing")
	}
	if strings.Contains(out.String(), "Timezone") {
		t.Error("kept asking questions after the token failed")
	}
}

func TestInitRequiresAToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	session, _ := oneWorkspace().session(t, "\n", path)

	if err := runInit(session); err == nil {
		t.Fatal("expected an error when no token is given")
	}
}

// An existing config is not silently replaced.
func TestInitRefusesToClobber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("settings:\n  toggl_api_token: existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	session, _ := oneWorkspace().session(t, "token\n\n\n\n\n\n", path)

	err := runInit(session)
	if err == nil {
		t.Fatal("expected an error rather than overwriting an existing config")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error = %q, want it to mention --force", err)
	}

	written, _ := os.ReadFile(path)
	if !strings.Contains(string(written), "existing") {
		t.Error("the existing config was modified")
	}
}

func TestInitForceReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("settings:\n  toggl_api_token: existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	session, _ := oneWorkspace().session(t, "replacement\n\n\n\n\n\n", path)
	session.force = true

	if err := runInit(session); err != nil {
		t.Fatal(err)
	}

	written, _ := os.ReadFile(path)
	if strings.Contains(string(written), "existing") {
		t.Error("the old config survived --force")
	}
	if !strings.Contains(string(written), "replacement") {
		t.Error("the new token was not written")
	}
}

// The token flag exists so it does not have to be typed where it is visible.
func TestInitTakesTheTokenFromAFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	session, out := oneWorkspace().session(t, "\n\n\n\n\n", path)
	session.token = "from-the-flag"

	if err := runInit(session); err != nil {
		t.Fatal(err)
	}

	written, _ := os.ReadFile(path)
	if !strings.Contains(string(written), "from-the-flag") {
		t.Error("the flag's token was not used")
	}
	if strings.Contains(out.String(), "Api token") {
		t.Error("asked for a token that was already supplied")
	}
}

// The config holds an api token, so it must not be world readable.
func TestInitWritesAPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	session, _ := oneWorkspace().session(t, "token\n\n\n\n\n\n", path)

	if err := runInit(session); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("config mode = %o, want 600", mode)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if mode := dirInfo.Mode().Perm(); mode != 0o700 {
		t.Errorf("config directory mode = %o, want 700", mode)
	}
}

func TestInitPicksAmongSeveralWorkspaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	stub := oneWorkspace()
	stub.workspaces = `[{"id":1,"name":"First"},{"id":2,"name":"Second"}]`

	// token, workspace 2, then defaults
	session, out := stub.session(t, "token\n2\n\n\n\n\n\n", path)

	if err := runInit(session); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "1) First") || !strings.Contains(out.String(), "2) Second") {
		t.Errorf("did not list the workspaces:\n%s", out)
	}

	loadConfigFrom(t, dir, func(cfg *workingon.Config) {
		if cfg.Settings.ToggleWid != 2 {
			t.Errorf("wid = %d, want the chosen 2", cfg.Settings.ToggleWid)
		}
	})
}

// Failing to list projects is not worth failing setup over.
func TestInitSurvivesAProjectListingFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.Contains(r.URL.Path, "projects") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "nope")
			return
		}
		fmt.Fprint(w, `[{"id":1562374,"name":"Sealworks"}]`)
	}))
	defer srv.Close()

	out := &bytes.Buffer{}
	session := initSession{
		in:     strings.NewReader("token\n\n\n\n\n\n"),
		out:    out,
		path:   path,
		client: func(apiToken string) *toggl.Toggl { return toggl.NewTogglAt(apiToken, srv.URL) },
	}

	if err := runInit(session); err != nil {
		t.Fatalf("a failed project listing should not stop setup: %v", err)
	}

	loadConfigFrom(t, dir, func(cfg *workingon.Config) {
		if cfg.Settings.ToggleDefaultPid != 0 {
			t.Errorf("toggl_default_pid = %d, want none", cfg.Settings.ToggleDefaultPid)
		}
	})
}

// configured is a loaded global config, which is what sends `wo init` looking
// at the repository instead of the account.
func configured() *workingon.Config {
	cfg := &workingon.Config{}
	cfg.Settings.ToggleApiToken = "already-set-up"
	cfg.Settings.ToggleWid = 1562374
	return cfg
}

// With a global config in place, init sets up the directory instead - and what
// it writes has to survive the real overlay merge.
func TestInitWritesALocalConfigWhenAlreadySetUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, workingon.LocalConfigName)

	// project 1, then task 1
	session, out := oneWorkspace().session(t, "1\n1\n", path)
	session.cfg = configured()

	if err := runInit(session); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no overlay written: %v", err)
	}

	if strings.Contains(string(written), "toggl_api_token") {
		t.Errorf("the overlay carries credentials:\n%s", written)
	}
	if strings.Contains(out.String(), "Api token") {
		t.Error("asked for a token that the global config already has")
	}

	// Only the task of the chosen project is on offer.
	if strings.Contains(out.String(), "Elsewhere") {
		t.Errorf("offered a task from another project:\n%s", out)
	}

	loadWithLocalConfig(t, dir, func(cfg *workingon.Config) {
		if cfg.Settings.ToggleDefaultPid != 91210706 {
			t.Errorf("toggl_default_pid = %d, want the chosen project", cfg.Settings.ToggleDefaultPid)
		}
		if cfg.Settings.ToggleDefaultTask != 241929955 {
			t.Errorf("toggl_default_task = %d, want the chosen task", cfg.Settings.ToggleDefaultTask)
		}
		// The overlay changes only what it names.
		if cfg.Settings.ToggleApiToken != "global-token" {
			t.Errorf("token = %q, want the global one", cfg.Settings.ToggleApiToken)
		}
	})
}

// loadWithLocalConfig runs the real loader from a directory holding an overlay,
// with a minimal global config in place.
func loadWithLocalConfig(t *testing.T, dir string, check func(*workingon.Config)) {
	t.Helper()

	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "working_on")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}

	global := "settings:\n  toggl_api_token: global-token\n  toggl_wid: 1562374\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(global), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	cfg, err := workingon.InitConfig()
	if err != nil {
		t.Fatalf("the generated overlay does not load: %v", err)
	}

	check(cfg)
}

// A project with no task chosen is a complete answer.
func TestInitLocalLeavesTheTaskOut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, workingon.LocalConfigName)

	// project 1, no task
	session, _ := oneWorkspace().session(t, "1\n\n", path)
	session.cfg = configured()

	if err := runInit(session); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	written, _ := os.ReadFile(path)
	if strings.Contains(string(written), "toggl_default_task") {
		t.Errorf("wrote a task that was not chosen:\n%s", written)
	}

	loadWithLocalConfig(t, dir, func(cfg *workingon.Config) {
		if cfg.Settings.ToggleDefaultPid != 91210706 {
			t.Errorf("toggl_default_pid = %d", cfg.Settings.ToggleDefaultPid)
		}
		if cfg.Settings.ToggleDefaultTask != 0 {
			t.Errorf("toggl_default_task = %d, want none", cfg.Settings.ToggleDefaultTask)
		}
	})
}

// The overlay is likely to be committed, so it must not be written private the
// way the credential holding global config is.
func TestInitLocalWritesAReadableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, workingon.LocalConfigName)

	session, _ := oneWorkspace().session(t, "1\n\n", path)
	session.cfg = configured()

	if err := runInit(session); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o644 {
		t.Errorf("overlay mode = %o, want 644", mode)
	}
}

func TestInitLocalRefusesToClobber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, workingon.LocalConfigName)

	if err := os.WriteFile(path, []byte("settings:\n  toggl_default_pid: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	session, _ := oneWorkspace().session(t, "1\n1\n", path)
	session.cfg = configured()

	err := runInit(session)
	if err == nil {
		t.Fatal("expected an error rather than overwriting an existing overlay")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error = %q, want it to mention --force", err)
	}
}

// --global reaches the account setup even once it has been done.
func TestInitGlobalFlagOverridesTheGuess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	session, out := oneWorkspace().session(t, "a-new-token\n\n\n\n\n\n", path)
	session.cfg = configured()
	session.global = true

	if err := runInit(session); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	if !strings.Contains(out.String(), "Api token") {
		t.Errorf("ran the local flow despite --global:\n%s", out)
	}

	written, _ := os.ReadFile(path)
	if !strings.Contains(string(written), "a-new-token") {
		t.Errorf("did not write the global config:\n%s", written)
	}
}

// --local without a global config to build on says so, rather than writing an
// overlay over nothing.
func TestInitLocalNeedsAGlobalConfig(t *testing.T) {
	dir := t.TempDir()

	session, _ := oneWorkspace().session(t, "1\n1\n", filepath.Join(dir, workingon.LocalConfigName))
	session.local = true

	err := runInit(session)
	if err == nil {
		t.Fatal("expected an error when there is no global config")
	}
	if !strings.Contains(err.Error(), "--global") {
		t.Errorf("error = %q, want it to point at `wo init --global`", err)
	}
}

func TestInitRejectsBothTargets(t *testing.T) {
	session, _ := oneWorkspace().session(t, "", filepath.Join(t.TempDir(), "config.yaml"))
	session.global = true
	session.local = true

	if err := runInit(session); err == nil {
		t.Fatal("expected an error when both --global and --local are given")
	}
}

// The overlay belongs at the repository root, not in whichever subdirectory it
// was run from.
func TestDefaultLocalConfigPathUsesTheRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	nested := filepath.Join(root, "cmd", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	path, err := defaultLocalConfigPath()
	if err != nil {
		t.Fatal(err)
	}

	// The temporary directory may be reached through a symlink, so compare
	// the directories as the filesystem resolves them.
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}

	if got != want || filepath.Base(path) != workingon.LocalConfigName {
		t.Errorf("path = %q, want %s at the repository root %q", path, workingon.LocalConfigName, want)
	}
}

// time.Local reports "Local" on most machines, so the zone has to come from
// where /etc/localtime points.
func TestZoneFromPath(t *testing.T) {
	cases := map[string]string{
		"/var/db/timezone/zoneinfo/America/New_York": "America/New_York",
		"/usr/share/zoneinfo/Europe/Berlin":          "Europe/Berlin",
		"/usr/share/zoneinfo/UTC":                    "UTC",
		"/etc/localtime":                             "",
		"":                                           "",
	}

	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			if got := zoneFromPath(path); got != want {
				t.Errorf("zoneFromPath(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

func TestLocalTimezonePrefersTheEnvironment(t *testing.T) {
	t.Setenv("TZ", "Europe/Berlin")

	if got := localTimezone(); got != "Europe/Berlin" {
		t.Errorf("localTimezone() = %q, want the TZ value", got)
	}
}

// With no TZ set it should still name a real zone rather than falling back to
// UTC, which is almost never where the user is.
func TestLocalTimezoneReadsTheSystemZone(t *testing.T) {
	// t.Setenv registers the restore; unsetting afterwards is how you get a
	// genuinely absent TZ, which is different from an empty one.
	t.Setenv("TZ", "placeholder")
	if err := os.Unsetenv("TZ"); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	zoneinfo := filepath.Join(dir, "zoneinfo", "Europe")
	if err := os.MkdirAll(zoneinfo, 0o755); err != nil {
		t.Fatal(err)
	}
	zone := filepath.Join(zoneinfo, "Berlin")
	if err := os.WriteFile(zone, []byte("tzdata"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "localtime")
	if err := os.Symlink(zone, link); err != nil {
		t.Fatal(err)
	}

	previous := localTimeFile
	localTimeFile = link
	t.Cleanup(func() { localTimeFile = previous })

	if got := localTimezone(); got != "Europe/Berlin" {
		t.Errorf("localTimezone() = %q, want Europe/Berlin from the symlink", got)
	}
}

func TestDayFirstLayout(t *testing.T) {
	cases := map[string]bool{
		"2.1.2006":   true,
		"02.01.2006": true,
		"2/1/2006":   true,
		// Starts with a 2, but that 2 is the year.
		"2006-01-02": false,
		"1/2/2006":   false,
		"01.02.2006": false,
	}

	for layout, want := range cases {
		t.Run(layout, func(t *testing.T) {
			if got := dayFirstLayout(layout); got != want {
				t.Errorf("dayFirstLayout(%q) = %v, want %v", layout, got, want)
			}
		})
	}
}

func TestRenderedConfigIsValidYaml(t *testing.T) {
	rendered, err := renderConfig(initAnswers{
		ApiToken:       "tok",
		WorkspaceId:    5,
		WorkspaceName:  "Test",
		Location:       "Europe/Berlin",
		DayFirst:       true,
		DateLayout:     "2.1.2006",
		DateTimeLayout: "2.1.2006 15:04",
		PidRequired:    true,
		DefaultPid:     91210706,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"toggl_api_token: tok",
		"toggl_wid: 5",
		"# Test",
		"location: Europe/Berlin",
		"day_first: true",
		"toggl_pid_required: true",
		"toggl_default_pid: 91210706",
		"templates: []",
		"sources: {}",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered config missing %q:\n%s", want, rendered)
		}
	}
}

// manyNames stands in for a workspace with more projects than a listing shows.
func manyNames() []string {
	names := make([]string, 0, choicesShown+5)
	for i := 0; i < choicesShown; i++ {
		names = append(names, fmt.Sprintf("Filler %d", i+1))
	}
	return append(names,
		"Invoicing Backend",
		"Invoicing Frontend",
		"Legacy Invoices",
		"Acme Website",
		"Acme Mobile",
	)
}

func askWith(answers string, names []string) (int, string) {
	out := &bytes.Buffer{}
	prompt := &prompter{reader: bufio.NewReader(strings.NewReader(answers)), out: out}

	return pickOne(prompt, "Which one", names), out.String()
}

func TestPickOneReachesPastTheListingByTyping(t *testing.T) {
	names := manyNames()

	index, out := askWith("invoicing front\n1\n", names)

	if got := names[index]; got != "Invoicing Frontend" {
		t.Errorf("picked %q, want Invoicing Frontend", got)
	}
	if !strings.Contains(out, "... 5 more") {
		t.Errorf("listing did not say what it left out:\n%s", out)
	}
	if !strings.Contains(out, `1 matching "invoicing front"`) {
		t.Errorf("listing did not report the filter:\n%s", out)
	}
}

func TestPickOneNumbersTheFilteredList(t *testing.T) {
	names := manyNames()

	// 2) of the three matches, not the second project overall.
	index, _ := askWith("invoic\n2\n", names)

	if got := names[index]; got != "Invoicing Frontend" {
		t.Errorf("picked %q, want Invoicing Frontend", got)
	}
}

func TestPickOneFiltersFromTheWholeList(t *testing.T) {
	names := manyNames()

	// A second filter replaces the first rather than narrowing within it.
	index, _ := askWith("invoic\nacme mob\n1\n", names)

	if got := names[index]; got != "Acme Mobile" {
		t.Errorf("picked %q, want Acme Mobile", got)
	}
}

func TestPickOneKeepsTheListWhenNothingMatches(t *testing.T) {
	names := manyNames()

	index, out := askWith("nothing here\nacme web\n1\n", names)

	if got := names[index]; got != "Acme Website" {
		t.Errorf("picked %q, want Acme Website", got)
	}
	if !strings.Contains(out, `Nothing matches "nothing here"`) {
		t.Errorf("a filter matching nothing went unmentioned:\n%s", out)
	}
}

func TestPickOneShowsEverythingAgain(t *testing.T) {
	names := manyNames()

	index, out := askWith("invoic\n*\n1\n", names)

	if got := names[index]; got != names[0] {
		t.Errorf("picked %q, want %q", got, names[0])
	}
	if strings.Count(out, "... 5 more") != 2 {
		t.Errorf("the full listing did not come back:\n%s", out)
	}
}

func TestPickOneRejectsANumberOutsideTheListing(t *testing.T) {
	names := manyNames()

	index, out := askWith("invoic\n4\n1\n", names)

	if got := names[index]; got != "Invoicing Backend" {
		t.Errorf("picked %q, want Invoicing Backend", got)
	}
	if !strings.Contains(out, "Pick a number between 1 and 3") {
		t.Errorf("out of range number was not refused:\n%s", out)
	}
}

func TestPickOneTakesABlankAnswerAsNone(t *testing.T) {
	if index, _ := askWith("\n", manyNames()); index != -1 {
		t.Errorf("blank answer picked %d, want -1", index)
	}
}

// Nothing left to read must end the loop rather than spin on it.
func TestPickOneGivesUpAtEndOfInput(t *testing.T) {
	if index, _ := askWith("invoic\n", manyNames()); index != -1 {
		t.Errorf("exhausted input picked %d, want -1", index)
	}
}
