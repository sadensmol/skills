package tests_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/controller/socket"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/engine"
	"github.com/sadensmol/autopilot/internal/gate"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/internal/usecase"
	"github.com/sadensmol/autopilot/tests"
)

const (
	gateKey     = domain.IssueKey("ACM-11")
	gateRepo    = "acme-api"
	gateProduct = "widgets"

	addTestFile  = "tests/add_test.stub"
	oldTestFile  = "tests/old_test.stub"
	testAdd      = "acme.add.TestAdd"
	testAddNeg   = "acme.add.TestAddRejectsNegative"
	gateTimeout  = 30 * time.Second
	qaTestsAdded = "acme.add TestAdd needs:src/add.txt\nacme.add TestAddRejectsNegative needs:src/add.txt\n"
)

type GateTestSuite struct {
	tests.Suite
	stub     *tests.StubProfile
	runs     *repository.Run
	verdicts *repository.Verdict
	base     string
	checkout string
	mainSHA  string
	qaSHA    string
	root     string
	registry config.Registry
	uc       *usecase.Gate
}

func TestGateTestSuite(t *testing.T) {
	suite.Run(t, new(GateTestSuite))
}

func gateRegistry() config.Registry {
	var b config.Registry
	b.Projects = []config.Project{{
		SlackChannel: gateProduct, LinearTeam: "ACM",
		Repos: []config.Repo{{Name: gateRepo, DefaultBranch: "main"}},
	}}
	return b
}

func (s *GateTestSuite) SetupTest() {
	s.Suite.SetupTest()
	root := s.T().TempDir()
	s.root = root
	s.registry = gateRegistry()
	origin := filepath.Join(root, "origin.git")
	s.git(root, "init", "--quiet", "--bare", "-b", "main", origin)
	s.base = filepath.Join(root, "runs", string(gateKey))
	s.checkout = filepath.Join(s.base, gateRepo)
	s.git(root, "clone", "--quiet", origin, s.checkout)
	s.git(s.checkout, "checkout", "--quiet", "-b", "main")
	s.mainSHA = s.commit("main", map[string]string{
		oldTestFile:    "acme.core TestOld pass\nacme.core TestOld2 pass\nacme.core TestOld3 pass\n",
		"src/core.txt": "core\n",
	})
	s.git(s.checkout, "push", "--quiet", "origin", "main")
	s.git(s.checkout, "checkout", "--quiet", "-b", "kim/acm-11")
	s.Require().NoError(os.MkdirAll(engine.RunFolder(s.base), 0o755))
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{
		{ID: "AC1", Text: "adds two numbers", HasInputs: true, Tests: []engine.TestRef{
			{ID: "T1", Level: engine.LevelUnit, Kind: engine.KindHappy, File: addTestFile, JUnitName: testAdd},
			{ID: "T2", Level: engine.LevelUnit, Kind: engine.KindNegative, File: addTestFile, JUnitName: testAddNeg},
		}},
		{ID: "AC2", Text: "the result looks right on the page", Manual: "open the page and read the sum"},
	}})
	s.writeQATests(addTestFile)

	s.stub = &tests.StubProfile{}
	s.runs = repository.NewRun(s.DB)
	s.verdicts = repository.NewVerdict(s.DB)
	_, err := s.runs.Create(s.Ctx, domain.Run{
		IssueKey: gateKey, Product: gateProduct, ChannelID: "C1", RequestTS: "1.1", ThreadTS: "1.1", Phase: "planning",
	}, s.Clock.Now())
	s.Require().NoError(err)
	s.Require().NoError(s.runs.SetSession(s.Ctx, gateKey, "sess-1", s.base, s.Clock.Now()))
	s.uc = s.newGate(gateTimeout)
}

func (s *GateTestSuite) TearDownTest() {
	s.Equal(1, s.worktreeCount(), "the gate left a temporary worktree behind")
	s.Suite.TearDownTest()
}

func (s *GateTestSuite) newGate(timeout time.Duration) *usecase.Gate {
	return usecase.NewGate(usecase.GateData{
		Registry: s.registry,
		Runs:     s.runs,
		Verdicts: s.verdicts,
		Checker:  gate.New(gate.Data{Detect: s.stub.Detect, Timeout: timeout}),
		Now:      s.Clock.Now,
	})
}

func (s *GateTestSuite) git(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Kim Tester", "GIT_AUTHOR_EMAIL=kim@example.test",
		"GIT_COMMITTER_NAME=Kim Tester", "GIT_COMMITTER_EMAIL=kim@example.test",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	s.Require().NoError(err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// commit writes files (an empty content deletes the file) and commits them; it returns the new HEAD.
func (s *GateTestSuite) commit(message string, files map[string]string) string {
	for rel, content := range files {
		path := filepath.Join(s.checkout, rel)
		if content == "" {
			s.Require().NoError(os.Remove(path))
			continue
		}
		s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o755))
		s.Require().NoError(os.WriteFile(path, []byte(content), 0o644))
	}
	s.git(s.checkout, "add", "-A")
	s.git(s.checkout, "commit", "--quiet", "-m", message)
	return s.git(s.checkout, "rev-parse", "HEAD")
}

func (s *GateTestSuite) commitQA(content string, extra map[string]string) string {
	files := map[string]string{addTestFile: content}
	for k, v := range extra {
		files[k] = v
	}
	s.qaSHA = s.commit("qa tests", files)
	return s.qaSHA
}

// implFiles is a change that makes the QA tests pass and meets every floor.
func implFiles() map[string]string {
	return map[string]string{
		"src/add.txt":           "add\n",
		".stub/coverage.xml":    "<coverage line-rate=\"0.9\"></coverage>\n",
		".stub/diff-cover.json": `{"lines":10,"missing":1,"percent":90}`,
		".stub/mutation.json":   `{"killed":8,"survived":2}`,
	}
}

func (s *GateTestSuite) commitImpl(changes map[string]string) string {
	files := implFiles()
	for k, v := range changes {
		files[k] = v
	}
	return s.commit("implementation", files)
}

func (s *GateTestSuite) writeCriteria(c engine.Criteria) {
	body, err := json.Marshal(c)
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(filepath.Join(engine.RunFolder(s.base), "criteria.json"), body, 0o644))
}

func (s *GateTestSuite) writeQATests(files ...string) {
	body, err := json.Marshal(engine.QATests{Files: files})
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(filepath.Join(engine.RunFolder(s.base), "qa-tests.json"), body, 0o644))
}

func (s *GateTestSuite) worktreeCount() int {
	return strings.Count(s.git(s.checkout, "worktree", "list", "--porcelain"), "worktree ")
}

// provenRed writes QA's tests, proves them red and commits the implementation; it returns the head SHA.
func (s *GateTestSuite) provenRed(changes map[string]string) string {
	s.commitQA(qaTestsAdded, nil)
	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, s.qaSHA)
	s.Require().NoError(err)
	return s.commitImpl(changes)
}

func (s *GateTestSuite) runGate(sha string) domain.Verdict {
	verdict, err := s.uc.Run(s.Ctx, gateKey, gateRepo, sha)
	s.Require().NoError(err)
	return verdict
}

func checkOf(v domain.Verdict, name string) domain.CheckResult {
	for _, c := range v.Checks {
		if c.Name == name {
			return c
		}
	}
	return domain.CheckResult{}
}

func (s *GateTestSuite) requireOnlyFailing(v domain.Verdict, name, detail string) {
	s.False(v.Passed())
	for _, c := range v.Checks {
		if c.Name == name {
			s.Equal(domain.CheckFail, c.Status, "%s: %s", c.Name, c.Detail)
			s.Contains(c.Detail, detail)
			continue
		}
		s.Contains([]domain.CheckStatus{domain.CheckPass, domain.CheckNotApplicable}, c.Status, "%s: %s", c.Name, c.Detail)
	}
}

func (s *GateTestSuite) TestGate_PassBindsToTheHeadSHA() {
	head := s.provenRed(nil)

	verdict := s.runGate(head)

	s.True(verdict.Passed(), "%+v", verdict.Checks)
	s.Equal(head, verdict.SHA)
	s.Equal(gateRepo, verdict.Repo)
	var names []string
	for _, c := range verdict.Checks {
		s.Equal(domain.CheckPass, c.Status, "%s: %s", c.Name, c.Detail)
		names = append(names, c.Name)
	}
	s.Equal([]string{gate.CheckTestsPass, gate.CheckRedGreen, gate.CheckCriteria, gate.CheckDiffCoverage, gate.CheckMutation, gate.CheckTestCount}, names)
	s.Equal([]string{"AC2: open the page and read the sum"}, verdict.Manual)
	stored, err := s.verdicts.Latest(s.Ctx, gateKey, gateRepo)
	s.Require().NoError(err)
	s.Equal(head, stored.SHA)
	s.Equal(verdict.Checks, stored.Checks)
	s.Equal(verdict.Manual, stored.Manual)
	s.Equal([]string{s.mainSHA}, s.stub.CompareRefs())
	s.Equal([][]string{{".stub/coverage.xml", ".stub/diff-cover.json", ".stub/mutation.json", "src/add.txt", addTestFile}}, s.stub.MutationFiles())
}

func (s *GateTestSuite) TestGate_FailingTestFails() {
	head := s.provenRed(map[string]string{"tests/broken_test.stub": "acme.core TestBroken fail\n"})

	s.requireOnlyFailing(s.runGate(head), gate.CheckTestsPass, "acme.core.TestBroken")
}

func (s *GateTestSuite) TestGate_TestThatFailsInTheSecondRunIsFlaky() {
	head := s.provenRed(map[string]string{"tests/race_test.stub": "acme.core TestRace flaky\n"})

	s.requireOnlyFailing(s.runGate(head), gate.CheckTestsPass, "flaky: acme.core.TestRace failed in one of two runs")
}

func (s *GateTestSuite) TestGate_TestsPassInBothRunsAtTheHead() {
	head := s.provenRed(nil)

	verdict := s.runGate(head)

	s.Equal(domain.CheckPass, checkOf(verdict, gate.CheckTestsPass).Status, checkOf(verdict, gate.CheckTestsPass).Detail)
	s.Subset(s.stub.TestOuts(), []string{"head", "head-again"})
}

func (s *GateTestSuite) TestGate_ErroringTestFails() {
	head := s.provenRed(map[string]string{"tests/broken_test.stub": "acme.core TestBroken error\n"})

	s.requireOnlyFailing(s.runGate(head), gate.CheckTestsPass, "acme.core.TestBroken")
}

func (s *GateTestSuite) TestGate_CriterionTestMissingFromJUnitFails() {
	head := s.provenRed(nil)
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{{ID: "AC1", Text: "adds two numbers", Tests: []engine.TestRef{
		{ID: "T1", Level: engine.LevelUnit, Kind: engine.KindHappy, File: addTestFile, JUnitName: testAdd},
		{ID: "T3", Level: engine.LevelUnit, Kind: engine.KindHappy, File: addTestFile, JUnitName: "acme.add.TestGhost"},
	}}}})

	s.requireOnlyFailing(s.runGate(head), gate.CheckCriteria, "acme.add.TestGhost")
}

func (s *GateTestSuite) TestGate_CriterionFileInNoRepoFails() {
	head := s.provenRed(nil)
	s.writeQATests(addTestFile, "tests/ghost_test.stub")
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{{ID: "AC1", Text: "adds two numbers", Tests: []engine.TestRef{
		{ID: "T1", Level: engine.LevelUnit, Kind: engine.KindHappy, File: addTestFile, JUnitName: testAdd},
		{ID: "T4", Level: engine.LevelUnit, Kind: engine.KindHappy, File: "tests/ghost_test.stub", JUnitName: "acme.ghost.TestGhost"},
	}}}})

	s.requireOnlyFailing(s.runGate(head), gate.CheckCriteria, "tests/ghost_test.stub")
}

func (s *GateTestSuite) TestGate_CriterionTestNotOwnedByQAFails() {
	head := s.provenRed(map[string]string{"tests/impl_test.stub": "acme.add TestImpl pass\n"})
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{{ID: "AC1", Text: "adds two numbers", Tests: []engine.TestRef{
		{ID: "T1", Level: engine.LevelUnit, Kind: engine.KindHappy, File: "tests/impl_test.stub", JUnitName: "acme.add.TestImpl"},
	}}}})

	s.requireOnlyFailing(s.runGate(head), gate.CheckCriteria, "not a QA test file")
}

func (s *GateTestSuite) TestGate_MissingCriteriaFileFails() {
	head := s.provenRed(nil)
	s.Require().NoError(os.Remove(filepath.Join(engine.RunFolder(s.base), "criteria.json")))

	s.requireOnlyFailing(s.runGate(head), gate.CheckCriteria, "criteria")
}

func (s *GateTestSuite) TestGate_DiffCoverageBelowFloorFails() {
	head := s.provenRed(map[string]string{".stub/diff-cover.json": `{"lines":10,"missing":5,"percent":50}`})

	s.requireOnlyFailing(s.runGate(head), gate.CheckDiffCoverage, "50.0%")
}

func (s *GateTestSuite) TestGate_NoChangedCoverableLineIsNotApplicable() {
	head := s.provenRed(map[string]string{".stub/diff-cover.json": `{"lines":0,"missing":0,"percent":0}`})

	verdict := s.runGate(head)

	s.True(verdict.Passed(), "%+v", verdict.Checks)
	s.Equal(domain.CheckNotApplicable, checkOf(verdict, gate.CheckDiffCoverage).Status)
}

func (s *GateTestSuite) TestGate_MutationScoreBelowFloorFails() {
	head := s.provenRed(map[string]string{".stub/mutation.json": `{"killed":1,"survived":9}`})

	s.requireOnlyFailing(s.runGate(head), gate.CheckMutation, "10.0%")
}

func (s *GateTestSuite) TestGate_NothingMutableIsNotApplicable() {
	files := implFiles()
	delete(files, ".stub/mutation.json")
	s.commitQA(qaTestsAdded, nil)
	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, s.qaSHA)
	s.Require().NoError(err)
	head := s.commit("implementation", files)

	verdict := s.runGate(head)

	s.True(verdict.Passed(), "%+v", verdict.Checks)
	s.Equal(domain.CheckNotApplicable, checkOf(verdict, gate.CheckMutation).Status)
}

func (s *GateTestSuite) TestMutation_TimeboxHitFails() {
	head := s.provenRed(map[string]string{".stub/mutation-timeout": "x\n"})

	s.requireOnlyFailing(s.runGate(head), gate.CheckMutation, "mutation timed out")
}

func (s *GateTestSuite) TestGate_FewerTestsThanMainFails() {
	head := s.provenRed(map[string]string{oldTestFile: ""})

	s.requireOnlyFailing(s.runGate(head), gate.CheckTestCount, "2 tests on the head, 3 on main")
}

func (s *GateTestSuite) TestGate_NewSkipMarkerFails() {
	head := s.provenRed(map[string]string{"src/later.go": "package src\n\nfunc later(t T) {\n\tt.Skip(\"later\")\n}\n"})

	s.requireOnlyFailing(s.runGate(head), gate.CheckTestCount, "src/later.go:4")
}

func (s *GateTestSuite) TestGate_NoRedProofFails() {
	s.commitQA(qaTestsAdded, nil)
	head := s.commitImpl(nil)

	s.requireOnlyFailing(s.runGate(head), gate.CheckRedGreen, "no red proof")
}

func (s *GateTestSuite) TestGate_DirtyTreeIsRefused() {
	head := s.provenRed(nil)
	s.Require().NoError(os.WriteFile(filepath.Join(s.checkout, "scratch.txt"), []byte("wip\n"), 0o644))

	_, err := s.uc.Run(s.Ctx, gateKey, gateRepo, head)

	s.Require().ErrorIs(err, gate.ErrRefused)
	s.Contains(err.Error(), "dirty")
	s.Equal(0, s.CountRows("verdict"))
}

func (s *GateTestSuite) TestGate_SHANotHeadIsRefused() {
	s.provenRed(nil)

	_, err := s.uc.Run(s.Ctx, gateKey, gateRepo, s.qaSHA)

	s.Require().ErrorIs(err, gate.ErrRefused)
	s.Contains(err.Error(), "HEAD")
	s.Equal(0, s.CountRows("verdict"))
}

func (s *GateTestSuite) TestGate_RepoOutsideTheProductIsRefused() {
	head := s.provenRed(nil)

	_, err := s.uc.Run(s.Ctx, gateKey, "other-repo", head)

	s.Require().Error(err)
	s.Contains(err.Error(), "other-repo")
	s.Equal(0, s.CountRows("verdict"))
}

func (s *GateTestSuite) TestGate_ProductNotInTheRegistryIsAClearError() {
	head := s.provenRed(nil)
	s.registry.Projects = nil
	s.uc = s.newGate(gateTimeout)

	_, err := s.uc.Run(s.Ctx, gateKey, gateRepo, head)

	s.Require().Error(err)
	s.Contains(err.Error(), fmt.Sprintf("product %s of run %s is not in the workspace registry", gateProduct, gateKey))
	s.Equal(0, s.CountRows("verdict"))
}

func (s *GateTestSuite) TestGate_ToolUnavailableCouldNotRun() {
	head := s.provenRed(map[string]string{".stub/probe-fails": "x\n"})

	verdict := s.runGate(head)

	s.False(verdict.Passed())
	s.Len(verdict.Checks, 6)
	for _, c := range verdict.Checks {
		s.Equal(domain.CheckCouldNotRun, c.Status, c.Name)
		s.Contains(c.Detail, "download failed")
	}
}

func (s *GateTestSuite) TestGate_TimeBoxFailsAndRemovesWorktrees() {
	head := s.provenRed(map[string]string{".stub/sleep": "x\n"})
	s.uc = s.newGate(time.Second)

	verdict := s.runGate(head)

	s.False(verdict.Passed())
	for _, c := range verdict.Checks {
		s.Equal(domain.CheckFail, c.Status, c.Name)
		s.Contains(c.Detail, "time box")
	}
}

func (s *GateTestSuite) TestGate_CancelRemovesWorktrees() {
	head := s.provenRed(map[string]string{".stub/sleep": "x\n"})
	ctx, cancel := context.WithTimeout(s.Ctx, 500*time.Millisecond)
	defer cancel()

	_, err := s.uc.Run(ctx, gateKey, gateRepo, head)

	s.Require().ErrorIs(err, context.DeadlineExceeded)
	s.Equal(0, s.CountRows("verdict"))
}

func (s *GateTestSuite) TestGateHandler_PassAnswersOKAndFailAnswersError() {
	head := s.provenRed(nil)
	handler := socket.NewGateHandler(s.uc)

	passed := handler(s.Ctx, socket.Request{Verb: socket.VerbGate, Key: string(gateKey), Repo: gateRepo, SHA: head})
	s.Equal(socket.CodeOK, passed.Code, passed.Error)
	s.Contains(passed.Output, head)

	failing := s.commit("drop a test", map[string]string{oldTestFile: ""})
	failed := handler(s.Ctx, socket.Request{Verb: socket.VerbGate, Key: string(gateKey), Repo: gateRepo, SHA: failing})
	s.Equal(socket.CodeError, failed.Code)
	s.Contains(failed.Output, gate.CheckTestCount)
}

func (s *GateTestSuite) TestRed_AssertionFailuresProveRed() {
	qa := s.commitQA(qaTestsAdded, nil)

	result, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)

	s.Require().NoError(err)
	s.Equal([]string{testAdd, testAddNeg}, result.TestIDs)
	repos, err := s.runs.Repos(s.Ctx, gateKey)
	s.Require().NoError(err)
	s.Require().Len(repos, 1)
	s.Equal(s.mainSHA, repos[0].RedBaseSHA)
	s.Equal(qa, repos[0].RedTestsSHA)
	s.Equal([]string{testAdd, testAddNeg}, repos[0].RedTestIDs)
}

func (s *GateTestSuite) TestRed_ErroringTestFails() {
	qa := s.commitQA("acme.add TestAdd compiles-with:src/add.txt\nacme.add TestAddRejectsNegative needs:src/add.txt\n", nil)

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)

	s.requireNotRed(err, testAdd, "error")
}

func (s *GateTestSuite) TestRed_TestMissingFromJUnitFails() {
	qa := s.commitQA("acme.add TestAdd needs:src/add.txt\n", nil)

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)

	s.requireNotRed(err, testAddNeg, "missing")
}

func (s *GateTestSuite) TestRed_TestAlreadyGreenOnTheBaseFails() {
	qa := s.commitQA("acme.add TestAdd needs:src/add.txt\nacme.add TestAddRejectsNegative pass\n", nil)

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)

	s.requireNotRed(err, testAddNeg, "passes")
}

func (s *GateTestSuite) TestRed_AppliesOnlyQATestFiles() {
	qa := s.commitQA(qaTestsAdded, map[string]string{"src/add.txt": "add\n"})

	result, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)

	s.Require().NoError(err)
	s.Equal([]string{testAdd, testAddNeg}, result.TestIDs)
}

func (s *GateTestSuite) TestRed_SameIDsMustPassOnTheHead() {
	s.commitQA("acme.add TestAdd needs:src/add.txt\nacme.add TestAddRejectsNegative needs:src/neg.txt\n", nil)
	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, s.qaSHA)
	s.Require().NoError(err)
	head := s.commitImpl(nil)

	verdict := s.runGate(head)

	s.False(verdict.Passed())
	redGreen := checkOf(verdict, gate.CheckRedGreen)
	s.Equal(domain.CheckFail, redGreen.Status)
	s.Contains(redGreen.Detail, testAddNeg)
	s.NotContains(redGreen.Detail, testAdd+" ")
}

func (s *GateTestSuite) TestRed_DirtyTreeIsRefused() {
	qa := s.commitQA(qaTestsAdded, nil)
	s.Require().NoError(os.WriteFile(filepath.Join(s.checkout, "src", "core.txt"), []byte("changed\n"), 0o644))

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)

	s.Require().ErrorIs(err, gate.ErrRefused)
	s.Contains(err.Error(), "dirty")
}

func (s *GateTestSuite) TestRed_SHANotHeadIsRefused() {
	s.commitQA(qaTestsAdded, nil)
	s.commitImpl(nil)

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, s.qaSHA)

	s.Require().ErrorIs(err, gate.ErrRefused)
	s.Contains(err.Error(), "HEAD")
	repos, err := s.runs.Repos(s.Ctx, gateKey)
	s.Require().NoError(err)
	s.Empty(repos)
}

func (s *GateTestSuite) requireNotRed(err error, id, reason string) {
	s.Require().ErrorIs(err, gate.ErrNotRed)
	for _, line := range strings.Split(err.Error(), "\n") {
		if strings.Contains(line, id) {
			s.Contains(line, reason)
			repos, rerr := s.runs.Repos(s.Ctx, gateKey)
			s.Require().NoError(rerr)
			s.Empty(repos)
			return
		}
	}
	s.Failf("no reason for the test", "%s not named in %q", id, err.Error())
}

// addWebRepo adds a second repo to the product whose clean checkout holds the only criterion test of the run.
func (s *GateTestSuite) addWebRepo() {
	origin := filepath.Join(s.root, "web.git")
	s.git(s.root, "init", "--quiet", "--bare", "-b", "main", origin)
	web := filepath.Join(s.base, "acme-web")
	s.git(s.root, "clone", "--quiet", origin, web)
	s.Require().NoError(os.MkdirAll(filepath.Join(web, "tests"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(web, "tests", "web_test.stub"), []byte("acme.web TestPage pass\n"), 0o644))
	s.git(web, "add", "-A")
	s.git(web, "commit", "--quiet", "-m", "web tests")
	s.registry.Projects[0].Repos = append(s.registry.Projects[0].Repos, config.Repo{Name: "acme-web", DefaultBranch: "main"})
	s.writeQATests("tests/web_test.stub")
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{{ID: "AC1", Text: "shows the page", Tests: []engine.TestRef{
		{ID: "T1", Level: engine.LevelE2E, Kind: engine.KindHappy, File: "tests/web_test.stub", JUnitName: "acme.web.TestPage"},
	}}}})
	s.uc = s.newGate(gateTimeout)
}

func (s *GateTestSuite) TestRed_QATestEditedAfterRedFailsRedGreen() {
	head := s.provenRed(map[string]string{addTestFile: "acme.add TestAdd pass\nacme.add TestAddRejectsNegative pass\n"})

	s.requireOnlyFailing(s.runGate(head), gate.CheckRedGreen, addTestFile)
}

func (s *GateTestSuite) TestRed_QATestRenamedAfterRedFailsRedGreen() {
	head := s.provenRed(map[string]string{addTestFile: "", "tests/sum_test.stub": qaTestsAdded})

	verdict := s.runGate(head)

	redGreen := checkOf(verdict, gate.CheckRedGreen)
	s.Equal(domain.CheckFail, redGreen.Status)
	s.Contains(redGreen.Detail, addTestFile)
}

func (s *GateTestSuite) TestRed_PanicOnTheBaseIsRefused() {
	qa := s.commitQA("acme.add TestAdd panic-unless:src/add.txt\nacme.add TestAddRejectsNegative needs:src/add.txt\n", nil)

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)

	s.requireNotRed(err, testAdd, "errored")
}

func (s *GateTestSuite) TestRed_ImportErrorOnTheBaseIsRefused() {
	qa := s.commitQA("acme.add TestAdd raise-unless:src/add.txt:ModuleNotFoundError\nacme.add TestAddRejectsNegative needs:src/add.txt\n", nil)

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)

	s.requireNotRed(err, testAdd, "errored")
}

func (s *GateTestSuite) TestRed_TypeErrorOnTheBaseIsRefused() {
	qa := s.commitQA("acme.add TestAdd needs:src/add.txt\nacme.add TestAddRejectsNegative raise-unless:src/add.txt:TypeError\n", nil)

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)

	s.requireNotRed(err, testAddNeg, "errored")
}

func (s *GateTestSuite) TestRed_SecondCallIsRefusedOutsideQA() {
	s.commitQA(qaTestsAdded, nil)
	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, s.qaSHA)
	s.Require().NoError(err)
	again := s.commit("more qa tests", map[string]string{"tests/more_test.stub": "acme.more TestMore pass\n"})

	_, err = s.uc.Red(s.Ctx, gateKey, gateRepo, again)

	s.Require().ErrorIs(err, gate.ErrRefused)
	s.Contains(err.Error(), "already")
	repos, err := s.runs.Repos(s.Ctx, gateKey)
	s.Require().NoError(err)
	s.Equal(s.qaSHA, repos[0].RedTestsSHA)
}

func (s *GateTestSuite) TestRed_SecondCallIsAllowedBackInQA() {
	s.commitQA(qaTestsAdded, nil)
	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, s.qaSHA)
	s.Require().NoError(err)
	s.Require().NoError(s.runs.SetPhase(s.Ctx, gateKey, "qa", s.Clock.Now()))
	again := s.commit("qa tests again", map[string]string{addTestFile: qaTestsAdded + "acme.add TestAddZero needs:src/add.txt\n"})

	red, err := s.uc.Red(s.Ctx, gateKey, gateRepo, again)

	s.Require().NoError(err)
	s.Equal(again, red.TestsSHA)
	s.Equal(s.mainSHA, red.BaseSHA)
}

func (s *GateTestSuite) TestGate_RepoWithoutCriterionTestsIsNotApplicableForRedGreen() {
	s.addWebRepo()
	head := s.commitImpl(nil)

	verdict := s.runGate(head)

	s.True(verdict.Passed(), "%+v", verdict.Checks)
	s.Equal(domain.CheckNotApplicable, checkOf(verdict, gate.CheckRedGreen).Status)
}

func (s *GateTestSuite) TestGate_ChangedFileMissingFromCoverageCountsAsUncovered() {
	head := s.provenRed(map[string]string{"src/orphan.code": "a\nb\nc\nd\ne\n"})

	s.requireOnlyFailing(s.runGate(head), gate.CheckDiffCoverage, "src/orphan.code")
}

func (s *GateTestSuite) TestGate_UnreportedCodeIsNotApplicableNever() {
	head := s.provenRed(map[string]string{
		"src/orphan.code":       "a\nb\n",
		".stub/diff-cover.json": `{"lines":0,"missing":0,"percent":0}`,
	})

	s.requireOnlyFailing(s.runGate(head), gate.CheckDiffCoverage, "src/orphan.code")
}

func (s *GateTestSuite) TestRed_ReProofInQAKeepsTheOriginalBase() {
	s.provenRed(nil)
	s.Require().NoError(s.runs.SetPhase(s.Ctx, gateKey, "qa", s.Clock.Now()))
	verified := s.commit("qa verify edits a proven-red test", map[string]string{
		addTestFile: qaTestsAdded + "acme.add TestAddZero needs:src/add.txt\n",
	})

	red, err := s.uc.Red(s.Ctx, gateKey, gateRepo, verified)

	s.Require().NoError(err)
	s.Equal(s.mainSHA, red.BaseSHA)
	s.Equal(verified, red.TestsSHA)
	s.Equal([]string{testAdd, testAddNeg}, red.TestIDs)
	repos, err := s.runs.Repos(s.Ctx, gateKey)
	s.Require().NoError(err)
	s.Equal(s.mainSHA, repos[0].RedBaseSHA)
	s.Equal(verified, repos[0].RedTestsSHA)
	verdict := s.runGate(verified)
	s.True(verdict.Passed(), "%+v", verdict.Checks)
	s.Equal(domain.CheckPass, checkOf(verdict, gate.CheckRedGreen).Status)
}

func (s *GateTestSuite) TestRed_ReProofAlreadyGreenOnTheOriginalBaseIsRefused() {
	s.provenRed(nil)
	s.Require().NoError(s.runs.SetPhase(s.Ctx, gateKey, "qa", s.Clock.Now()))
	verified := s.commit("qa verify weakens a proven-red test", map[string]string{
		addTestFile: "acme.add TestAdd needs:src/add.txt\nacme.add TestAddRejectsNegative pass\n",
	})

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, verified)

	s.Require().ErrorIs(err, gate.ErrNotRed)
	s.Contains(err.Error(), s.mainSHA)
	s.Contains(err.Error(), testAddNeg+" passes")
	repos, err := s.runs.Repos(s.Ctx, gateKey)
	s.Require().NoError(err)
	s.Equal(s.qaSHA, repos[0].RedTestsSHA)
}

func (s *GateTestSuite) TestRed_ReProofOffTheOriginalBaseIsRefused() {
	qa := s.commitQA(qaTestsAdded, nil)
	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, qa)
	s.Require().NoError(err)
	s.Require().NoError(s.runs.SetPhase(s.Ctx, gateKey, "qa", s.Clock.Now()))
	s.Require().NoError(s.runs.SetRedBase(s.Ctx, gateKey, domain.RedBase{Repo: gateRepo, BaseSHA: qa, TestsSHA: qa, TestIDs: []string{testAdd}}, s.Clock.Now()))
	s.git(s.checkout, "checkout", "--quiet", "-b", "elsewhere", s.mainSHA)
	elsewhere := s.commit("qa tests on another line", map[string]string{addTestFile: qaTestsAdded})

	_, err = s.uc.Red(s.Ctx, gateKey, gateRepo, elsewhere)

	s.Require().ErrorIs(err, gate.ErrRefused)
	s.Contains(err.Error(), "not an ancestor")
}

func (s *GateTestSuite) TestGate_LiveQAListCannotEscapeTheTamperDiff() {
	head := s.provenRed(nil)
	weak := "acme.add TestAdd pass\nacme.add TestAddRejectsNegative pass\n"
	head = s.commit("weaken and decoy", map[string]string{addTestFile: weak, "tests/decoy_test.stub": weak})
	s.writeQATests("tests/decoy_test.stub")
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{{ID: "AC1", Text: "adds two numbers", HasInputs: true, Tests: []engine.TestRef{
		{ID: "T1", Level: engine.LevelUnit, Kind: engine.KindHappy, File: "tests/decoy_test.stub", JUnitName: testAdd},
		{ID: "T2", Level: engine.LevelUnit, Kind: engine.KindNegative, File: "tests/decoy_test.stub", JUnitName: testAddNeg},
	}}}})

	verdict := s.runGate(head)

	s.False(verdict.Passed())
	s.Equal(domain.CheckFail, checkOf(verdict, gate.CheckRedGreen).Status)
	s.Contains(checkOf(verdict, gate.CheckRedGreen).Detail, addTestFile)
	s.Equal(domain.CheckFail, checkOf(verdict, gate.CheckCriteria).Status)
}

func (s *GateTestSuite) TestGate_QAListDroppingAStoredFileFailsRedGreen() {
	head := s.provenRed(nil)
	s.writeQATests("tests/other_test.stub")

	verdict := s.runGate(head)

	redGreen := checkOf(verdict, gate.CheckRedGreen)
	s.Equal(domain.CheckFail, redGreen.Status)
	s.Contains(redGreen.Detail, "qa-tests.json")
}

func (s *GateTestSuite) TestGate_CriteriaDroppingAStoredTestFailsCriteria() {
	head := s.provenRed(nil)
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{{ID: "AC1", Text: "adds two numbers", Tests: []engine.TestRef{
		{ID: "T1", Level: engine.LevelUnit, Kind: engine.KindHappy, File: addTestFile, JUnitName: testAdd},
	}}}})

	s.requireOnlyFailing(s.runGate(head), gate.CheckCriteria, "T2")
}

// reProofSetup proves red with a negative test the implementation does not satisfy, then moves the run back to QA.
func (s *GateTestSuite) reProofSetup() {
	s.commitQA("acme.add TestAdd needs:src/add.txt\nacme.add TestAddRejectsNegative needs:src/neg.txt\n", nil)
	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, s.qaSHA)
	s.Require().NoError(err)
	s.commitImpl(nil)
	s.Require().NoError(s.runs.SetPhase(s.Ctx, gateKey, "qa", s.Clock.Now()))
}

func (s *GateTestSuite) TestRed_ReProofDroppingAStoredRedIDIsRefused() {
	s.reProofSetup()
	dropped := s.commit("drop the failing test", map[string]string{addTestFile: "acme.add TestAdd needs:src/add.txt\n"})
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{{ID: "AC1", Text: "adds two numbers", Tests: []engine.TestRef{
		{ID: "T1", Level: engine.LevelUnit, Kind: engine.KindHappy, File: addTestFile, JUnitName: testAdd},
	}}}})

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, dropped)

	s.Require().ErrorIs(err, gate.ErrRefused)
	s.Contains(err.Error(), testAddNeg)
	repos, err := s.runs.Repos(s.Ctx, gateKey)
	s.Require().NoError(err)
	s.Equal(s.qaSHA, repos[0].RedTestsSHA)
}

func (s *GateTestSuite) TestRed_ReProofDroppingAStoredCriteriaTestIsRefused() {
	s.reProofSetup()
	renamed := s.commit("rename the failing test", map[string]string{addTestFile: "acme.add TestAdd needs:src/add.txt\nacme.add TestAddOther needs:src/neg.txt\nacme.add TestAddRejectsNegative needs:src/neg.txt\n"})
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{{ID: "AC1", Text: "adds two numbers", HasInputs: true, Tests: []engine.TestRef{
		{ID: "T1", Level: engine.LevelUnit, Kind: engine.KindHappy, File: addTestFile, JUnitName: testAdd},
		{ID: "T3", Level: engine.LevelUnit, Kind: engine.KindNegative, File: addTestFile, JUnitName: testAddNeg},
		{ID: "T4", Level: engine.LevelUnit, Kind: engine.KindNegative, File: addTestFile, JUnitName: "acme.add.TestAddOther"},
	}}}})

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, renamed)

	s.Require().ErrorIs(err, gate.ErrRefused)
	s.Contains(err.Error(), "T2")
}

func (s *GateTestSuite) TestRed_ReProofIsListedInTheVerdict() {
	s.reProofSetup()
	weakened := s.commit("weaken the failing test", map[string]string{addTestFile: "acme.add TestAdd needs:src/add.txt\nacme.add TestAddRejectsNegative needs:src/add.txt\n"})

	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, weakened)
	s.Require().NoError(err)
	verdict := s.runGate(weakened)

	s.Require().Len(verdict.RedReProofs, 1)
	s.Contains(verdict.RedReProofs[0], weakened)
	s.Contains(verdict.RedReProofs[0], addTestFile)
	stored, err := s.verdicts.Latest(s.Ctx, gateKey, gateRepo)
	s.Require().NoError(err)
	s.Equal(verdict.RedReProofs, stored.RedReProofs)
}

func (s *GateTestSuite) TestGate_NewQAFileAfterRedKeepsRedGreen() {
	head := s.provenRed(map[string]string{"tests/extra_test.stub": "acme.add TestAddExtra pass\n"})
	s.writeQATests(addTestFile, "tests/extra_test.stub")

	verdict := s.runGate(head)

	s.True(verdict.Passed(), "%+v", verdict.Checks)
	s.Equal(domain.CheckPass, checkOf(verdict, gate.CheckRedGreen).Status)
}

func (s *GateTestSuite) TestGate_DotSlashPathsInTheRunFilesPass() {
	s.writeQATests("./" + addTestFile)
	s.writeCriteria(engine.Criteria{Criteria: []engine.Criterion{{ID: "AC1", Text: "adds two numbers", HasInputs: true, Tests: []engine.TestRef{
		{ID: "T1", Level: engine.LevelUnit, Kind: engine.KindHappy, File: "./" + addTestFile, JUnitName: testAdd},
		{ID: "T2", Level: engine.LevelUnit, Kind: engine.KindNegative, File: "./" + addTestFile, JUnitName: testAddNeg},
	}}}})
	head := s.provenRed(nil)

	verdict := s.runGate(head)

	s.True(verdict.Passed(), "%+v", verdict.Checks)
	s.Equal(domain.CheckPass, checkOf(verdict, gate.CheckCriteria).Status)
}

func (s *GateTestSuite) TestRed_ReProofDroppingAStoredQAFileIsRefused() {
	const helperFile = "tests/helper.stub"
	s.writeQATests(addTestFile, helperFile)
	s.commitQA(qaTestsAdded, map[string]string{helperFile: "strict helper\n"})
	_, err := s.uc.Red(s.Ctx, gateKey, gateRepo, s.qaSHA)
	s.Require().NoError(err)
	head := s.commitImpl(nil)
	s.Require().NoError(s.runs.SetPhase(s.Ctx, gateKey, "qa", s.Clock.Now()))
	s.writeQATests(addTestFile)

	_, err = s.uc.Red(s.Ctx, gateKey, gateRepo, head)

	s.Require().ErrorIs(err, gate.ErrRefused)
	s.Contains(err.Error(), helperFile)
	repos, err := s.runs.Repos(s.Ctx, gateKey)
	s.Require().NoError(err)
	s.Equal(s.qaSHA, repos[0].RedTestsSHA)
	s.Equal([]string{addTestFile, helperFile}, repos[0].RedQAFiles)
}
