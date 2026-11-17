package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/controller/socket"
	"github.com/sadensmol/autopilot/internal/repository"
)

type ServeTestSuite struct {
	suite.Suite
	binary   string
	settings string
}

func TestServeTestSuite(t *testing.T) {
	suite.Run(t, new(ServeTestSuite))
}

func (s *ServeTestSuite) SetupTest() {
	// the unix socket path limit is 103 bytes on macOS, so the base is short
	root, err := os.MkdirTemp("", "ap")
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(os.RemoveAll(root)) })
	s.settings = filepath.Join(root, ".workspace", "autopilot")
	s.Require().NoError(os.MkdirAll(s.settings, 0o700))
	write := func(path, body string, mode os.FileMode) { s.Require().NoError(os.WriteFile(path, []byte(body), mode)) }
	write(filepath.Join(s.settings, "config.yaml"), "owner:\n  slack_user_id: U0ABC\nservice:\n  source_dir: /src\n", 0o600)
	write(filepath.Join(s.settings, "secrets.env"), "SLACK_APP_TOKEN=a\nSLACK_BOT_TOKEN=b\nLINEAR_API_KEY=c\n", 0o600)
	write(filepath.Join(root, ".workspace", "settings.json"), `{"company": "acme", "agterm_workspace": "acme", "slack": {"workspace": "acme"},
  "linear": {"workspace": "acme", "team": "ABC", "default_label": "autopilot",
    "statuses": {"todo": "a", "planning": "b", "implementing": "c", "qa": "d", "review": "e", "pr": "f", "blocked": "g", "done": "h", "canceled": "i"}}}`, 0o600)
	s.Require().NoError(os.MkdirAll(filepath.Join(root, ".workspace", "projects"), 0o700))
	write(filepath.Join(root, ".workspace", "projects", "abc.json"),
		`{"slack_channel": "abc", "linear_team": "ABC", "repos": [{"name": "abc-api", "default_branch": "main", "deploy": {"manual": "run make release"}}]}`, 0o600)
	s.binary = filepath.Join(s.T().TempDir(), "autopilot")
	build := exec.Command("go", "build", "-o", s.binary, "./cmd/autopilot")
	build.Dir = ".."
	out, err := build.CombinedOutput()
	s.Require().NoError(err, string(out))
}

func (s *ServeTestSuite) TestSecondServeOnTheSameFolderExitsAlreadyRunningBeforeTouchingTheStore() {
	first, err := socket.NewServer(filepath.Join(s.settings, socket.FileName), socket.NewMux())
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.NoError(first.Close()) })

	cmd := exec.Command(s.binary, "serve", "--settings", s.settings, "--agterm-socket", "/tmp/ap-none.sock")
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	err = cmd.Run()

	var exit *exec.ExitError
	s.Require().ErrorAs(err, &exit)
	s.Equal(4, exit.ExitCode(), "serve has a distinct exit code for already running")
	s.Contains(errOut.String(), "already running")
	s.NoFileExists(filepath.Join(s.settings, repository.DBFileName), "the second serve must stop before it opens the store")
}
