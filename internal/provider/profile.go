package provider

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// cliProfile is the subset of the ZCP CLI profile that the provider uses.
// Keep this shape compatible with zcp-cli/internal/config.Profile.
type cliProfile struct {
	BearerToken string `yaml:"bearer_token"`
	APIURL      string `yaml:"api_url"`
	Project     string `yaml:"project"`
}

type cliConfig struct {
	ActiveProfile string                `yaml:"active_profile"`
	Profiles      map[string]cliProfile `yaml:"profiles"`
}

var errActiveCLIProfileNotFound = errors.New("active ZCP CLI profile not found")

func cliConfigPath() (string, error) {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", errors.New("APPDATA environment variable not set")
		}
		return filepath.Join(appData, "zcp", "config.yaml"), nil
	}

	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "zcp", "config.yaml"), nil
}

func loadCLIProfile(name string) (cliProfile, error) {
	path, err := cliConfigPath()
	if err != nil {
		return cliProfile{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cliProfile{}, fmt.Errorf("ZCP CLI profile %q was not found: no CLI configuration exists at %s", name, path)
		}
		return cliProfile{}, fmt.Errorf("reading ZCP CLI configuration: %w", err)
	}

	var config cliConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return cliProfile{}, errors.New("ZCP CLI configuration is invalid")
	}
	profile, ok := config.Profiles[name]
	if !ok {
		return cliProfile{}, fmt.Errorf("ZCP CLI profile %q was not found", name)
	}
	return profile, nil
}

func activeCLIProfile() (string, cliProfile, error) {
	path, err := cliConfigPath()
	if err != nil {
		return "", cliProfile{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", cliProfile{}, nil
		}
		return "", cliProfile{}, fmt.Errorf("reading ZCP CLI configuration: %w", err)
	}

	var config cliConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", cliProfile{}, errors.New("ZCP CLI configuration is invalid")
	}
	if config.ActiveProfile == "" {
		return "", cliProfile{}, nil
	}
	profile, ok := config.Profiles[config.ActiveProfile]
	if !ok {
		return "", cliProfile{}, fmt.Errorf("%w: %q", errActiveCLIProfileNotFound, config.ActiveProfile)
	}
	return config.ActiveProfile, profile, nil
}
