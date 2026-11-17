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

type VerdictRepositoryTestSuite struct {
	tests.Suite
	verdicts *repository.Verdict
}

func TestVerdictRepositoryTestSuite(t *testing.T) {
	suite.Run(t, &VerdictRepositoryTestSuite{})
}

func (s *VerdictRepositoryTestSuite) SetupTest() {
	s.Suite.SetupTest()
	s.verdicts = repository.NewVerdict(s.DB)
	_, err := repository.NewRun(s.DB).Create(s.Ctx, domain.Run{
		IssueKey: "ACM-1", Product: "widgets", ChannelID: "C1", RequestTS: "1.1", ThreadTS: "1.1", Phase: domain.Phase("planning"),
	}, s.Clock.Now())
	s.Require().NoError(err)
}

func (s *VerdictRepositoryTestSuite) TestLatestIsTheNewestForRepo() {
	s.Require().NoError(s.verdicts.Save(s.Ctx, domain.Verdict{
		IssueKey: "ACM-1", Repo: "acme-api", SHA: "s1",
		Checks: []domain.CheckResult{{Name: "mutation", Status: domain.CheckFail, Detail: "mutation timed out"}},
	}, s.Clock.Now()))
	s.Clock.Advance(time.Minute)
	s.Require().NoError(s.verdicts.Save(s.Ctx, domain.Verdict{
		IssueKey: "ACM-1", Repo: "acme-api", SHA: "s2",
		Checks: []domain.CheckResult{{Name: "mutation", Status: domain.CheckPass}, {Name: "coverage", Status: domain.CheckNotApplicable}},
	}, s.Clock.Now()))
	s.Require().NoError(s.verdicts.Save(s.Ctx, domain.Verdict{
		IssueKey: "ACM-1", Repo: "acme-web", SHA: "w1",
	}, s.Clock.Now()))

	latest, err := s.verdicts.Latest(s.Ctx, "ACM-1", "acme-api")
	s.Require().NoError(err)
	s.Equal(domain.Verdict{
		IssueKey: "ACM-1", Repo: "acme-api", SHA: "s2",
		Checks:    []domain.CheckResult{{Name: "mutation", Status: domain.CheckPass}, {Name: "coverage", Status: domain.CheckNotApplicable}},
		CreatedAt: s.Clock.Now(),
	}, latest)

	all, err := s.verdicts.List(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Len(all, 3)
}

func (s *VerdictRepositoryTestSuite) TestLatestMissingIsNotFound() {
	_, err := s.verdicts.Latest(s.Ctx, "ACM-1", "acme-api")
	s.True(errors.Is(err, domain.ErrNotFound))
}
