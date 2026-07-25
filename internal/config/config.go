package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	configDir  = ".cipi"
	configFile = "config.json"
)

var profileNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

type Profile struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

type fileConfig struct {
	Profiles map[string]Profile `json:"profiles"`
	Default  string             `json:"default,omitempty"`

	// Legacy single-server format (migrated automatically).
	Endpoint string `json:"endpoint,omitempty"`
	Token    string `json:"token,omitempty"`
}

var activeProfile string

func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDir)
}

func Path() string {
	return filepath.Join(Dir(), configFile)
}

func SetActiveProfile(name string) {
	activeProfile = name
}

func ActiveProfile() string {
	return activeProfile
}

func ValidateProfileName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("profile name is required")
	}
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("invalid profile name %q — use letters, numbers, hyphens, and underscores", name)
	}
	return nil
}

func Load() (*Profile, error) {
	fc, err := readFile()
	if err != nil {
		return nil, err
	}

	name := activeProfile
	if name == "" {
		name = fc.Default
	}
	if name == "" && len(fc.Profiles) == 1 {
		for k := range fc.Profiles {
			name = k
			break
		}
	}
	if name == "" {
		return nil, fmt.Errorf("no server selected — use 'cipi-cli <profile> <command>', set a default with 'cipi-cli profiles use <name>', or add one with 'cipi-cli configure --profile <name>'")
	}

	profile, ok := fc.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("server profile %q not found — run 'cipi-cli profiles' to list servers", name)
	}
	if profile.Endpoint == "" || profile.Token == "" {
		return nil, fmt.Errorf("server profile %q is incomplete — run 'cipi-cli configure --profile %s' or 'cipi-cli profiles add %s'", name, name, name)
	}

	return &profile, nil
}

func readFile() (*fileConfig, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not configured — add a server with 'cipi-cli configure --profile <name>' (or 'cipi-cli profiles add <name>')")
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var fc fileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if fc.Profiles == nil {
		fc.Profiles = make(map[string]Profile)
	}

	if fc.Endpoint != "" && fc.Token != "" && len(fc.Profiles) == 0 {
		fc.Profiles["default"] = Profile{
			Endpoint: fc.Endpoint,
			Token:    fc.Token,
		}
		if fc.Default == "" {
			fc.Default = "default"
		}
		fc.Endpoint = ""
		fc.Token = ""
		if err := writeFile(&fc); err != nil {
			return nil, fmt.Errorf("migrating config: %w", err)
		}
	}

	if len(fc.Profiles) == 0 {
		return nil, fmt.Errorf("not configured — add a server with 'cipi-cli configure --profile <name>' (or 'cipi-cli profiles add <name>')")
	}

	return &fc, nil
}

func writeFile(fc *fileConfig) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(Path(), data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

func ProfileExists(name string) bool {
	fc, err := readFile()
	if err != nil {
		return false
	}
	_, ok := fc.Profiles[name]
	return ok
}

func SaveProfile(name string, profile *Profile) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if profile.Endpoint == "" || profile.Token == "" {
		return fmt.Errorf("endpoint and token are required")
	}

	fc, err := readFile()
	if err != nil {
		if !strings.Contains(err.Error(), "not configured") {
			return err
		}
		fc = &fileConfig{Profiles: make(map[string]Profile)}
	}

	fc.Profiles[name] = *profile
	if fc.Default == "" && len(fc.Profiles) == 1 {
		fc.Default = name
	}

	return writeFile(fc)
}

func ListProfiles() ([]string, string, error) {
	fc, err := readFile()
	if err != nil {
		return nil, "", err
	}

	names := make([]string, 0, len(fc.Profiles))
	for name := range fc.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, fc.Default, nil
}

func GetProfile(name string) (*Profile, error) {
	if err := ValidateProfileName(name); err != nil {
		return nil, err
	}

	fc, err := readFile()
	if err != nil {
		return nil, err
	}

	profile, ok := fc.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("server profile %q not found — run 'cipi-cli profiles'", name)
	}
	return &profile, nil
}

func DeleteProfile(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}

	fc, err := readFile()
	if err != nil {
		return err
	}

	if _, ok := fc.Profiles[name]; !ok {
		return fmt.Errorf("server profile %q not found — run 'cipi-cli profiles'", name)
	}

	delete(fc.Profiles, name)
	if fc.Default == name {
		fc.Default = ""
		if len(fc.Profiles) == 1 {
			for k := range fc.Profiles {
				fc.Default = k
				break
			}
		}
	}

	if len(fc.Profiles) == 0 {
		return os.Remove(Path())
	}

	return writeFile(fc)
}

func SetDefaultProfile(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}

	fc, err := readFile()
	if err != nil {
		return err
	}

	if _, ok := fc.Profiles[name]; !ok {
		return fmt.Errorf("server profile %q not found — run 'cipi-cli profiles'", name)
	}

	fc.Default = name
	return writeFile(fc)
}
