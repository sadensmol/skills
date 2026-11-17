package tests_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/tests"
)

const migrationsCount = 9

type StoreTestSuite struct {
	tests.Suite
}

func TestStoreTestSuite(t *testing.T) {
	suite.Run(t, &StoreTestSuite{})
}

func (s *StoreTestSuite) TestMigrationsApplyOnEmptyDB() {
	var version int
	s.Require().NoError(s.DB.QueryRowContext(s.Ctx, `SELECT MAX(version_id) FROM goose_db_version`).Scan(&version))
	s.Equal(migrationsCount, version)

	for _, table := range []string{"run", "run_loop", "run_repo", "ask", "verdict"} {
		s.Zero(s.CountRows(table), table)
	}
}

func (s *StoreTestSuite) TestDataSurvivesRestart() {
	runs := repository.NewRun(s.DB)
	asks := repository.NewAsk(s.DB)
	verdicts := repository.NewVerdict(s.DB)
	now := s.Clock.Now()

	_, err := runs.Create(s.Ctx, domain.Run{
		IssueKey: "ACM-1", Product: "widgets", ChannelID: "C1", RequestTS: "1.1", ThreadTS: "1.1", Phase: domain.Phase("planning"),
	}, now)
	s.Require().NoError(err)
	s.Require().NoError(runs.SetRedBase(s.Ctx, "ACM-1", domain.RedBase{Repo: "acme-api", BaseSHA: "aaa", TestsSHA: "bbb"}, now))
	_, err = runs.IncLoop(s.Ctx, "ACM-1", domain.LoopGate, now)
	s.Require().NoError(err)
	_, err = asks.Create(s.Ctx, repository.CreateAskData{IssueKey: "ACM-1", Question: "which db?", Now: now})
	s.Require().NoError(err)
	s.Require().NoError(verdicts.Save(s.Ctx, domain.Verdict{
		IssueKey: "ACM-1", Repo: "acme-api", SHA: "bbb", Checks: []domain.CheckResult{{Name: "mutation", Status: domain.CheckPass}},
	}, now))

	s.Require().NoError(s.DB.Close())
	reopened, err := repository.Open(s.Ctx, s.DBPath)
	s.Require().NoError(err)
	s.DB = reopened

	var version int
	s.Require().NoError(s.DB.QueryRowContext(s.Ctx, `SELECT MAX(version_id) FROM goose_db_version`).Scan(&version))
	s.Equal(migrationsCount, version)

	run, err := repository.NewRun(s.DB).FindByKey(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(domain.Phase("planning"), run.Phase)
	s.Equal(now.UTC(), run.CreatedAt)
	reds, err := repository.NewRun(s.DB).Repos(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal([]domain.RunRepo{{Repo: "acme-api", RedBaseSHA: "aaa", RedTestsSHA: "bbb"}}, reds)
	loops, err := repository.NewRun(s.DB).Loops(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(map[domain.Loop]int{domain.LoopGate: 1}, loops)
	open, err := repository.NewAsk(s.DB).FindOpen(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal("which db?", open.Question)
	latest, err := repository.NewVerdict(s.DB).Latest(s.Ctx, "ACM-1", "acme-api")
	s.Require().NoError(err)
	s.Equal("bbb", latest.SHA)
}
