package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gh"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/internal/usecase"
)

const (
	fixURL      = "https://github.com/acme/acme-api/pull/12"
	fixMerged   = "pr\tmerge\t" + fixURL + "\t--squash\t--delete-branch\n"
	fixLine     = "🔧 ACM-9: merged a fix for the default branch: " + fixURL
	testCheck   = `{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://ci.example/test"}`
	lintSkipped = `{"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"SKIPPED","detailsUrl":"https://ci.example/lint"}`
	statusGreen = `{"__typename":"StatusContext","context":"ci/build","state":"SUCCESS","targetUrl":"https://ci.example/build"}`
	testRunning = `{"__typename":"CheckRun","name":"test","status":"IN_PROGRESS","conclusion":"","detailsUrl":"https://ci.example/test"}`
	testFailed  = `{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://ci.example/test"}`
)

type FixMergeTestSuite struct {
	statusBase
	ghDir string
	// onSleep runs after the fake clock moved; tests use it to change the PR while the merge waits.
	onSleep func()
}

func TestFixMergeTestSuite(t *testing.T) {
	suite.Run(t, new(FixMergeTestSuite))
}

func (s *FixMergeTestSuite) SetupTest() {
	s.statusBase.SetupTest()
	s.onSleep = nil
	s.ghDir = s.T().TempDir()
	s.T().Setenv("FAKE_GH_DIR", s.ghDir)
	s.writeGH("repo.json", `{"allow_merge_commit":false,"allow_squash_merge":true,"allow_rebase_merge":true}`)
	s.Require().NoError(s.runs.SetPR(s.Ctx, statusRunKey, prRepo, prURL, s.Clock.Now()))
	s.Require().NoError(s.runs.SetPRWatch(s.Ctx, statusRunKey, prRepo, domain.PRWatch{State: "merged"}, s.Clock.Now()))
	s.deployIs(domain.DeployFaulted)
	s.Require().NoError(s.runs.End(s.Ctx, statusRunKey, s.Clock.Now()))
}

func (s *FixMergeTestSuite) deployIs(state domain.DeployState) {
	s.Require().NoError(s.runs.SetMerge(s.Ctx, statusRunKey, prRepo, domain.Merge{
		Branch: prBranch, SHA: mergeSHA, DeployState: state, At: s.Clock.Now(),
	}, s.Clock.Now()))
}

func (s *FixMergeTestSuite) writeGH(name, body string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.ghDir, name), []byte(body), 0o644))
}

// fixView sets what gh pr view reports for the fix PR.
func (s *FixMergeTestSuite) fixView(state, base string, checks string) {
	s.writeGH("view-12.json", `{"state":"`+state+`","baseRefName":"`+base+`","reviews":[],"comments":[],"statusCheckRollup":[`+checks+`]}`)
}

func (s *FixMergeTestSuite) ghLog() string {
	data, err := os.ReadFile(filepath.Join(s.ghDir, "argv.log"))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	s.Require().NoError(err)
	return string(data)
}

func (s *FixMergeTestSuite) mergeFix(url string) error {
	bin, err := filepath.Abs(filepath.Join("testdata", "fake-gh.sh"))
	s.Require().NoError(err)
	m := usecase.NewFixMerge(usecase.FixMergeData{
		Registry: testRegistry(),
		GH:       gh.New(bin, time.Second),
		Slack:    slackpost.NewPoster(s.slack.API()),
		Runs:     s.runs,
		Now:      s.Clock.Now,
		Sleep: func(_ context.Context, d time.Duration) error {
			s.Clock.Advance(d)
			if s.onSleep != nil {
				s.onSleep()
			}
			return nil
		},
	})
	return m.Merge(s.Ctx, statusRunKey, url)
}

func (s *FixMergeTestSuite) requireNoMerge() {
	s.NotContains(s.ghLog(), "pr\tmerge")
	s.Empty(s.slack.Posts())
}

func (s *FixMergeTestSuite) TestGreenChecksMergeTheFixAndPostTheLine() {
	s.fixView("OPEN", "main", testCheck+","+lintSkipped+","+statusGreen)

	s.Require().NoError(s.mergeFix(fixURL))

	s.Contains(s.ghLog(), fixMerged)
	s.Equal([]string{fixLine}, s.texts())
	repos, err := s.runs.Repos(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Equal(domain.DeployFaulted, repos[0].Merge.DeployState, "the deploy watch follows the default branch by itself")
}

func (s *FixMergeTestSuite) TestPendingAfterRerunAcceptsTheFix() {
	s.deployIs(domain.DeployPending)
	s.Require().NoError(s.runs.SetDeployRetried(s.Ctx, statusRunKey, prRepo, s.Clock.Now()))
	s.fixView("OPEN", "main", testCheck)

	s.Require().NoError(s.mergeFix(fixURL))

	s.Contains(s.ghLog(), fixMerged)
}

func (s *FixMergeTestSuite) TestRunningChecksAreAwaitedThenMerged() {
	s.fixView("OPEN", "main", testRunning)
	s.onSleep = func() { s.fixView("OPEN", "main", testCheck) }
	started := s.Clock.Now()

	s.Require().NoError(s.mergeFix(fixURL))

	s.Equal(30*time.Second, s.Clock.Now().Sub(started))
	s.Contains(s.ghLog(), fixMerged)
	s.Equal([]string{fixLine}, s.texts())
}

func (s *FixMergeTestSuite) TestFailedCheckNamesItWithoutMerge() {
	s.fixView("OPEN", "main", testFailed+","+statusGreen)

	err := s.mergeFix(fixURL)

	s.Require().Error(err)
	s.Contains(err.Error(), "test (https://ci.example/test)")
	s.NotContains(err.Error(), "ci/build")
	s.requireNoMerge()
}

func (s *FixMergeTestSuite) TestChecksNotGreenInThirtyMinutesEndWithoutMerge() {
	s.fixView("OPEN", "main", testRunning)
	started := s.Clock.Now()

	s.Require().Error(s.mergeFix(fixURL))

	s.Equal(30*time.Minute, s.Clock.Now().Sub(started))
	s.requireNoMerge()
}

func (s *FixMergeTestSuite) TestNoChecksYetIsNotGreen() {
	s.fixView("OPEN", "main", "")

	s.Require().Error(s.mergeFix(fixURL))

	s.requireNoMerge()
}

func (s *FixMergeTestSuite) TestPRInAnotherRepoIsRefused() {
	s.fixView("OPEN", "main", testCheck)

	s.Require().Error(s.mergeFix("https://github.com/acme/acme-web/pull/12"))

	s.requireNoMerge()
}

func (s *FixMergeTestSuite) TestPROnAnotherBranchIsRefused() {
	s.fixView("OPEN", "release", testCheck)

	s.Require().Error(s.mergeFix(fixURL))

	s.requireNoMerge()
}

func (s *FixMergeTestSuite) TestClosedPRIsRefused() {
	s.fixView("CLOSED", "main", testCheck)

	s.Require().Error(s.mergeFix(fixURL))

	s.requireNoMerge()
}

func (s *FixMergeTestSuite) TestNotAPlainPRURLIsRefused() {
	s.fixView("OPEN", "main", testCheck)

	s.Require().Error(s.mergeFix("https://example.test/acme/acme-api/pull/12"))

	s.requireNoMerge()
}

func (s *FixMergeTestSuite) TestDeployThatDidNotFailIsRefused() {
	for _, state := range []domain.DeployState{domain.DeployDeployed, domain.DeployPending} {
		s.deployIs(state)
		s.fixView("OPEN", "main", testCheck)

		s.Require().Error(s.mergeFix(fixURL), state)

		s.requireNoMerge()
	}
}
