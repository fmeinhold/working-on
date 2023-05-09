package cfg

import (
	"fmt"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
)

var (
	GlobalConfig  *viper.Viper
	ProjectConfig *viper.Viper
)

const (
	TogglApiToken   = "toggl.api_token"
	TogglDefaultWid = "toggl.default_wid"
	TogglDefaultPid = "toggl.default_pid"
)

func parseGlobalConfig() (*viper.Viper, error) {
	config := viper.New()

	config.SetDefault("time_format", "Mon, Jan 2")
	config.SetConfigName("config-next")
	config.SetConfigType("yaml")

	config.AddConfigPath("$HOME/.config/working_on")

	config.SetEnvPrefix("WO")
	config.AutomaticEnv()

	if err := config.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return config, nil
		}
		return nil, err
	}

	return config, nil
}

func parseProjectConfig(create bool) (*viper.Viper, error) {
	config := viper.New()

	config.SetConfigName(".wo_config")
	config.SetConfigType("yaml")
	config.SetEnvPrefix("WO")

	config.AutomaticEnv()
	config.AddConfigPath(".")

	if !create {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		for {
			wd = filepath.Dir(wd)
			path := filepath.Join(wd, ".wo_config.yaml")
			_, err := os.Open(path)
			if err == nil {
				config.AddConfigPath(wd)
				break
			}

			if wd == "/" {
				break
			}

		}
		if err := config.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, err
			}
		}
	}

	return config, nil
}

func InitGlobalConfig() error {
	var err error
	GlobalConfig, err = parseGlobalConfig()
	if err != nil {
		return err
	}
	return nil
}

func InitProjectConfig(create bool) error {
	var err error
	ProjectConfig, err = parseProjectConfig(create)
	if !create && err != nil {
		return err
	}

	return nil
}

func GetProjectByName(name string) map[string]interface{} {
	return ProjectConfig.GetStringMap(fmt.Sprintf("projects.%s", name))
}

func GetDefaultProject() (int, error) {
	if ProjectConfig == nil {
		err := InitProjectConfig(false)
		if err != nil {
			return 0, err
		}
	}

	defaultProject := ProjectConfig.GetString("default_project")

	if defaultProject == "" {
		return 0, nil
	}

	return ProjectConfig.GetInt(fmt.Sprintf("projects.%s.toggl_project_pid", defaultProject)), nil
}
