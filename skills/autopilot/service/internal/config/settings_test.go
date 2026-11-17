package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sadensmol/autopilot/internal/config"
)

const validConfig = `owner:
  slack_user_id: U0ABC
service:
  source_dir: /src/autopilot
`

const validSecrets = "SLACK_APP_TOKEN=xapp-secret-one\nSLACK_BOT_TOKEN=xoxb-secret-two\nLINEAR_API_KEY=lin-secret-three\n"  // pragma: allowlist secret

// secretValues must never appear in an error.
var secretValues = []string{"xapp-secret-one", "xoxb-secret-two", "lin-secret-three", "U0ABC"}  // pragma: allowlist secret

func newDir(t *testing.T, cfg, secrets string, dirMode, secretsMode os.FileMode) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".workspace", "autopilot")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if cfg != "" {
		writeFile(t, filepath.Join(dir, "config.yaml"), cfg, 0o600)
	}
	if secrets != "" {
		writeFile(t, filepath.Join(dir, "secrets.env"), secrets, secretsMode)
	}
	if err := os.Chmod(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSettings(t *testing.T) {
	tests := map[string]struct {
		cfg, secrets string
		dirMode      os.FileMode
		secretsMode  os.FileMode
		wantErr      string
	}{
		"valid":                   {cfg: validConfig, secrets: validSecrets, dirMode: 0o700, secretsMode: 0o600},
		"folder mode 755":         {cfg: validConfig, secrets: validSecrets, dirMode: 0o755, secretsMode: 0o600, wantErr: "mode 755"},
		"secrets mode 644":        {cfg: validConfig, secrets: validSecrets, dirMode: 0o700, secretsMode: 0o644, wantErr: "secrets.env has mode 644"},
		"missing config":          {secrets: validSecrets, dirMode: 0o700, secretsMode: 0o600, wantErr: "config.yaml is missing"},
		"missing secrets file":    {cfg: validConfig, dirMode: 0o700, wantErr: "secrets.env is missing"},
		"missing owner":           {cfg: "service:\n  source_dir: /x\n", secrets: validSecrets, dirMode: 0o700, secretsMode: 0o600, wantErr: "owner.slack_user_id"},
		"missing source dir":      {cfg: "owner:\n  slack_user_id: U0ABC\n", secrets: validSecrets, dirMode: 0o700, secretsMode: 0o600, wantErr: "service.source_dir"},
		"unknown key":             {cfg: validConfig + "permission_mode: x\n", secrets: validSecrets, dirMode: 0o700, secretsMode: 0o600, wantErr: "permission_mode"},
		"zero loop cap":           {cfg: validConfig + "loop_cap: 0\n", secrets: validSecrets, dirMode: 0o700, secretsMode: 0o600, wantErr: "loop_cap"},
		"wrong type hides value":  {cfg: validConfig + "loop_cap: hunter2\n", secrets: validSecrets, dirMode: 0o700, secretsMode: 0o600, wantErr: "invalid"},
		"missing slack bot token": {cfg: validConfig, secrets: "SLACK_APP_TOKEN=a\nLINEAR_API_KEY=c\n", dirMode: 0o700, secretsMode: 0o600, wantErr: "SLACK_BOT_TOKEN"},
		"empty linear key":        {cfg: validConfig, secrets: "SLACK_APP_TOKEN=a\nSLACK_BOT_TOKEN=b\nLINEAR_API_KEY=\n", dirMode: 0o700, secretsMode: 0o600, wantErr: "LINEAR_API_KEY"},
		"bad secrets line":        {cfg: validConfig, secrets: "justtext\n", dirMode: 0o700, secretsMode: 0o600, wantErr: "line 1"},
		"products override":       {cfg: validConfig + "products:\n  blog:\n    linear_project: blog\n", secrets: validSecrets, dirMode: 0o700, secretsMode: 0o600, wantErr: "products"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := newDir(t, tc.cfg, tc.secrets, tc.dirMode, tc.secretsMode)
			_, err := config.LoadSettings(dir)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
			for _, v := range append(secretValues, "hunter2") {
				if strings.Contains(err.Error(), v) {
					t.Fatalf("error leaks a value: %v", err)
				}
			}
		})
	}
}

func TestLoadSettings_MissingFolder(t *testing.T) {
	_, err := config.LoadSettings(filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "is missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadSettings_ValuesAndDefaults(t *testing.T) {
	cfg := validConfig + "repo_paths:\n  api: /custom/api\nloop_cap: 5\n"
	dir := newDir(t, cfg, "# note\nexport SLACK_APP_TOKEN=\"a\"\nSLACK_BOT_TOKEN='b'\nLINEAR_API_KEY=c\n", 0o700, 0o600)
	s, err := config.LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.LoopCap != 5 || s.HeartbeatMinutes != 30 || s.WaveTimeoutMinutes != 45 || s.PRPollMinutes != 5 || s.GateTimeoutMinutes != 45 {
		t.Fatalf("limits = %+v", s)
	}
	if s.SlackAppToken != "a" || s.SlackBotToken != "b" || s.LinearAPIKey != "c" {
		t.Fatal("secrets not read")
	}
	if got := s.RepoPath("api"); got != "/custom/api" {
		t.Fatalf("RepoPath(api) = %s", got)
	}
	want := filepath.Join(filepath.Dir(filepath.Dir(dir)), "web")
	if got := s.RepoPath("web"); got != want {
		t.Fatalf("RepoPath(web) = %s, want %s", got, want)
	}
}
