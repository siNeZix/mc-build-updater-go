package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const FileName = "mc-mods-updater.config.yml"

type Local struct {
	Branch string `yaml:"Branch"`
}

func LoadOrCreate(directory string) (Local, error) {
	path := filepath.Join(directory, FileName)
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		configuration := Local{Branch: "dead-inside-land"}
		serialized, marshalErr := yaml.Marshal(configuration)
		if marshalErr != nil {
			return Local{}, fmt.Errorf("marshal default configuration: %w", marshalErr)
		}
		if writeErr := os.WriteFile(path, serialized, 0o644); writeErr != nil {
			return Local{}, fmt.Errorf("write default configuration: %w", writeErr)
		}
		return configuration, nil
	}
	if err != nil {
		return Local{}, fmt.Errorf("read configuration: %w", err)
	}

	var configuration Local
	if err := yaml.Unmarshal(contents, &configuration); err != nil {
		return Local{}, fmt.Errorf("parse configuration: %w", err)
	}
	if configuration.Branch == "" {
		return Local{}, fmt.Errorf("configuration %s has an empty Branch", path)
	}
	return configuration, nil
}
