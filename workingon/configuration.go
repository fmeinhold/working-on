package workingon

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

type Config struct {
	CreatedWith string                 `yaml:"-"`
	Settings    Settings               `yaml:"settings" mapstructure:"settings"`
	Templates   []TemplateConfig       `yaml:"templates" mapstructure:"templates"`
	Sanitize    SanitizeConfig         `yaml:"sanitize" mapstructure:"sanitize"`
	Sources     map[string]interface{} `yaml:"sources" mapstructure:"sources"`
}

// SanitizeConfig is how `wo sanitize` tidies a day.
//
// The durations are read as text so that leaving one out and setting it to
// zero can mean different things: absent takes the default, "0" turns that
// part of the tidying off.
type SanitizeConfig struct {
	// Snap is the grid start and stop times are rounded to, "5m" by default.
	Snap string `mapstructure:"snap"`

	// Short is the length under which an entry is a stub rather than work in
	// its own right, "15m" by default.
	Short string `mapstructure:"short"`

	// NoWork are spans of the day nothing is stretched into, as "12:00-13:00".
	NoWork []string `mapstructure:"no_work"`

	// DayEnds is the time of day work stops, as "18:00". An entry still running
	// past it is ended there rather than left to run overnight. Empty where
	// there is no such time, and nothing is capped.
	DayEnds string `mapstructure:"day_ends"`
}

type Settings struct {
	Location          time.Location `yaml:"location" mapstructure:"location"`
	DayFirst          bool          `mapstructure:"day_first" yaml:"day_first"`
	DateLayout        string        `mapstructure:"date_layout" yaml:"date_layout"`
	DateTimeLayout    string        `mapstructure:"date_time_layout" yaml:"date_time_layout"`
	ToggleApiToken    string        `mapstructure:"toggl_api_token" yaml:"toggle_api_token"`
	ToggleWid         int           `mapstructure:"toggl_wid" yaml:"toggl_wid"`
	TogglePidRequired bool          `mapstructure:"toggl_pid_required" yaml:"toggl_pid_required"`

	// ToggleTaskRequired is the workspace rule that every entry names a task.
	// Where it holds, `wo` asks which one rather than creating an entry the
	// workspace will not accept.
	ToggleTaskRequired bool `mapstructure:"toggl_task_required" yaml:"toggl_task_required"`

	ToggleDefaultPid  int `mapstructure:"toggl_default_pid" yaml:"toggl_default_pid"`
	ToggleDefaultTask int `mapstructure:"toggl_default_task" yaml:"toggl_default_task"`

	// ToggleDefaultDescription names an entry that would otherwise have no
	// description. Left empty, `wo` asks for one instead.
	ToggleDefaultDescription string `mapstructure:"toggl_default_description" yaml:"toggl_default_description"`
}

type TemplateConfig struct {
	Alias       string `mapstructure:"alias"`
	Description string `mapstructure:"description"`
	Start       string `mapstructure:"start"`
	Stop        string `mapstructure:"stop"`
	TogglPid    int    `mapstructure:"toggl_pid"`
	TogglTask   int    `mapstructure:"toggl_task"`
}

var (
	Configuration Config
)

func (c *Config) GetTemplate(alias string) (*TemplateConfig, error) {
	templates := c.Templates
	for _, template := range templates {
		if strings.EqualFold(template.Alias, alias) {
			return &template, nil
		}
	}
	return nil, nil
}

func newStringToLocationHookFunc() mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, data interface{}) (interface{}, error) {
		if from.Kind() == reflect.String && to.Name() == "Location" {
			return time.LoadLocation(data.(string))
		}
		return data, nil
	}
}

// LocalConfigName is the per repository overlay, merged over the global
// config so a checkout can override parts of it without restating the rest.
const LocalConfigName = ".workingon.yaml"

// localConfigDenyList are keys never honoured from a repository local file.
//
// An overlay may pick a project or a workspace; it may not decide who you
// authenticate as. Otherwise cloning someone's repository would be enough to
// swap out your credentials.
var localConfigDenyList = [][]string{
	{"settings", "toggl_api_token"},
	{"sources"},
}

// configSearchPaths are where the global config is looked for. The working
// directory is deliberately absent: this tool runs inside arbitrary
// repositories, and any of them could otherwise shadow your real settings.
func configSearchPaths() []string {
	return []string{
		"$HOME/.config/working_on",
		"$HOME/.config/working-on",
	}
}

func InitConfig() (*Config, error) {
	// Start from a clean slate so repeated calls cannot inherit stale paths.
	viper.Reset()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	for _, path := range configSearchPaths() {
		viper.AddConfigPath(path)
	}

	viper.SetEnvPrefix("WO")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, missing := err.(viper.ConfigFileNotFoundError); missing {
			return nil, fmt.Errorf(
				"no config file found - expected config.yaml in one of: %s",
				strings.Join(configSearchPaths(), ", "))
		}
		return nil, fmt.Errorf("unable to read the config file: %w", err)
	}

	if overlay := FindLocalConfig(); overlay != "" {
		if err := mergeLocalConfig(overlay); err != nil {
			return nil, fmt.Errorf("unable to read %s: %w", overlay, err)
		}
	}

	Configuration = Config{}
	Configuration.CreatedWith = "working_on"

	err := viper.Unmarshal(&Configuration,
		viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(newStringToLocationHookFunc())))

	if err != nil {
		return nil, err
	}

	// An unset location decodes to the zero time.Location, which Go treats as
	// UTC. For a tool that reads times you type and shows them back, silently
	// answering in UTC is worse than guessing: the machine's own zone is what
	// someone who never configured one means.
	if Configuration.Settings.Location.String() == "" {
		Configuration.Settings.Location = localLocation()
	}

	return &Configuration, nil
}

// localLocation is the machine's own zone, by value.
//
// time.Local is initialised lazily on first use, and the copy has to be taken
// after that: dereferencing it too early yields a zero Location, which is
// indistinguishable from the unset one this replaces. Calling String forces
// the initialisation.
func localLocation() time.Location {
	local := time.Local
	_ = local.String()
	return *local
}

// FindLocalConfig looks for a repository local overlay, starting at the
// working directory and walking up. The search stops at the repository root,
// so a file far above an unrelated checkout is never picked up.
func FindLocalConfig() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		candidate := filepath.Join(dir, LocalConfigName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}

		// Having checked this directory, stop if it is the repository root.
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info != nil {
			return ""
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// mergeLocalConfig layers an overlay over the config already loaded, minus
// anything on the deny list.
func mergeLocalConfig(path string) error {
	local := viper.New()
	local.SetConfigFile(path)
	local.SetConfigType("yaml")

	if err := local.ReadInConfig(); err != nil {
		return err
	}

	settings := local.AllSettings()
	for _, key := range localConfigDenyList {
		deleteNested(settings, key)
	}

	return viper.MergeConfigMap(settings)
}

// deleteNested removes a dotted key from a decoded config tree, pruning any
// parent left empty behind it.
func deleteNested(tree map[string]interface{}, path []string) {
	if len(tree) == 0 || len(path) == 0 {
		return
	}

	if len(path) == 1 {
		delete(tree, path[0])
		return
	}

	child, ok := tree[path[0]].(map[string]interface{})
	if !ok {
		return
	}

	deleteNested(child, path[1:])

	if len(child) == 0 {
		delete(tree, path[0])
	}
}
