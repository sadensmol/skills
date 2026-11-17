package tests_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/tests"
)

type RunRepositoryTestSuite struct {
	tests.Suite
	runs *repository.Run
}

func TestRunRepositoryTestSuite(t *testing.T) {
	suite.Run(t, &RunRepositoryTestSuite{})
}

func (s *RunRepositoryTestSuite) SetupTest() {
	s.Suite.SetupTest()
	s.runs = repository.NewRun(s.DB)
}

func (s *RunRepositoryTestSuite) create(key domain.IssueKey, channel, ts string) {
	_, err := s.runs.Create(s.Ctx, domain.Run{
		IssueKey: key, Product: "widgets", ChannelID: channel, RequestTS: ts, ThreadTS: ts, Phase: domain.Phase("planning"),
	}, s.Clock.Now())
	s.Require().NoError(err)
}

func (s *RunRepositoryTestSuite) TestCreateAndFind() {
	s.create("ACM-1", "C1", "10.1")

	run, err := s.runs.FindByKey(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(domain.Run{
		IssueKey: "ACM-1", Product: "widgets", ChannelID: "C1", RequestTS: "10.1", ThreadTS: "10.1",
		Phase: domain.Phase("planning"), CreatedAt: s.Clock.Now(),
	}, run)

	byRequest, err := s.runs.FindByRequest(s.Ctx, "C1", "10.1")
	s.Require().NoError(err)
	s.Equal(domain.IssueKey("ACM-1"), byRequest.IssueKey)
}

func (s *RunRepositoryTestSuite) TestFindMissingIsNotFound() {
	_, err := s.runs.FindByKey(s.Ctx, "ACM-9")
	s.True(errors.Is(err, domain.ErrNotFound))
	_, err = s.runs.FindByRequest(s.Ctx, "C1", "10.1")
	s.True(errors.Is(err, domain.ErrNotFound))
}

func (s *RunRepositoryTestSuite) TestSameRequestTwiceIsRefused() {
	s.create("ACM-1", "C1", "10.1")

	_, err := s.runs.Create(s.Ctx, domain.Run{
		IssueKey: "ACM-2", Product: "widgets", ChannelID: "C1", RequestTS: "10.1", ThreadTS: "10.1", Phase: domain.Phase("planning"),
	}, s.Clock.Now())
	s.True(errors.Is(err, domain.ErrAlreadyExists))
	s.Equal(1, s.CountRows("run"))

	s.create("ACM-3", "C2", "10.1") // same ts in another channel is a different request
}

func (s *RunRepositoryTestSuite) TestSameIssueKeyTwiceIsRefused() {
	s.create("ACM-1", "C1", "10.1")

	_, err := s.runs.Create(s.Ctx, domain.Run{
		IssueKey: "ACM-1", Product: "widgets", ChannelID: "C1", RequestTS: "10.2", ThreadTS: "10.2", Phase: domain.Phase("planning"),
	}, s.Clock.Now())
	s.True(errors.Is(err, domain.ErrAlreadyExists))
}

func (s *RunRepositoryTestSuite) TestSetPhaseKeepsPreviousPhase() {
	s.create("ACM-1", "C1", "10.1")

	s.Require().NoError(s.runs.SetPhase(s.Ctx, "ACM-1", domain.Phase("qa"), s.Clock.Now()))
	s.Require().NoError(s.runs.SetPhase(s.Ctx, "ACM-1", domain.Phase("blocked"), s.Clock.Now()))

	run, err := s.runs.FindByKey(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(domain.Phase("blocked"), run.Phase)
	s.Equal(domain.Phase("qa"), run.PreviousPhase)

	s.True(errors.Is(s.runs.SetPhase(s.Ctx, "ACM-9", domain.Phase("qa"), s.Clock.Now()), domain.ErrNotFound))
}

func (s *RunRepositoryTestSuite) TestSessionCardAndLastPost() {
	s.create("ACM-1", "C1", "10.1")
	later := s.Clock.Now().Add(time.Hour)

	s.Require().NoError(s.runs.SetCardTS(s.Ctx, "ACM-1", "10.2", s.Clock.Now()))
	s.Require().NoError(s.runs.SetSession(s.Ctx, "ACM-1", "sess-1", "/work/runs/ACM-1", s.Clock.Now()))
	s.Require().NoError(s.runs.SetLastPostAt(s.Ctx, "ACM-1", later))

	run, err := s.runs.FindByKey(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal("10.2", run.CardTS)
	s.Equal("sess-1", run.SessionID)
	s.Equal("/work/runs/ACM-1", run.WorktreeBase)
	s.Equal(later, run.LastPostAt)
}

func (s *RunRepositoryTestSuite) TestEndAndListActive() {
	s.create("ACM-1", "C1", "10.1")
	s.create("ACM-2", "C1", "10.2")

	s.Require().NoError(s.runs.End(s.Ctx, "ACM-1", s.Clock.Now()))

	active, err := s.runs.ListActive(s.Ctx)
	s.Require().NoError(err)
	s.Require().Len(active, 1)
	s.Equal(domain.IssueKey("ACM-2"), active[0].IssueKey)

	ended, err := s.runs.FindByKey(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(s.Clock.Now(), ended.EndedAt)
}

func (s *RunRepositoryTestSuite) TestLoopCountsGrowPerLoop() {
	s.create("ACM-1", "C1", "10.1")

	n, err := s.runs.IncLoop(s.Ctx, "ACM-1", domain.LoopGate, s.Clock.Now())
	s.Require().NoError(err)
	s.Equal(1, n)
	n, err = s.runs.IncLoop(s.Ctx, "ACM-1", domain.LoopGate, s.Clock.Now())
	s.Require().NoError(err)
	s.Equal(2, n)
	_, err = s.runs.IncLoop(s.Ctx, "ACM-1", domain.LoopReview, s.Clock.Now())
	s.Require().NoError(err)

	loops, err := s.runs.Loops(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(map[domain.Loop]int{domain.LoopGate: 2, domain.LoopReview: 1}, loops)
}

func (s *RunRepositoryTestSuite) TestRedBaseAndPRWatchPerRepo() {
	s.create("ACM-1", "C1", "10.1")
	now := s.Clock.Now()

	s.Require().NoError(s.runs.SetRedBase(s.Ctx, "ACM-1", domain.RedBase{Repo: "acme-api", BaseSHA: "a1", TestsSHA: "t1"}, now))
	s.Require().NoError(s.runs.SetRedBase(s.Ctx, "ACM-1", domain.RedBase{Repo: "acme-web", BaseSHA: "a2", TestsSHA: "t2"}, now))
	s.Require().NoError(s.runs.SetRedBase(s.Ctx, "ACM-1", domain.RedBase{Repo: "acme-api", BaseSHA: "a3", TestsSHA: "t3"}, now)) // red again replaces
	s.Require().NoError(s.runs.SetPR(s.Ctx, "ACM-1", "acme-api", "https://example.com/acme/acme-api/pull/7", now))
	s.Require().NoError(s.runs.SetPRWatch(s.Ctx, "ACM-1", "acme-api", domain.PRWatch{State: "open", LastCommentID: "99", CheckedAt: now}, now))

	repos, err := s.runs.Repos(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal([]domain.RunRepo{
		{
			Repo: "acme-api", RedBaseSHA: "a3", RedTestsSHA: "t3", PRURL: "https://example.com/acme/acme-api/pull/7",
			Watch: domain.PRWatch{State: "open", LastCommentID: "99", CheckedAt: now},
		},
		{Repo: "acme-web", RedBaseSHA: "a2", RedTestsSHA: "t2"},
	}, repos)

}

func (s *RunRepositoryTestSuite) TestPRWithoutARedProofCreatesTheRepoRow() {
	s.create("ACM-1", "C1", "10.1")
	now := s.Clock.Now()

	s.Require().NoError(s.runs.SetPR(s.Ctx, "ACM-1", "acme-cfg", "https://example.com/acme/acme-cfg/pull/3", now))
	s.Require().NoError(s.runs.SetPRWatch(s.Ctx, "ACM-1", "acme-cfg", domain.PRWatch{State: "open"}, now))
	s.Require().NoError(s.runs.SetPRWatch(s.Ctx, "ACM-1", "acme-none", domain.PRWatch{State: "open"}, now))

	repos, err := s.runs.Repos(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Require().Len(repos, 2)
	s.Equal(domain.RunRepo{Repo: "acme-cfg", PRURL: "https://example.com/acme/acme-cfg/pull/3", Watch: domain.PRWatch{State: "open"}}, repos[0])
	var updatedAt string
	s.Require().NoError(s.DB.QueryRowContext(s.Ctx, `SELECT updated_at FROM run_repo WHERE repo = 'acme-cfg'`).Scan(&updatedAt))
	s.Equal(now.UTC().Format(time.RFC3339), updatedAt, "a watch without a check time still stamps the row")
}

func (s *RunRepositoryTestSuite) TestMergeAndDeployStatePerRepo() {
	s.create("ACM-1", "C1", "10.1")
	merged := domain.Merge{Branch: "acm-1-x", SHA: "abc", DeployState: domain.DeployPending, At: s.Clock.Now()}
	s.Require().NoError(s.runs.SetMerge(s.Ctx, "ACM-1", "acme-api", merged, s.Clock.Now()))
	s.Require().NoError(s.runs.SetMerge(s.Ctx, "ACM-1", "acme-web", domain.Merge{Branch: "acm-1-x", SHA: "def", At: s.Clock.Now()}, s.Clock.Now()))

	pending, err := s.runs.PendingDeploys(s.Ctx)
	s.Require().NoError(err)
	s.Require().Len(pending, 1)
	s.Equal(domain.IssueKey("ACM-1"), pending[0].Run.IssueKey)
	s.Equal(domain.RunRepo{Repo: "acme-api", Merge: merged}, pending[0].Repo)

	s.Require().NoError(s.runs.SetDeployState(s.Ctx, "ACM-1", "acme-api", domain.DeployDeployed, s.Clock.Now()))

	pending, err = s.runs.PendingDeploys(s.Ctx)
	s.Require().NoError(err)
	s.Empty(pending)
	repos, err := s.runs.Repos(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(domain.DeployDeployed, repos[0].Merge.DeployState)
	s.Empty(repos[1].Merge.DeployState)
}
