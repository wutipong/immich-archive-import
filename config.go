package main

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	ImmichURL    string `yaml:"immich_url"`
	ImmichAPIKey string `yaml:"immich_api_key"`

	LogLevel string `yaml:"log_level"`
}

func LoadConfig(profile string, path string) (config Config, err error) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	// Parse the YAML file into the Config struct
	configMap := make(map[string]Config)
	err = yaml.NewDecoder(file).Decode(&configMap)
	if err != nil {
		return
	}

	config, ok := configMap[profile]

	if !ok {
		err = fmt.Errorf("config profile not found: %s", profile)
		return
	}

	return
}
