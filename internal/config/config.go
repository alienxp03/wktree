package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Copy      []string `yaml:"copy"`
	Symlink   []string `yaml:"symlink"`
	PostSetup []string `yaml:"postSetup"`
}

type LoadOptions struct {
	Env     map[string]string
	HomeDir string
}

func GlobalPath(options LoadOptions) (string, error) {
	configHome := envValue(options.Env, "XDG_CONFIG_HOME")
	if configHome == "" {
		homeDir := options.HomeDir
		if homeDir == "" {
			var err error
			homeDir, err = os.UserHomeDir()
			if err != nil {
				return "", err
			}
		}
		configHome = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(configHome, "wktree", "config.yaml"), nil
}

func ProjectPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".wktree.yaml")
}

func LoadMerged(repoRoot string, options LoadOptions) (Config, error) {
	globalPath, err := GlobalPath(options)
	if err != nil {
		return Config{}, err
	}
	configs := []string{globalPath, ProjectPath(repoRoot)}
	merged := Config{}
	for _, filePath := range configs {
		config, err := LoadFile(filePath)
		if err != nil {
			return Config{}, err
		}
		merged.Copy = append(merged.Copy, config.Copy...)
		merged.Symlink = append(merged.Symlink, config.Symlink...)
		merged.PostSetup = append(merged.PostSetup, config.PostSetup...)
	}
	return merged, nil
}

func LoadFile(filePath string) (Config, error) {
	stat, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	if !stat.Mode().IsRegular() {
		return Config{}, fmt.Errorf("config path is not a file: %s", filePath)
	}

	source, err := os.ReadFile(filePath)
	if err != nil {
		return Config{}, err
	}
	if len(bytes.TrimSpace(source)) == 0 {
		return Config{}, nil
	}

	var node yaml.Node
	if err := yaml.Unmarshal(source, &node); err != nil {
		return Config{}, fmt.Errorf("invalid YAML in %s: %w", filePath, err)
	}
	if err := validateStructure(&node, filePath); err != nil {
		return Config{}, err
	}

	var config Config
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("invalid config in %s: %w", filePath, err)
	}
	if err := validateSetupPaths(config.Copy, "copy", filePath); err != nil {
		return Config{}, err
	}
	if err := validateSetupPaths(config.Symlink, "symlink", filePath); err != nil {
		return Config{}, err
	}
	if err := validateStrings(config.PostSetup, "postSetup", filePath); err != nil {
		return Config{}, err
	}
	return config, nil
}

func HasSetup(config Config) bool {
	return len(config.Copy) > 0 || len(config.Symlink) > 0 || len(config.PostSetup) > 0
}

func validateStructure(node *yaml.Node, filePath string) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("invalid config in %s: expected an object", filePath)
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if key != "copy" && key != "symlink" && key != "postSetup" {
			return fmt.Errorf("invalid config in %s: unsupported key %q", filePath, key)
		}
		value := node.Content[i+1]
		if value.Kind != yaml.SequenceNode {
			return fmt.Errorf("invalid config in %s: %q must be an array", filePath, key)
		}
		for index, item := range value.Content {
			if item.Kind != yaml.ScalarNode || item.ShortTag() != "!!str" || strings.TrimSpace(item.Value) == "" {
				return fmt.Errorf("invalid config in %s: %q must be a non-empty string", filePath, fmt.Sprintf("%s[%d]", key, index))
			}
		}
	}
	return nil
}

func validateSetupPaths(values []string, key string, filePath string) error {
	if err := validateStrings(values, key, filePath); err != nil {
		return err
	}
	for index, value := range values {
		if filepath.IsAbs(value) {
			return fmt.Errorf("invalid config in %s: %q must be a relative file path", filePath, fmt.Sprintf("%s[%d]", key, index))
		}
		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("invalid config in %s: %q cannot contain null bytes", filePath, fmt.Sprintf("%s[%d]", key, index))
		}
		for _, segment := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
			if segment == ".." {
				return fmt.Errorf("invalid config in %s: %q cannot contain \"..\"", filePath, fmt.Sprintf("%s[%d]", key, index))
			}
		}
	}
	return nil
}

func validateStrings(values []string, key string, filePath string) error {
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("invalid config in %s: %q must be a non-empty string", filePath, fmt.Sprintf("%s[%d]", key, index))
		}
	}
	return nil
}

func envValue(env map[string]string, key string) string {
	if env != nil {
		return env[key]
	}
	return os.Getenv(key)
}
