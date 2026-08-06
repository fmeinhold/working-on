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
	Projects    []ProjectMapping       `mapstructure:"mappings"`
	Templates   []TemplateConfig       `yaml:"templates" mapstructure:"templates"`
	Sources     map[string]interface{} `yaml:"sources" mapstructure:"sources"`
}

type Settings struct {
	Location          time.Location `yaml:"location" mapstructure:"location"`
	DayFirst          bool          `mapstructure:"day_first" yaml:"day_first"`
	DateLayout        string        `mapstructure:"date_layout" yaml:"date_layout"`
	DateTimeLayout    string        `mapstructure:"date_time_layout" yaml:"date_time_layout"`
	ToggleApiToken    string        `mapstructure:"toggl_api_token" yaml:"toggle_api_token"`
	ToggleWid         int           `mapstructure:"toggl_wid" yaml:"toggl_wid"`
	TogglePidRequired bool          `mapstructure:"toggl_pid_required" yaml:"toggl_pid_required"`
	ToggleDefaultPid  int           `mapstructure:"toggl_default_pid" yaml:"toggl_default_pid"`
}

type TemplateConfig struct {
	Alias       string `mapstructure:"alias"`
	Description string `mapstructure:"description"`
	Start       string `mapstructure:"start"`
	Stop        string `mapstructure:"stop"`
	Project     int    `mapstructure:"project"`
	TogglTask   int    `mapstructure:"toggl_task"`
}

type ProjectMapping struct {
	Name      string `yaml:"name" mapstructure:"name"`
	TogglePid int    `yaml:"toggl_pid"  mapstructure:"toggl_pid"`
	Git       string `yaml:"git" mapstructure:"git"`
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

func (c *Config) GetMapping(key string) (*ProjectMapping, error) {
	var projectMapping *ProjectMapping
	for _, n := range c.Projects {
		if strings.EqualFold(n.Name, key) {
			projectMapping = &n
			break
		}
	}
	if projectMapping == nil {
		return nil, fmt.Errorf("project mapping not found for key %s", key)
	}
	return projectMapping, nil
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
	return &Configuration, nil
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
