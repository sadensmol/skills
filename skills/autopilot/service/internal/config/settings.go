package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirMode     = 0o700
	secretsMode = 0o600

	configFile  = "config.yaml"
	secretsFile = "secrets.env"
)

// requiredSecrets are the keys secrets.env must hold.
var requiredSecrets = []string{"SLACK_APP_TOKEN", "SLACK_BOT_TOKEN", "LINEAR_API_KEY"}

// Settings is the local, never-committed configuration of one workspace folder.
type Settings struct {
	Owner struct {
		SlackUserID string `yaml:"slack_user_id"`
	} `yaml:"owner"`
	Service struct {
		SourceDir string `yaml:"source_dir"`
	} `yaml:"service"`
	RepoPaths          map[string]string `yaml:"repo_paths"`
	HeartbeatMinutes   int               `yaml:"heartbeat_minutes"`
	LoopCap            int               `yaml:"loop_cap"`
	WaveTimeoutMinutes int               `yaml:"wave_timeout_minutes"`
	PRPollMinutes      int               `yaml:"pr_poll_minutes"`
	GateTimeoutMinutes int               `yaml:"gate_timeout_minutes"`
	SlackAppToken      string            `yaml:"-"`
	SlackBotToken      string            `yaml:"-"`
	LinearAPIKey       string            `yaml:"-"`
	WorkspaceFolder    string            `yaml:"-"`
}

// RepoPath returns the checkout folder of a repo: the repo_paths entry, else <workspace folder>/<repo>.
func (s Settings) RepoPath(repo string) string {
	if p, ok := s.RepoPaths[repo]; ok {
		return p
	}
	return filepath.Join(s.WorkspaceFolder, repo)
}

// LoadSettings reads config.yaml and secrets.env from dir (<workspace folder>/.workspace/autopilot).
func LoadSettings(dir string) (Settings, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Settings{}, fmt.Errorf("folder %s is missing", dir)
		}
		return Settings{}, fmt.Errorf("cannot read folder %s: %w", dir, err)
	}
	if !info.IsDir() {
		return Settings{}, fmt.Errorf("%s is not a folder", dir)
	}
	if info.Mode().Perm() != dirMode {
		return Settings{}, fmt.Errorf("folder %s has mode %o, want %o", dir, info.Mode().Perm(), dirMode)
	}

	s := Settings{
		HeartbeatMinutes:   30,
		LoopCap:            3,
		WaveTimeoutMinutes: 45,
		PRPollMinutes:      5,
		GateTimeoutMinutes: 45,
	}
	if err := decodeStrict(filepath.Join(dir, configFile), &s); err != nil {
		return Settings{}, err
	}
	if err := s.validate(filepath.Join(dir, configFile)); err != nil {
		return Settings{}, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Settings{}, fmt.Errorf("cannot resolve folder %s: %w", dir, err)
	}
	s.WorkspaceFolder = filepath.Dir(filepath.Dir(abs))

	secrets, err := loadSecrets(filepath.Join(dir, secretsFile))
	if err != nil {
		return Settings{}, err
	}
	s.SlackAppToken = secrets["SLACK_APP_TOKEN"]
	s.SlackBotToken = secrets["SLACK_BOT_TOKEN"]
	s.LinearAPIKey = secrets["LINEAR_API_KEY"]
	return s, nil
}

func (s Settings) validate(path string) error {
	if s.Owner.SlackUserID == "" {
		return fmt.Errorf("key owner.slack_user_id is missing in %s", path)
	}
	if s.Service.SourceDir == "" {
		return fmt.Errorf("key service.source_dir is missing in %s", path)
	}
	for _, n := range []struct {
		key string
		val int
	}{
		{"heartbeat_minutes", s.HeartbeatMinutes},
		{"loop_cap", s.LoopCap},
		{"wave_timeout_minutes", s.WaveTimeoutMinutes},
		{"pr_poll_minutes", s.PRPollMinutes},
		{"gate_timeout_minutes", s.GateTimeoutMinutes},
	} {
		if n.val <= 0 {
			return fmt.Errorf("key %s must be above zero in %s", n.key, path)
		}
	}
	for repo, p := range s.RepoPaths {
		if p == "" {
			return fmt.Errorf("key repo_paths.%s is empty in %s", repo, path)
		}
	}
	return nil
}

func loadSecrets(path string) (map[string]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("file %s is missing", path)
		}
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	if info.Mode().Perm() != secretsMode {
		return nil, fmt.Errorf("file %s has mode %o, want %o", path, info.Mode().Perm(), secretsMode)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	defer f.Close()

	values := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			return nil, fmt.Errorf("line %d of %s is not KEY=value", n, path)
		}
		values[strings.TrimSpace(key)] = unquote(strings.TrimSpace(val))
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	for _, key := range requiredSecrets {
		if values[key] == "" {
			return nil, fmt.Errorf("key %s is missing in %s", key, path)
		}
	}
	return values, nil
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}
