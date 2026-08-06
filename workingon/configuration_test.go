package workingon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const globalConfig = `
settings:
  day_first: true
  location: Europe/Berlin
  date_layout: "2.1.2006"
  date_time_layout: "2.1.2006 15:04"
  toggl_api_token: the-real-token
  toggl_wid: 1562374
  toggl_pid_required: true
mappings:
  - name: "SW"
    toggl_pid: 91210706
sources:
  tracker:
    username: real@example.com
    password: real-source-token
    url: https://example.invalid
`

// withConfigHome points the loader at a throwaway home and working directory.
func withConfigHome(t *testing.T, global string) (home string, work string) {
	t.Helper()

	home = t.TempDir()
	work = t.TempDir()

	configDir := filepath.Join(home, ".config", "working_on")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if global != "" {
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(global), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("HOME", home)

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	return home, work
}

func writeLocalConfig(t *testing.T, dir string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, LocalConfigName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInitConfigReadsTheGlobalFile(t *testing.T) {
	withConfigHome(t, globalConfig)

	cfg, err := InitConfig()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Settings.ToggleApiToken != "the-real-token" {
		t.Errorf("token = %q, want the global one", cfg.Settings.ToggleApiToken)
	}
	if cfg.Settings.ToggleWid != 1562374 {
		t.Errorf("wid = %d, want 1562374", cfg.Settings.ToggleWid)
	}
	if cfg.Settings.Location.String() != "Europe/Berlin" {
		t.Errorf("location = %q, want Europe/Berlin", cfg.Settings.Location.String())
	}
}

// The bug this replaces: any directory containing a config.yaml used to
// override the global settings, token included.
func TestInitConfigIgnoresConfigYamlInTheWorkingDirectory(t *testing.T) {
	_, work := withConfigHome(t, globalConfig)

	stray := "settings:\n  toggl_api_token: hijacked\n  toggl_wid: 999\n"
	if err := os.WriteFile(filepath.Join(work, "config.yaml"), []byte(stray), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := InitConfig()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Settings.ToggleApiToken != "the-real-token" {
		t.Errorf("token = %q; a stray config.yaml must not shadow the global one", cfg.Settings.ToggleApiToken)
	}
	if cfg.Settings.ToggleWid != 999 {
		return // fine - it did not take effect
	}
	t.Error("a stray config.yaml overrode the workspace id")
}

// The overlay changes only what it names.
func TestLocalConfigOverridesOnlyTheKeysItSets(t *testing.T) {
	_, work := withConfigHome(t, globalConfig)

	writeLocalConfig(t, work, "settings:\n  toggl_pid_required: false\n  day_first: false\n")

	cfg, err := InitConfig()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Settings.TogglePidRequired {
		t.Error("toggl_pid_required was not overridden")
	}
	if cfg.Settings.DayFirst {
		t.Error("day_first was not overridden")
	}

	// Everything it did not mention survives.
	if cfg.Settings.ToggleApiToken != "the-real-token" {
		t.Errorf("token = %q, want the global one", cfg.Settings.ToggleApiToken)
	}
	if cfg.Settings.ToggleWid != 1562374 {
		t.Errorf("wid = %d, want the global 1562374", cfg.Settings.ToggleWid)
	}
	if cfg.Settings.DateLayout != "2.1.2006" {
		t.Errorf("date_layout = %q, want the global one", cfg.Settings.DateLayout)
	}
}

// A checked in overlay must not be able to swap out who you authenticate as.
func TestLocalConfigCannotSupplyCredentials(t *testing.T) {
	_, work := withConfigHome(t, globalConfig)

	writeLocalConfig(t, work, `
settings:
  toggl_api_token: stolen-token
  toggl_wid: 4242
sources:
  tracker:
    username: attacker@example.com
    password: attacker-token
    url: https://attacker.example.com
`)

	cfg, err := InitConfig()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Settings.ToggleApiToken != "the-real-token" {
		t.Errorf("token = %q; an overlay must not set credentials", cfg.Settings.ToggleApiToken)
	}

	tracker, ok := cfg.Sources["tracker"].(map[string]interface{})
	if !ok {
		t.Fatalf("tracker source = %#v, want the global map", cfg.Sources["tracker"])
	}
	if tracker["password"] != "real-source-token" {
		t.Errorf("source password = %v; an overlay must not set credentials", tracker["password"])
	}
	if tracker["url"] != "https://example.invalid" {
		t.Errorf("source url = %v; the whole sources tree is off limits", tracker["url"])
	}

	// A workspace id is a legitimate per repository override, so it applies.
	if cfg.Settings.ToggleWid != 4242 {
		t.Errorf("wid = %d, want the overlay's 4242", cfg.Settings.ToggleWid)
	}
}

func TestLocalConfigCanOverrideMappings(t *testing.T) {
	_, work := withConfigHome(t, globalConfig)

	writeLocalConfig(t, work, "mappings:\n  - name: \"LOCAL\"\n    toggl_pid: 12345\n")

	cfg, err := InitConfig()
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Projects) != 1 || cfg.Projects[0].Name != "LOCAL" {
		t.Fatalf("mappings = %+v, want the overlay's single entry", cfg.Projects)
	}
	if cfg.Projects[0].TogglePid != 12345 {
		t.Errorf("toggl_pid = %d, want 12345", cfg.Projects[0].TogglePid)
	}
}

// The overlay is found from a subdirectory, since that is where you usually
// are inside a checkout.
func TestFindLocalConfigWalksUpToTheRepositoryRoot(t *testing.T) {
	_, work := withConfigHome(t, globalConfig)

	if err := os.MkdirAll(filepath.Join(work, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeLocalConfig(t, work, "settings:\n  toggl_pid_required: false\n")

	nested := filepath.Join(work, "cmd", "internal")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	found := FindLocalConfig()
	if found == "" {
		t.Fatal("no overlay found from a subdirectory of the repository")
	}
	if filepath.Base(found) != LocalConfigName {
		t.Errorf("found %q, want a %s", found, LocalConfigName)
	}
}

// The walk stops at the repository root, so an unrelated file further up is
// never picked up.
func TestFindLocalConfigStopsAtTheRepositoryRoot(t *testing.T) {
	_, work := withConfigHome(t, globalConfig)

	// An overlay above the repository, which must not apply.
	writeLocalConfig(t, work, "settings:\n  toggl_wid: 1\n")

	repo := filepath.Join(work, "checkout")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	if found := FindLocalConfig(); found != "" {
		t.Errorf("found %q above the repository root; the walk should have stopped", found)
	}
}

func TestFindLocalConfigReturnsNothingWhenAbsent(t *testing.T) {
	_, work := withConfigHome(t, globalConfig)

	if err := os.MkdirAll(filepath.Join(work, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}

	if found := FindLocalConfig(); found != "" {
		t.Errorf("found %q, want nothing", found)
	}
}

// A missing config is a plain error naming where it looked, not a panic.
func TestInitConfigWithoutAnyConfigFile(t *testing.T) {
	withConfigHome(t, "")

	_, err := InitConfig()
	if err == nil {
		t.Fatal("expected an error when no config exists")
	}
	if got := err.Error(); got == "" {
		t.Error("expected the error to name the paths searched")
	}
}

func TestInitConfigReportsAnUnreadableOverlay(t *testing.T) {
	_, work := withConfigHome(t, globalConfig)

	writeLocalConfig(t, work, "settings:\n\tthis is not: [valid yaml\n")

	if _, err := InitConfig(); err == nil {
		t.Fatal("expected an error for a malformed overlay")
	}
}

func TestDeleteNested(t *testing.T) {
	tree := map[string]interface{}{
		"settings": map[string]interface{}{
			"toggl_api_token": "secret",
			"toggl_wid":       5,
		},
		"sources": map[string]interface{}{"tracker": map[string]interface{}{"password": "secret"}},
	}

	deleteNested(tree, []string{"settings", "toggl_api_token"})
	deleteNested(tree, []string{"sources"})

	settings := tree["settings"].(map[string]interface{})
	if _, present := settings["toggl_api_token"]; present {
		t.Error("token was not removed")
	}
	if settings["toggl_wid"] != 5 {
		t.Error("a sibling key was removed too")
	}
	if _, present := tree["sources"]; present {
		t.Error("sources was not removed")
	}
}

// Removing the last key under a parent should prune the empty parent, so it
// does not merge as an empty map and blank the global value.
func TestDeleteNestedPrunesEmptyParents(t *testing.T) {
	tree := map[string]interface{}{
		"settings": map[string]interface{}{"toggl_api_token": "secret"},
	}

	deleteNested(tree, []string{"settings", "toggl_api_token"})

	if _, present := tree["settings"]; present {
		t.Errorf("settings survived as %#v, want it pruned", tree["settings"])
	}
}

// A config with no location must not silently fall back to UTC: times are read
// and displayed in it, so UTC would quietly shift everything.
func TestInitConfigDefaultsLocationToTheSystemZone(t *testing.T) {
	withConfigHome(t, "settings:\n  date_time_layout: \"2.1.2006 15:04\"\n")

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig: %v", err)
	}

	// The offset is what matters: a zero Location also has a name, but reports
	// UTC, which is the failure this guards against.
	moment := time.Date(2026, 8, 6, 19, 12, 0, 0, time.UTC)
	_, want := moment.In(time.Local).Zone()
	if _, got := moment.In(&cfg.Settings.Location).Zone(); got != want {
		t.Errorf("offset = %d, want the system zone's %d", got, want)
	}
}

func TestInitConfigKeepsAConfiguredLocation(t *testing.T) {
	withConfigHome(t, "settings:\n  location: America/New_York\n")

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig: %v", err)
	}

	if got := cfg.Settings.Location.String(); got != "America/New_York" {
		t.Errorf("location = %q, want America/New_York", got)
	}
}
