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
			return Local{}, fmt.Errorf("сериализовать конфигурацию по умолчанию: %w", marshalErr)
		}
		if writeErr := os.WriteFile(path, serialized, 0o644); writeErr != nil {
			return Local{}, fmt.Errorf("записать конфигурацию по умолчанию: %w", writeErr)
		}
		return configuration, nil
	}
	if err != nil {
		return Local{}, fmt.Errorf("прочитать конфигурацию: %w", err)
	}

	var configuration Local
	if err := yaml.Unmarshal(contents, &configuration); err != nil {
		return Local{}, fmt.Errorf("разобрать конфигурацию: %w", err)
	}
	if configuration.Branch == "" {
		return Local{}, fmt.Errorf("в конфигурации %s не указана ветка Branch", path)
	}
	return configuration, nil
}
