package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/controller/cli"
	"github.com/sadensmol/autopilot/internal/engine"
	"github.com/sadensmol/autopilot/internal/repository"
)

const (
	waveSocket = "/tmp/ap-test.sock"
	waveParent = "outer-run"
	waveKey    = "ACM-7"
)

const treeThree = `{"ok":true,"result":{"tree":{"workspaces":[{"name":"acme","sessions":[` +
	`{"id":"sess-1","realized":true},{"id":"sess-2","realized":true},{"id":"sess-3","realized":true}]}]}}}`

const resultComplete = "# Summary\nDid it.\n\n## Files Modified\n- src/a/x.go\n"

type WaveSuite struct {
	suite.Suite
	base   string
	repo   string
	fake   string
	runner *engine.Runner
}

func TestWaveSuite(t *testing.T) {
	suite.Run(t, new(WaveSuite))
}

func (s *WaveSuite) git(args ...string) {
	cmd := exec.Command("git", append([]string{"-C", s.repo}, args...)...)
	out, err := cmd.CombinedOutput()
	s.Require().NoError(err, string(out))
}

func (s *WaveSuite) write(path, content string) {
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o755))
	s.Require().NoError(os.WriteFile(path, []byte(content), 0o644))
}

func (s *WaveSuite) SetupTest() {
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"} {
		s.T().Setenv(k, v)
	}
	s.base = s.T().TempDir()
	s.repo = filepath.Join(s.base, "api")
	s.Require().NoError(os.MkdirAll(s.repo, 0o755))
	s.git("init", "-b", "main")
	s.write(filepath.Join(s.repo, "src", "a", "x.go"), "package a\n")
	s.write(filepath.Join(s.repo, "src", "b", "y.go"), "package b\n")
	s.git("add", ".")
	s.git("commit", "-m", "first")
	s.fake = s.T().TempDir()
	s.T().Setenv("FAKE_AGTERM_DIR", s.fake)
	s.write(filepath.Join(s.fake, "tree.json"), treeThree)
	s.runner = s.newRunner(3)
}

func (s *WaveSuite) newRunner(dialogAttempts int) *engine.Runner {
	fakeBin, err := filepath.Abs(filepath.Join("testdata", "fake-agtermctl.sh"))
	s.Require().NoError(err)
	return engine.NewRunner(engine.RunnerData{
		Sessions:         agterm.New(fakeBin, waveSocket),
		Now:              time.Now,
		PollInterval:     time.Millisecond,
		WaitSlice:        60 * time.Second,
		RealizedAttempts: 3,
		DialogAttempts:   dialogAttempts,
	})
}

func (s *WaveSuite) twoPartPlan() engine.Plan {
	return engine.Plan{Task: "add a health endpoint", Cwd: s.repo, Waves: []engine.Wave{{Parts: []engine.Part{
		{ID: "part-a", Label: "api", Subtask: "do a", Role: "implementer", Files: []string{"src/a/**"}},
		{ID: "part-b", Label: "docs", Subtask: "do b", Role: "implementer", Files: []string{"docs/readme.md"}},
	}}}}
}

func (s *WaveSuite) runDir() string {
	return filepath.Join(s.base, "run")
}

// initRun writes the state and a prompt file per part.
func (s *WaveSuite) initRun(p engine.Plan) {
	if _, err := os.Stat(filepath.Join(s.runDir(), "qa-tests.json")); os.IsNotExist(err) {
		s.write(filepath.Join(s.runDir(), "qa-tests.json"), `{"files":[]}`)
	}
	_, err := engine.Init(s.base, p, waveKey, waveParent)
	s.Require().NoError(err)
	for _, w := range p.Waves {
		for _, part := range w.Parts {
			s.write(filepath.Join(s.runDir(), part.ID+"-prompt.md"), "mission "+part.ID)
		}
	}
}

// settle runs a short wave wait: a part with a result file exits, every other running part times out.
func (s *WaveSuite) settle() {
	_, err := s.runner.Wait(s.T().Context(), s.base, 1, 30*time.Millisecond)
	s.Require().NoError(err)
}

func (s *WaveSuite) calls(verb ...string) [][]string {
	data, err := os.ReadFile(filepath.Join(s.fake, "argv.log"))
	if os.IsNotExist(err) {
		return nil
	}
	s.Require().NoError(err)
	var out [][]string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) >= len(verb) && strings.Join(f[:len(verb)], "\t") == strings.Join(verb, "\t") {
			out = append(out, f)
		}
	}
	return out
}

func (s *WaveSuite) state() engine.State {
	st, err := engine.LoadState(s.base)
	s.Require().NoError(err)
	return st
}

func (s *WaveSuite) TestInit_RejectsInvalidPlanAndWritesNothing() {
	p := s.twoPartPlan()
	p.Waves[0].Parts[1].Files = []string{"src/**"}
	s.write(filepath.Join(s.runDir(), "qa-tests.json"), `{"files":[]}`)

	_, err := engine.Init(s.base, p, waveKey, waveParent)

	s.Require().ErrorContains(err, "both write")
	s.NoFileExists(filepath.Join(s.runDir(), "engine.json"))
}

func (s *WaveSuite) TestInit_WritesRunStateWithDomainType() {
	s.initRun(s.twoPartPlan())

	state, found, err := repository.ReadRunState(s.base)

	s.Require().NoError(err)
	s.True(found)
	s.Equal(1, state.Wave)
	s.Equal(2, state.Parts)
	s.Empty(state.Running)
}

func (s *WaveSuite) TestSpawn_PassesParentNoSelectNameAndCommand() {
	s.initRun(s.twoPartPlan())

	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")

	s.Require().NoError(err)
	news := s.calls("session", "new")
	s.Require().Len(news, 2)
	first := news[0]
	s.Equal([]string{"session", "new", "--socket", waveSocket, "--json", "--parent", waveParent, "--no-select", "--cwd", s.repo, "--name", "api", "--command"}, first[:13])
	s.NotContains(first, "--workspace-name")
	command := first[13]
	s.True(strings.HasPrefix(command, `zsh -lic 'claude --dangerously-skip-permissions --agent autopilot-implementer '\''`), command)
	s.Contains(command, filepath.Join(s.runDir(), "part-a-prompt.md"))
	s.Contains(command, filepath.Join(s.runDir(), "part-a-result.md"))
	s.Contains(command, "Workflow: autopilot.")
}

func (s *WaveSuite) TestSpawn_UsesDedicatedAutopilotAgents() {
	cases := []struct {
		role  string
		agent string
	}{
		{role: "planner", agent: "autopilot-planner"},
		{role: "implementer", agent: "autopilot-implementer"},
		{role: "qa-plan", agent: "autopilot-qa-plan"},
		{role: "qa-verify", agent: "autopilot-qa-verify"},
		{role: "reviewer", agent: "autopilot-reviewer"},
	}
	p := s.twoPartPlan()
	p.Waves[0].Parts = nil
	for _, tc := range cases {
		p.Waves[0].Parts = append(p.Waves[0].Parts, engine.Part{
			ID: tc.role, Label: tc.role, Subtask: "run the role", Role: tc.role,
			Files: []string{fmt.Sprintf("%s/**", tc.role)},
		})
	}
	s.initRun(p)
	s.write(filepath.Join(s.fake, "tree.json"), `{"ok":true,"result":{"tree":{"workspaces":[{"sessions":[{"id":"sess-1","realized":true},{"id":"sess-2","realized":true},{"id":"sess-3","realized":true},{"id":"sess-4","realized":true},{"id":"sess-5","realized":true}]}]}}}`)

	report, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")

	s.Require().NoError(err)
	s.Require().True(report.OK)
	calls := s.calls("session", "new")
	s.Require().Len(calls, len(cases))
	for i, tc := range cases {
		command := calls[i][13]
		s.Contains(command, fmt.Sprintf("--agent %s ", tc.agent))
		s.Contains(command, "Workflow: autopilot.")
		s.NotContains(command, "--agent task-planner ")
		s.Equal(tc.role, s.state().Waves[0].Parts[i].Role)
	}
}

func (s *WaveSuite) TestSpawn_RecordsSessionsRunningAndRunState() {
	s.initRun(s.twoPartPlan())

	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")

	s.Require().NoError(err)
	parts := s.state().Waves[0].Parts
	s.Equal("sess-1", parts[0].SessionID)
	s.Equal("running", parts[0].Status)
	s.Equal("sess-2", parts[1].SessionID)
	run, _, err := repository.ReadRunState(s.base)
	s.Require().NoError(err)
	s.Equal([]string{"part-a", "part-b"}, run.Running)
}

func (s *WaveSuite) TestSpawn_PromptFileMissingFailsThePartOnly() {
	s.initRun(s.twoPartPlan())
	s.Require().NoError(os.Remove(filepath.Join(s.runDir(), "part-b-prompt.md")))

	report, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")

	s.Require().NoError(err)
	s.False(report.OK)
	parts := s.state().Waves[0].Parts
	s.Equal("running", parts[0].Status)
	s.Equal("failed", parts[1].Status)
	s.Len(s.calls("session", "new"), 1)
}

func (s *WaveSuite) TestSpawn_ExistingResultFileFailsThePartOnly() {
	s.initRun(s.twoPartPlan())
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), "### Summary\nan earlier part with the same id\n### Files Modified\n")

	report, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), "FAIL part-b: result file")
	parts := s.state().Waves[0].Parts
	s.Equal("running", parts[0].Status)
	s.Equal("failed", parts[1].Status)
	s.Len(s.calls("session", "new"), 1)
}

func (s *WaveSuite) TestVerify_PartRefusedAtSpawnFailsTheWave() {
	s.initRun(s.twoPartPlan())
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.settle()

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), "FAIL part-b: the part failed before verify")
	s.Equal("failed", s.state().Waves[0].Status)
}

func (s *WaveSuite) TestVerify_UnsettledPartFailsTheWave() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	joined := strings.Join(report.Lines, "\n")
	s.Contains(joined, "FAIL part-a: the part has not settled (running)")
	s.Contains(joined, "FAIL part-b: the part has not settled (running)")
}

func (s *WaveSuite) TestSpawn_SnapshotsTreeBeforeTheWave() {
	s.initRun(s.twoPartPlan())

	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")

	s.Require().NoError(err)
	s.FileExists(filepath.Join(s.runDir(), "tree-before-wave-1.json"))
}

func (s *WaveSuite) TestSpawn_AnswersTheDialogOnlyOnItsExactText() {
	s.initRun(s.twoPartPlan())
	s.write(filepath.Join(s.fake, "text.txt"), "WARNING: Claude Code running in Bypass Permissions mode\n 1. No, exit\n 2. Yes, I accept\n")

	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")

	s.Require().NoError(err)
	typed := s.calls("session", "type")
	s.Require().Len(typed, 4)
	keys := map[string][]string{}
	for _, c := range typed {
		s.Equal([]string{"session", "type", "--socket", waveSocket, "--target"}, c[:5])
		keys[c[5]] = append(keys[c[5]], c[6])
	}
	want := []string{"\x1b[B", "\r"}
	s.Equal(map[string][]string{"sess-1": want, "sess-2": want}, keys)
}

func (s *WaveSuite) TestSpawn_AnswersTheFolderTrustDialogOncePerSession() {
	s.initRun(s.twoPartPlan())
	s.write(filepath.Join(s.fake, "text.txt"), " Quick safety check: Is this a project you created or one you trust?\n ❯ No, exit\n   Yes, I trust this folder\n")

	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")

	s.Require().NoError(err)
	keys := map[string][]string{}
	for _, c := range s.calls("session", "type") {
		keys[c[5]] = append(keys[c[5]], c[6])
	}
	want := []string{"\x1b[B", "\r"}
	s.Equal(map[string][]string{"sess-1": want, "sess-2": want}, keys)
}

func (s *WaveSuite) TestSpawn_OtherScreenTextTypesNothing() {
	s.initRun(s.twoPartPlan())
	s.write(filepath.Join(s.fake, "text.txt"), "Do you trust this folder? 1. Yes 2. No\n")

	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")

	s.Require().NoError(err)
	s.Empty(s.calls("session", "type"))
}

func (s *WaveSuite) TestSpawn_RedispatchClosesTheOldSessionAndStartsANewOne() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)

	_, err = s.runner.Spawn(s.T().Context(), s.base, 1, "part-a")

	s.Require().NoError(err)
	closes := s.calls("session", "close")
	s.Require().Len(closes, 1)
	s.Contains(closes[0], "sess-1")
	s.Len(s.calls("session", "new"), 3)
	parts := s.state().Waves[0].Parts
	s.Equal("sess-3", parts[0].SessionID)
	s.Equal("sess-2", parts[1].SessionID)
}

func (s *WaveSuite) TestWait_SettlesOnCompletedBlockedResultFileAndClosed() {
	p := s.twoPartPlan()
	p.Waves[0].Parts = append(p.Waves[0].Parts,
		engine.Part{ID: "part-c", Label: "c", Subtask: "do c", Role: "implementer", Files: []string{"c/**"}},
		engine.Part{ID: "part-d", Label: "d", Subtask: "do d", Role: "implementer", Files: []string{"d/**"}})
	s.initRun(p)
	s.write(filepath.Join(s.fake, "tree.json"), `{"ok":true,"result":{"tree":{"workspaces":[{"sessions":[{"id":"sess-1","realized":true},{"id":"sess-2","realized":true},{"id":"sess-3","realized":true},{"id":"sess-4","realized":true}]}]}}}`)
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.fake, "wait-sess-1.status"), "completed\n")
	s.write(filepath.Join(s.fake, "wait-sess-2.status"), "blocked\n")
	s.write(filepath.Join(s.runDir(), "part-c-result.md"), resultComplete)
	s.write(filepath.Join(s.fake, "wait-sess-4.exit"), "2")

	report, err := s.runner.Wait(s.T().Context(), s.base, 1, time.Minute)

	s.Require().NoError(err)
	s.False(report.OK, "a blocked part fails the wave")
	got := map[string]string{}
	for _, part := range s.state().Waves[0].Parts {
		got[part.ID] = part.Status
	}
	s.Equal(map[string]string{"part-a": "exited", "part-b": "blocked", "part-c": "exited", "part-d": "closed"}, got)
	for _, c := range s.calls("session", "wait") {
		s.Contains(c, "--until")
		s.Contains(c, "60")
	}
}

func (s *WaveSuite) TestWait_TimeoutSettlesAPartThatNeverReports() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.fake, "wait-sess-1.status"), "completed\n")

	report, err := s.runner.Wait(s.T().Context(), s.base, 1, 30*time.Millisecond)

	s.Require().NoError(err)
	s.False(report.OK)
	parts := s.state().Waves[0].Parts
	s.Equal("exited", parts[0].Status)
	s.Equal("timeout", parts[1].Status)
	run, _, err := repository.ReadRunState(s.base)
	s.Require().NoError(err)
	s.Empty(run.Running)
}

func (s *WaveSuite) TestVerify_PassesWithEvidenceInsideZones() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.repo, "src", "a", "x.go"), "package a\n// changed\n")
	s.write(filepath.Join(s.repo, "docs", "readme.md"), "new\n")
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)
	s.settle()

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.True(report.OK, strings.Join(report.Lines, "\n"))
}

func (s *WaveSuite) TestVerify_FailsOnMissingFileAndMissingSections() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), "# Summary\nonly this\n")
	s.settle()

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	joined := strings.Join(report.Lines, "\n")
	s.Contains(joined, "FAIL part-a: result file missing or empty")
	s.Contains(joined, "FAIL part-b: result file lacks section(s): Files Modified")
	parts := s.state().Waves[0].Parts
	s.Equal("failed", parts[0].Status)
	s.Equal("failed", parts[1].Status)
}

func (s *WaveSuite) TestVerify_ListsFilesChangedOutsideEveryZone() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.repo, "src", "b", "y.go"), "package b\n// not yours\n")
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)
	s.settle()

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), "src/b/y.go")
}

func (s *WaveSuite) TestVerify_ChangesMadeBeforeTheWaveAreNotCounted() {
	s.initRun(s.twoPartPlan())
	s.write(filepath.Join(s.repo, "src", "b", "y.go"), "package b\n// before the wave\n")
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)
	s.settle()

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.True(report.OK, strings.Join(report.Lines, "\n"))
}

func (s *WaveSuite) TestVerify_NoSnapshotFails() {
	s.initRun(s.twoPartPlan())
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), "no snapshot")
}

func (s *WaveSuite) TestSay_LogsOneLineAndSendsToThePane() {
	s.initRun(s.twoPartPlan())

	err := s.runner.Say(s.T().Context(), s.base, "part-a", "sess-9", "line one\r\nline two")

	s.Require().NoError(err)
	log, err := os.ReadFile(filepath.Join(s.runDir(), "messages.log"))
	s.Require().NoError(err)
	s.Regexp(`^\d\d:\d\d:\d\d part-a -> sess-9: line one  line two\n$`, string(log))
	sent := s.calls("msg", "send")
	s.Require().Len(sent, 1)
	s.Equal([]string{"msg", "send", "--socket", waveSocket, "--to", "sess-9:left", "part-a: line one  line two"}, sent[0])
}

func (s *WaveSuite) TestSay_CutsTheMessageAt500Characters() {
	s.initRun(s.twoPartPlan())

	s.Require().NoError(s.runner.Say(s.T().Context(), s.base, "a", "b", strings.Repeat("x", 600)))

	sent := s.calls("msg", "send")
	s.Equal("a: "+strings.Repeat("x", 500), sent[0][len(sent[0])-1])
}

func (s *WaveSuite) TestSay_FailedDeliveryIsLoggedAndReturned() {
	s.initRun(s.twoPartPlan())
	s.write(filepath.Join(s.fake, "msg-fails"), "")

	err := s.runner.Say(s.T().Context(), s.base, "a", "b", "hello")

	s.Require().Error(err)
	log, readErr := os.ReadFile(filepath.Join(s.runDir(), "messages.log"))
	s.Require().NoError(readErr)
	s.Contains(string(log), "(delivery failed)")
}

func (s *WaveSuite) TestRead_ReturnsTheLastLinesOfAPartSession() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.fake, "text.txt"), "tail of the screen\n")

	text, err := s.runner.Read(s.T().Context(), s.base, "part-a")

	s.Require().NoError(err)
	s.Contains(text, "tail of the screen")
	reads := s.calls("session", "text")
	s.Contains(reads[len(reads)-1], "80")
}

func (s *WaveSuite) TestClose_ClosesEverySessionOfTheWave() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)

	s.Require().NoError(s.runner.CloseWave(s.T().Context(), s.base, 1))

	s.Len(s.calls("session", "close"), 2)
}

func (s *WaveSuite) engineCLI(args ...string) (int, string, string) {
	fakeBin, err := filepath.Abs(filepath.Join("testdata", "fake-agtermctl.sh"))
	s.Require().NoError(err)
	env := map[string]string{"AGTERM_SOCKET": waveSocket, "AGTERM_SESSION_ID": waveParent}
	var out, errOut bytes.Buffer
	code := cli.RunEngine(args, cli.Env{Getenv: func(k string) string { return env[k] }, AgtermBin: fakeBin, PollInterval: time.Millisecond}, &out, &errOut)
	return code, out.String(), errOut.String()
}

func (s *WaveSuite) writePlanFile(p engine.Plan) string {
	data, err := json.Marshal(p)
	s.Require().NoError(err)
	path := filepath.Join(s.T().TempDir(), "plan.json")
	s.Require().NoError(os.WriteFile(path, data, 0o644))
	return path
}

func (s *WaveSuite) TestCLI_PlanCheckAcceptsAndRejects() {
	s.write(filepath.Join(s.runDir(), "qa-tests.json"), `{"files":[]}`)
	good := s.writePlanFile(s.twoPartPlan())
	bad := s.twoPartPlan()
	bad.Waves[0].Parts[1].Files = []string{"src/**"}

	code, out, _ := s.engineCLI("plan", "check", good, "--base", s.base)
	s.Equal(0, code)
	s.Contains(out, "plan ok")

	code, _, errOut := s.engineCLI("plan", "check", s.writePlanFile(bad), "--base", s.base)
	s.Equal(1, code)
	s.Contains(errOut, "PLAN PROBLEMS")
	s.Contains(errOut, "both write")
}

func (s *WaveSuite) TestCLI_InitSpawnWaitVerifyRoundTrip() {
	planFile := s.writePlanFile(s.twoPartPlan())
	s.write(filepath.Join(s.runDir(), "qa-tests.json"), `{"files":[]}`)

	code, _, errOut := s.engineCLI("plan", "init", planFile, "--base", s.base, "--key", waveKey)
	s.Require().Equal(0, code, errOut)
	for _, id := range []string{"part-a", "part-b"} {
		s.write(filepath.Join(s.runDir(), id+"-prompt.md"), "mission")
	}
	code, out, errOut := s.engineCLI("wave", "spawn", s.base, "1")
	s.Require().Equal(0, code, errOut)
	s.Contains(out, "spawned part-a")
	s.Equal(waveParent, s.calls("session", "new")[0][6], "the parent defaults to the session of the caller")

	for _, id := range []string{"part-a", "part-b"} {
		s.write(filepath.Join(s.runDir(), id+"-result.md"), resultComplete)
	}
	code, _, errOut = s.engineCLI("wave", "wait", s.base, "1", "--timeout-minutes", "1")
	s.Require().Equal(0, code, errOut)
	code, out, errOut = s.engineCLI("wave", "verify", s.base, "1")
	s.Require().Equal(0, code, errOut)
	s.Contains(out, "OK   part-a")

	code, _, errOut = s.engineCLI("say", s.base, "part-a", "sess-9", "hello", "there")
	s.Require().Equal(0, code, errOut)
	s.Equal("part-a: hello there", s.calls("msg", "send")[0][6])
}

func (s *WaveSuite) TestCLI_VerifyFailureExitsOne() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)

	code, out, _ := s.engineCLI("wave", "verify", s.base, "1")

	s.Equal(1, code)
	s.Contains(out, "FAIL part-a")
}

func (s *WaveSuite) TestCLI_UsageErrorsExitTwo() {
	code, _, errOut := s.engineCLI("wave", "spawn", s.base)
	s.Equal(2, code)
	s.Contains(errOut, "usage")
	code, _, _ = s.engineCLI("wave", "bogus")
	s.Equal(2, code)
}

func (s *WaveSuite) spawnedRun() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)
	s.settle()
}

func (s *WaveSuite) TestVerify_RenameOutOfZoneIntoZoneListsTheOldPath() {
	s.spawnedRun()
	s.git("mv", "src/b/y.go", "src/a/y.go")

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), "src/b/y.go")
}

func (s *WaveSuite) TestVerify_WorkerCommitOutOfZoneFails() {
	s.spawnedRun()
	s.write(filepath.Join(s.repo, "src", "b", "y.go"), "package b\n// committed\n")
	s.git("add", ".")
	s.git("commit", "-m", "worker commit")

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	joined := strings.Join(report.Lines, "\n")
	s.Contains(joined, "src/b/y.go")
	s.Contains(joined, "HEAD moved")
}

func (s *WaveSuite) TestVerify_WorkerCommitInsideZoneStillFailsOnMovedHead() {
	s.spawnedRun()
	s.write(filepath.Join(s.repo, "src", "a", "x.go"), "package a\n// committed\n")
	s.git("add", ".")
	s.git("commit", "-m", "worker commit")

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), "HEAD moved")
}

func (s *WaveSuite) TestSnapshot_RepoWithoutCommitIsNotAnError() {
	dir := s.T().TempDir()
	cmd := exec.Command("git", "-C", dir, "init", "-b", "main")
	out, err := cmd.CombinedOutput()
	s.Require().NoError(err, string(out))
	s.write(filepath.Join(dir, "new.go"), "package n\n")

	snap, err := engine.Snapshot(s.T().Context(), dir)

	s.Require().NoError(err)
	s.Empty(snap.Head)
	s.Contains(snap.Files, "new.go")
}

func (s *WaveSuite) TestSnapshot_GitErrorThatIsNotNoHeadIsReturned() {
	s.write(filepath.Join(s.repo, ".git", "index"), "garbage that is not an index")

	_, err := engine.Snapshot(s.T().Context(), s.repo)

	s.Require().Error(err)
}

func (s *WaveSuite) TestWait_SessionGoneBeforeTheWaitSettlesAsClosed() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.fake, "wait-sess-1.exit"), "1")
	s.write(filepath.Join(s.fake, "wait-sess-1.stderr"), "Error: no such session: sess-1")
	s.write(filepath.Join(s.fake, "wait-sess-2.status"), "completed\n")

	report, err := s.runner.Wait(s.T().Context(), s.base, 1, time.Minute)

	s.Require().NoError(err)
	s.False(report.OK)
	parts := s.state().Waves[0].Parts
	s.Equal("closed", parts[0].Status)
	s.Equal("exited", parts[1].Status)
}

const qaFile = "src/a/qa_test.go"

// recordQATests commits a QA test file and writes run/qa-tests.json for it, after the run is set up.
func (s *WaveSuite) recordQATests(files ...string) {
	s.write(filepath.Join(s.repo, qaFile), "package a\n")
	s.git("add", ".")
	s.git("commit", "-m", "qa tests")
	data, err := json.Marshal(map[string][]string{"files": files})
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "qa-tests.json"), string(data))
}

func (s *WaveSuite) verifyWithQATests() {
	s.initRun(s.twoPartPlan())
	s.recordQATests(qaFile)
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)
	s.settle()
}

func (s *WaveSuite) TestVerify_QATestEdited() {
	s.verifyWithQATests()
	s.write(filepath.Join(s.repo, qaFile), "package a\n// weakened\n")

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK, "an edit inside the part zone still fails")
	joined := strings.Join(report.Lines, "\n")
	s.Contains(joined, "FAIL QA test file edited")
	s.Contains(joined, qaFile)
}

func (s *WaveSuite) TestVerify_QATestDeleted() {
	s.verifyWithQATests()
	s.git("rm", "-q", qaFile)

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), qaFile)
}

func (s *WaveSuite) TestVerify_QATestRenamedAwayOrOntoIsAnEdit() {
	s.verifyWithQATests()
	s.git("mv", qaFile, "src/a/renamed.go")

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), "FAIL QA test file edited: "+qaFile)
}

func (s *WaveSuite) TestVerify_RenameOntoAQATestPathIsAnEdit() {
	s.initRun(s.twoPartPlan())
	s.recordQATests(qaFile, "src/a/later_test.go")
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)
	s.git("mv", "src/a/x.go", "src/a/later_test.go")
	s.settle()

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), "FAIL QA test file edited: src/a/later_test.go")
}

func (s *WaveSuite) TestVerify_QATestsUntouchedPass() {
	s.verifyWithQATests()
	s.write(filepath.Join(s.repo, "src", "a", "x.go"), "package a\n// changed\n")

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.True(report.OK, strings.Join(report.Lines, "\n"))
}

func (s *WaveSuite) TestVerify_QARolePartMayEditQATests() {
	p := s.twoPartPlan()
	p.Waves[0].Parts[0].Role = "qa-verify"
	s.initRun(p)
	s.recordQATests(qaFile)
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)
	s.write(filepath.Join(s.repo, qaFile), "package a\n// qa-verify fixed it\n")
	s.settle()

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.True(report.OK, strings.Join(report.Lines, "\n"))
}

func (s *WaveSuite) TestVerify_MalformedQATestsFileIsAnError() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "qa-tests.json"), "[")
	s.settle()

	_, err = s.runner.Verify(s.T().Context(), s.base, 1)

	s.Error(err)
}

func (s *WaveSuite) TestCLI_PlanInitRefusesZoneOverQATests() {
	s.write(filepath.Join(s.runDir(), "qa-tests.json"), `{"files":["src/a/qa_test.go"]}`)

	code, _, errOut := s.engineCLI("plan", "init", s.writePlanFile(s.twoPartPlan()), "--base", s.base, "--key", waveKey)

	s.Equal(1, code)
	s.Contains(errOut, `covers QA test file "src/a/qa_test.go"`)
	_, err := os.Stat(filepath.Join(s.runDir(), "engine.json"))
	s.True(os.IsNotExist(err), "a refused plan writes no state")
}

func (s *WaveSuite) TestCLI_PlanCheckWithBaseRefusesZoneOverQATests() {
	s.write(filepath.Join(s.runDir(), "qa-tests.json"), `{"files":["src/a/qa_test.go"]}`)
	planFile := s.writePlanFile(s.twoPartPlan())

	code, _, errOut := s.engineCLI("plan", "check", planFile)
	s.Equal(2, code, "--base is required: the QA rule cannot be skipped")
	s.Contains(errOut, "usage")

	code, _, errOut = s.engineCLI("plan", "check", planFile, "--base", s.base)
	s.Equal(1, code)
	s.Contains(errOut, "covers QA test file")
}

func (s *WaveSuite) writeCriteria(body string) string {
	path := filepath.Join(s.T().TempDir(), "criteria.json")
	s.Require().NoError(os.WriteFile(path, []byte(body), 0o644))
	return path
}

func (s *WaveSuite) TestCLI_CriteriaCheckUsesTheRepoLanguage() {
	s.write(filepath.Join(s.repo, "go.mod"), "module example.com/x\n")
	s.write(filepath.Join(s.runDir(), "qa-tests.json"), `{"files":["a_test.go"]}`)
	good := s.writeCriteria(`{"criteria":[{"id":"c1","text":"x","has_inputs":true,"tests":[` +
		`{"id":"t1","level":"unit","kind":"happy","file":"a_test.go","junit_name":"example.com/x.TestA"},` +
		`{"id":"t2","level":"unit","kind":"negative","file":"a_test.go","junit_name":"example.com/x.TestB"}]}]}`)
	wrongShape := s.writeCriteria(`{"criteria":[{"id":"c1","text":"x","tests":[` +
		`{"id":"t1","level":"unit","kind":"happy","file":"a.test.ts","junit_name":"a.test.ts > suite > works"}]}]}`)

	code, out, errOut := s.engineCLI("criteria", "check", good, "--repo", s.repo, "--base", s.base)
	s.Equal(0, code, errOut)
	s.Contains(out, "criteria ok")

	code, _, errOut = s.engineCLI("criteria", "check", wrongShape, "--repo", s.repo, "--base", s.base)
	s.Equal(1, code)
	s.Contains(errOut, "CRITERIA PROBLEMS")
	s.Contains(errOut, "c1: test t1 JUnit name")
}

func (s *WaveSuite) TestCLI_CriteriaCheckUsageAndMissingRepoProfile() {
	file := s.writeCriteria(`{"criteria":[]}`)

	s.write(filepath.Join(s.runDir(), "qa-tests.json"), `{"files":[]}`)
	code, _, _ := s.engineCLI("criteria", "check", file, "--base", s.base)
	s.Equal(2, code, "--repo is required")

	code, _, _ = s.engineCLI("criteria", "check", file, "--repo", s.repo)
	s.Equal(2, code, "--base is required")

	code, _, errOut := s.engineCLI("criteria", "check", file, "--repo", s.repo, "--base", s.base)
	s.Equal(1, code, "no marker file: no language to check names against")
	s.Contains(errOut, "no usable test profile")
}

func (s *WaveSuite) TestCLI_CriteriaCheckRefusesATestQADoesNotOwn() {
	s.write(filepath.Join(s.repo, "go.mod"), "module example.com/x\n")
	s.write(filepath.Join(s.runDir(), "qa-tests.json"), `{"files":["a_test.go"]}`)
	table := s.writeCriteria(`{"criteria":[{"id":"c1","text":"x","tests":[` +
		`{"id":"t1","level":"unit","kind":"happy","file":"impl_test.go","junit_name":"example.com/x.TestA"}]}]}`)

	code, _, errOut := s.engineCLI("criteria", "check", table, "--repo", s.repo, "--base", s.base)

	s.Equal(1, code)
	s.Contains(errOut, `c1: test t1 file "impl_test.go" is not a QA test file`)
}

func (s *WaveSuite) TestCLI_CriteriaCheckMissingQATestsFileIsAnError() {
	s.write(filepath.Join(s.repo, "go.mod"), "module example.com/x\n")
	table := s.writeCriteria(`{"criteria":[{"id":"c1","text":"x","manual":"look"}]}`)

	code, _, errOut := s.engineCLI("criteria", "check", table, "--repo", s.repo, "--base", s.base)

	s.Equal(1, code)
	s.Contains(errOut, "qa-tests.json")
}

func (s *WaveSuite) TestInit_MissingQATestsFileRefusesAPlanWithAnImplementer() {
	_, err := engine.Init(s.base, s.twoPartPlan(), waveKey, waveParent)
	s.Require().Error(err)
	s.Contains(err.Error(), "qa-tests.json")
	_, statErr := os.Stat(filepath.Join(s.runDir(), "engine.json"))
	s.True(os.IsNotExist(statErr))
}

func (s *WaveSuite) TestInit_MissingQATestsFileIsFineForAQAOnlyPlan() {
	p := engine.Plan{Task: "qa", Cwd: s.repo, Waves: []engine.Wave{{Parts: []engine.Part{
		{ID: "part-q", Label: "qa", Subtask: "plan tests", Role: "qa-plan", Files: []string{"tests/**"}},
	}}}}
	_, err := engine.Init(s.base, p, waveKey, waveParent)
	s.Require().NoError(err)
}

func (s *WaveSuite) TestVerify_MissingQATestsFileFailsAWaveWithAnImplementer() {
	s.initRun(s.twoPartPlan())
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.Require().NoError(os.Remove(filepath.Join(s.runDir(), "qa-tests.json")))
	s.settle()

	_, err = s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().Error(err)
	s.Contains(err.Error(), "qa-tests.json")
}

func (s *WaveSuite) TestVerify_QATestPathsCompareIgnoringCase() {
	s.initRun(s.twoPartPlan())
	s.recordQATests("SRC/A/QA_TEST.GO")
	_, err := s.runner.Spawn(s.T().Context(), s.base, 1, "")
	s.Require().NoError(err)
	s.write(filepath.Join(s.runDir(), "part-a-result.md"), resultComplete)
	s.write(filepath.Join(s.runDir(), "part-b-result.md"), resultComplete)
	s.write(filepath.Join(s.repo, qaFile), "package a\n// weakened\n")
	s.settle()

	report, err := s.runner.Verify(s.T().Context(), s.base, 1)

	s.Require().NoError(err)
	s.False(report.OK)
	s.Contains(strings.Join(report.Lines, "\n"), "FAIL QA test file edited")
}
