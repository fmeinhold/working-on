package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewWhereCommand(cfg *workingon.Config) *cobra.Command {
	var show bool

	whereCommand := &cobra.Command{
		Use:   "where",
		Short: "Show which config applies here, and where time booked here goes",
		Long: `Show which config applies in this directory.

Time is filed against the checkout you are standing in, so the answer depends on
where you ask from. This says which files were read, and which project an entry
started here would land in.

A checkout with no ` + "`.workingon.yaml`" + ` of its own is not set up for anything in
particular - entries from it fall back to your global default project, if you
have one. Run ` + "`wo init`" + ` there to give it one.

` + "`--show`" + ` adds the settings those files come to once they are merged, which is
what the commands actually run on and is not what either file says on its own.
Your API token is never printed, only whether there is one.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			where := whereWeAre(cfg)

			if show {
				inForce, err := settingsInForce(cfg)
				if err != nil {
					return err
				}
				where.Config = inForce
			}

			if jsonOutput {
				return emit(where)
			}

			fmt.Print(renderWhere(where))
			return nil
		},
	}

	whereCommand.Flags().BoolVarP(&show, "show", "s", false,
		"Also show the settings in force here, global and overlay merged")

	return whereCommand
}

// Configured is the question worth asking first and is spelled out rather than
// left to be inferred from LocalConfig being null - a caller deciding whether
// this checkout is one that books time should not have to know that the two
// mean the same thing.
type whereJSON struct {
	Directory    string `json:"directory"`
	LocalConfig  string `json:"local_config,omitempty"`
	GlobalConfig string `json:"global_config,omitempty"`
	Project      *ref   `json:"project"`
	Configured   bool   `json:"configured"`

	// Config is what --show was asked for, and is absent otherwise: the
	// ordinary answer is about this directory, not about your settings.
	Config *configJSON `json:"config,omitempty"`
}

func whereWeAre(cfg *workingon.Config) whereJSON {
	directory, err := os.Getwd()
	if err != nil {
		directory = ""
	}

	local := workingon.FindLocalConfig()

	where := whereJSON{
		Directory:    directory,
		LocalConfig:  local,
		GlobalConfig: viper.ConfigFileUsed(),
		Configured:   local != "",
	}

	if project := workingon.CurrentProject(cfg); project != 0 {
		where.Project = named(project, lookupProjectName(project))
	}

	return where
}

func renderWhere(where whereJSON) string {
	out := fmt.Sprintf("Directory      %s\n", where.Directory)

	out += "Repository     "
	if where.LocalConfig == "" {
		out += "no .workingon.yaml - `wo init` here to set one up\n"
	} else {
		out += filepath.Dir(where.LocalConfig) + "\n"
	}

	if where.GlobalConfig != "" {
		out += fmt.Sprintf("Config         %s\n", where.GlobalConfig)
	}

	out += "Project        "
	switch {
	case where.Project == nil:
		out += "none - no toggl_default_pid is set\n"
	case where.Project.Name == "":
		out += fmt.Sprintf("%d\n", where.Project.Id)
	default:
		out += fmt.Sprintf("%s (%d)\n", where.Project.Name, where.Project.Id)
	}

	if where.Config != nil {
		out += renderConfigInForce(where.Config)
	}

	return out
}

// configJSON is the settings in force in this directory: the global file with
// the checkout's overlay merged over it. Neither file says this on its own,
// which is the reason to be able to ask.
//
// The API token is not in here. Whether one is set is worth knowing and is
// reported as a fact; the token itself has no business in output people paste
// into bug reports.
type configJSON struct {
	Settings  settingsJSON `json:"settings"`
	Sanitize  sanitizeJSON `json:"sanitize"`
	Templates []string     `json:"templates,omitempty"`
	Sources   []string     `json:"sources,omitempty"`
}

type settingsJSON struct {
	Location                string `json:"location"`
	DayFirst                bool   `json:"day_first"`
	DateLayout              string `json:"date_layout,omitempty"`
	DateTimeLayout          string `json:"date_time_layout,omitempty"`
	TokenSet                bool   `json:"toggl_api_token_set"`
	TogglWid                int    `json:"toggl_wid,omitempty"`
	TogglPidRequired        bool   `json:"toggl_pid_required"`
	TogglTaskRequired       bool   `json:"toggl_task_required"`
	TogglDefaultPid         int    `json:"toggl_default_pid,omitempty"`
	TogglDefaultTask        int    `json:"toggl_default_task,omitempty"`
	TogglDefaultDescription string `json:"toggl_default_description,omitempty"`
}

// The values here are the ones tidying would run with rather than the text in
// the file: a setting left out reads as the default that stands in for it, so
// what this shows and what `wo sanitize` does are the same thing.
type sanitizeJSON struct {
	Snap    string   `json:"snap"`
	Short   string   `json:"short"`
	NoWork  []string `json:"no_work"`
	DayEnds string   `json:"day_ends,omitempty"`
}

// A configuration that cannot be read is worth failing over rather than
// showing around: the sanitize settings are parsed here for the same reason
// `wo sanitize` parses them, and an error names the setting that is wrong.
func settingsInForce(cfg *workingon.Config) (*configJSON, error) {
	sanitizer, err := workingon.NewSanitizer(cfg)
	if err != nil {
		return nil, err
	}

	view := &configJSON{
		Settings: settingsJSON{
			Location:                cfg.Settings.Location.String(),
			DayFirst:                cfg.Settings.DayFirst,
			DateLayout:              cfg.Settings.DateLayout,
			DateTimeLayout:          cfg.Settings.DateTimeLayout,
			TokenSet:                strings.TrimSpace(cfg.Settings.ToggleApiToken) != "",
			TogglWid:                cfg.Settings.ToggleWid,
			TogglPidRequired:        cfg.Settings.TogglePidRequired,
			TogglTaskRequired:       cfg.Settings.ToggleTaskRequired,
			TogglDefaultPid:         cfg.Settings.ToggleDefaultPid,
			TogglDefaultTask:        cfg.Settings.ToggleDefaultTask,
			TogglDefaultDescription: cfg.Settings.ToggleDefaultDescription,
		},
		Sanitize: sanitizeJSON{
			Snap:   settingDuration(sanitizer.Snap),
			Short:  settingDuration(sanitizer.Short),
			NoWork: zoneStrings(sanitizer.Zones),
		},
	}

	if sanitizer.DayEnds != nil {
		view.Sanitize.DayEnds = clockOf(*sanitizer.DayEnds)
	}

	for _, template := range cfg.Templates {
		view.Templates = append(view.Templates, template.Alias)
	}

	for name := range cfg.Sources {
		view.Sources = append(view.Sources, name)
	}
	sort.Strings(view.Sources)

	return view, nil
}

// An empty list rather than a null, so that reading the zones out of the
// document needs no check first.
func zoneStrings(zones []workingon.Zone) []string {
	out := make([]string, 0, len(zones))
	for _, zone := range zones {
		out = append(out, zone.String())
	}
	return out
}

// settingDuration writes a duration the way the config file spells one, so
// what comes back could be pasted straight back in. Go's own "5m0s" could not.
// Zero is written as "0", which is what turns that part of the tidying off.
func settingDuration(d time.Duration) string {
	if d <= 0 {
		return "0"
	}

	out := ""
	if hours := int(d / time.Hour); hours > 0 {
		out += fmt.Sprintf("%dh", hours)
		d -= time.Duration(hours) * time.Hour
	}
	if minutes := int(d / time.Minute); minutes > 0 {
		out += fmt.Sprintf("%dm", minutes)
		d -= time.Duration(minutes) * time.Minute
	}
	if seconds := int(d / time.Second); seconds > 0 {
		out += fmt.Sprintf("%ds", seconds)
	}

	return out
}

// clockOf writes minutes since midnight as the time of day they stand for.
func clockOf(minute int) string {
	return fmt.Sprintf("%02d:%02d", minute/60, minute%60)
}

func renderConfigInForce(view *configJSON) string {
	out := "\nSettings\n"
	out += setting("location", view.Settings.Location)
	out += setting("day_first", fmt.Sprintf("%t", view.Settings.DayFirst))
	out += setting("date_layout", view.Settings.DateLayout)
	out += setting("date_time_layout", view.Settings.DateTimeLayout)
	out += setting("toggl_api_token", tokenState(view.Settings.TokenSet))
	out += setting("toggl_wid", number(view.Settings.TogglWid))
	out += setting("toggl_pid_required", fmt.Sprintf("%t", view.Settings.TogglPidRequired))
	out += setting("toggl_task_required", fmt.Sprintf("%t", view.Settings.TogglTaskRequired))
	out += setting("toggl_default_pid", number(view.Settings.TogglDefaultPid))
	out += setting("toggl_default_task", number(view.Settings.TogglDefaultTask))
	out += setting("toggl_default_description", view.Settings.TogglDefaultDescription)

	out += "\nSanitize\n"
	out += setting("snap", view.Sanitize.Snap)
	out += setting("short", view.Sanitize.Short)
	out += setting("no_work", strings.Join(view.Sanitize.NoWork, ", "))
	out += setting("day_ends", view.Sanitize.DayEnds)

	if len(view.Templates) > 0 {
		out += "\nTemplates      " + strings.Join(view.Templates, ", ") + "\n"
	}
	if len(view.Sources) > 0 {
		out += "\nSources        " + strings.Join(view.Sources, ", ") + "\n"
	}

	return out
}

// A setting nobody has given a value to says so, rather than trailing off into
// blank space that reads as a rendering fault.
func setting(name, value string) string {
	if value == "" {
		value = "not set"
	}
	return fmt.Sprintf("  %-26s %s\n", name, value)
}

func number(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

// The token is reported as present or absent and never printed - `wo where`
// output is the sort of thing that ends up in a bug report.
func tokenState(set bool) string {
	if set {
		return "set"
	}
	return "not set"
}
