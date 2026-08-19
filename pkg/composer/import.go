package composer

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ImportYAML reads a docker-compose.yml file and parses it into a ComposeConfig.
func ImportYAML(path string) (*ComposeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse into a temporary structure that matches docker-compose.yml format
	var raw struct {
		Version  string                        `yaml:"version"`
		Services map[string]*ServiceConfig     `yaml:"services"`
		Volumes  map[string]interface{}        `yaml:"volumes"`
		Networks map[string]interface{}        `yaml:"networks"`
	}

	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Convert to our internal ComposeConfig structure
	config := NewComposeConfig()
	config.Version = raw.Version
	config.Volumes = raw.Volumes
	config.Networks = raw.Networks

	// Convert services map to ordered slice
	for name, svc := range raw.Services {
		config.Services = append(config.Services, ServiceEntry{
			Name:   name,
			Config: svc,
		})
	}

	return config, nil
}
