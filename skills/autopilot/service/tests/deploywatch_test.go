package tests

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gh"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/shell"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/internal/usecase"
)

const (
	mergeSHA  = "abc1234def5678"
	fixSHA    = "fed9876cba5432"
	deployJob = "deploy"
	runURL    = "https://github.com/acme/acme-api/actions/runs/11"
	jobURL    = "https://github.com/acme/acme-api/actions/runs/11/job/2"
	liveLine  = "✅ ACM-9 is live (abc1234). Ready to test."
	faultFix  = "; fix: find why CI on main fails at abc1234, fix it with a PR, then run autopilot merge-fix ACM-9 <PR url>; the deploy check then passes"
)

type DeployWatchTestSuite struct {
	statusBase
	ghDir     string
	agtermDir string
	registry  config.Registry
}

func TestDeployWatchTestSuite(t *testing.T) {
	suite.Run(t, new(DeployWatchTestSuite))
}

// deployRegistry is the test registry with an automatic deploy for its one repo.
func deployRegistry(deploy config.RepoDeploy) config.Registry {
	r := testRegistry()
	r.AgtermWorkspace = "acme"
	r.Projects[0].Repos[0].Deploy = deploy
	return r
}

func (s *DeployWatchTestSuite) SetupTest() {
	s.statusBase.SetupTest()
	s.ghDir = s.T().TempDir()
	s.T().Setenv("FAKE_GH_DIR", s.ghDir)
	s.agtermDir = os.Getenv("FAKE_AGTERM_DIR")
	s.Require().NoError(os.WriteFile(filepath.Join(s.agtermDir, "tree.json"), []byte(treeControlOnly), 0o644))
	s.registry = deployRegistry(config.RepoDeploy{Job: deployJob, Timeout: time.Hour})
	s.merge(prRepo, prURL)
	s.Require().NoError(s.runs.End(s.Ctx, statusRunKey, s.Clock.Now()))
}

func (s *DeployWatchTestSuite) merge(repo, url string) {
	s.Require().NoError(s.runs.SetPR(s.Ctx, statusRunKey, repo, url, s.Clock.Now()))
	s.Require().NoError(s.runs.SetPRWatch(s.Ctx, statusRunKey, repo, domain.PRWatch{State: "merged"}, s.Clock.Now()))
	s.Require().NoError(s.runs.SetMerge(s.Ctx, statusRunKey, repo, domain.Merge{
		Branch: prBranch, SHA: mergeSHA, DeployState: domain.DeployPending, At: s.Clock.Now(),
	}, s.Clock.Now()))
}

func (s *DeployWatchTestSuite) writeGH(name, body string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.ghDir, name), []byte(body), 0o644))
}

// deployJobIs makes the one workflow run of the merge commit hold a passed test job and the deploy job.
func (s *DeployWatchTestSuite) deployJobIs(status, conclusion string) {
	s.ciRunIs("success", status, conclusion)
}

// ciRunIs makes the one workflow run of the merge commit hold a completed test job and the deploy job.
func (s *DeployWatchTestSuite) ciRunIs(test, status, conclusion string) {
	s.writeGH("runs.json", `[{"databaseId":11}]`)
	s.writeGH("run-11.json", `{"url":"`+runURL+`","jobs":[{"name":"test","status":"completed","conclusion":"`+test+`","url":"https://x"},`+
		`{"name":"`+deployJob+`","status":"`+status+`","conclusion":"`+conclusion+`","url":"`+jobURL+`"}]}`)
}

// calls lists the argv of each call a fake made, from its log in dir.
func (s *DeployWatchTestSuite) calls(dir string, verb ...string) [][]string {
	data, err := os.ReadFile(filepath.Join(dir, "argv.log"))
	if os.IsNotExist(err) {
		return nil
	}
	s.Require().NoError(err)
	var out [][]string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if c := strings.Split(line, "\t"); len(c) >= len(verb) && slices.Equal(c[:len(verb)], verb) {
			out = append(out, c)
		}
	}
	return out
}

func (s *DeployWatchTestSuite) typed() []string {
	var out []string
	for _, c := range s.calls(s.agtermDir, "session", "type") {
		out = append(out, c[len(c)-1])
	}
	return out
}

// fault fails the deploy twice: the first failure re-runs the failed jobs, the second one is stuck.
func (s *DeployWatchTestSuite) fault() {
	s.ciRunIs("failure", "completed", "skipped")
	s.tick()
	s.tick()
	s.Require().Equal(domain.DeployFaulted, s.deployState())
}

func (s *DeployWatchTestSuite) tick() {
	bin, err := filepath.Abs(filepath.Join("testdata", "fake-gh.sh"))
	s.Require().NoError(err)
	agtermBin, err := filepath.Abs(filepath.Join("testdata", "fake-agtermctl.sh"))
	s.Require().NoError(err)
	watch := usecase.NewDeployWatch(usecase.DeployWatchData{
		Registry:        s.registry,
		Interval:        5 * time.Minute,
		Linear:          linear.NewClient(s.linear.URL(), "k", nil),
		Slack:           slackpost.NewPoster(s.slack.API()),
		Runs:            s.runs,
		GH:              gh.New(bin, time.Second),
		Checks:          shell.New(),
		AgtermWorkspace: s.registry.AgtermWorkspace,
		Sessions:        agterm.New(agtermBin, "/tmp/ap-test.sock"),
		Now:             s.Clock.Now,
	})
	s.Require().NoError(watch.Tick(s.Ctx))
}

func (s *DeployWatchTestSuite) deployState() domain.DeployState {
	repos, err := s.runs.Repos(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Require().NotEmpty(repos)
	return repos[0].Merge.DeployState
}

func (s *DeployWatchTestSuite) requireStuck(line string, state domain.DeployState) {
	s.Equal([]string{line}, s.texts())
	s.Equal(state, s.deployState())
	s.Empty(s.stateChanges(), "a stuck deploy keeps Linear as it is")
	s.Equal([]SlackReaction{
		{Method: "remove", Name: slackpost.ReactionWorking, Channel: statusChannelID, TS: statusThreadTS},
		{Method: "add", Name: slackpost.ReactionStuck, Channel: statusChannelID, TS: statusThreadTS},
	}, s.slack.Reactions())
}

func (s *DeployWatchTestSuite) TestJobAndCheckGoLiveOnlyAfterTheCheckPasses() {
	marker := filepath.Join(s.T().TempDir(), "live-abc1234")
	s.registry = deployRegistry(config.RepoDeploy{Job: deployJob, Check: "test -f " + filepath.Dir(marker) + "/live-{short_sha}", Timeout: time.Hour})
	s.deployJobIs("completed", "success")

	s.tick()

	s.Empty(s.slack.Posts(), "the job passed, but the check does not")
	s.Equal(domain.DeployPending, s.deployState())

	s.Require().NoError(os.WriteFile(marker, nil, 0o644))
	s.tick()
	s.tick()

	s.Equal([]string{liveLine}, s.texts(), "the line is posted once")
	s.Equal(domain.DeployDeployed, s.deployState())
	s.Equal([]string{"st-Done"}, s.stateChanges())
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Equal(domain.Phase("done"), run.Phase)
	s.Equal([]SlackReaction{
		{Method: "remove", Name: slackpost.ReactionWorking, Channel: statusChannelID, TS: statusThreadTS},
		{Method: "add", Name: slackpost.ReactionDone, Channel: statusChannelID, TS: statusThreadTS},
	}, s.slack.Reactions())
}

func (s *DeployWatchTestSuite) TestCheckIsNotRunBeforeTheJobPasses() {
	ran := filepath.Join(s.T().TempDir(), "ran")
	s.registry = deployRegistry(config.RepoDeploy{Job: deployJob, Check: "touch " + ran, Timeout: time.Hour})
	s.deployJobIs("in_progress", "")

	s.tick()

	s.NoFileExists(ran)
	s.Equal(domain.DeployPending, s.deployState())
}

func (s *DeployWatchTestSuite) TestJobAloneGoesLiveWhenItPasses() {
	s.deployJobIs("completed", "success")

	s.tick()

	s.Equal([]string{liveLine}, s.texts())
	s.Equal(domain.DeployDeployed, s.deployState())
}

func (s *DeployWatchTestSuite) TestCheckThatNeverPassesIsStuckAfterTheTimeout() {
	s.registry = deployRegistry(config.RepoDeploy{Check: "false", Timeout: 30 * time.Minute})
	s.Clock.Advance(29 * time.Minute)
	s.tick()
	s.Empty(s.slack.Posts())

	s.Clock.Advance(2 * time.Minute)
	s.tick()

	s.requireStuck("⛔ ACM-9 is stuck: deploy of acme-api not live after 30 minutes.", domain.DeployFaulted)
	s.Equal([]string{"autopilot fault ACM-9: deploy of acme-api not live after 30 minutes; proof: the deploy check did not pass" + faultFix, "\r"}, s.typed())
}

func (s *DeployWatchTestSuite) TestSkippedJobIsRerunOnceQuietly() {
	s.ciRunIs("failure", "completed", "skipped")

	s.tick()

	s.Empty(s.slack.Posts())
	s.Empty(s.slack.Reactions())
	s.Empty(s.typed())
	s.Equal(domain.DeployPending, s.deployState())
	s.Equal([][]string{{"run", "rerun", "11", "--failed", "--repo", "acme/acme-api"}}, s.calls(s.ghDir, "run", "rerun"))
}

func (s *DeployWatchTestSuite) TestTimeoutStartsAgainAtTheRerun() {
	s.Clock.Advance(50 * time.Minute)
	s.ciRunIs("failure", "completed", "skipped")
	s.tick()

	s.deployJobIs("in_progress", "")
	s.Clock.Advance(30 * time.Minute)
	s.tick()

	s.Empty(s.slack.Posts(), "80 minutes after the merge, but 30 after the rerun")
	s.Equal(domain.DeployPending, s.deployState())
}

func (s *DeployWatchTestSuite) TestSkippedAgainAfterTheRerunIsStuckAndHandedToTheControlRoom() {
	s.ciRunIs("failure", "completed", "skipped")
	s.tick()

	s.tick()

	s.requireStuck("⛔ ACM-9 is stuck: deploy of acme-api failed: job was skipped.", domain.DeployFaulted)
	s.Len(s.calls(s.ghDir, "run", "rerun"), 1, "the deploy is re-run once")
	s.Equal([]string{
		"autopilot fault ACM-9: deploy of acme-api failed: " + jobURL + "; proof: " + runURL + " (failed jobs: test)" + faultFix,
		"\r",
	}, s.typed())
	s.Equal("ctl-1", s.calls(s.agtermDir, "session", "type")[0][5], "the fault goes to the control room session")
}

func (s *DeployWatchTestSuite) TestJobFailureAfterTheRerunIsStuck() {
	s.deployJobIs("completed", "failure")
	s.tick()
	s.Empty(s.slack.Posts())

	s.tick()

	s.requireStuck("⛔ ACM-9 is stuck: deploy of acme-api failed: "+jobURL+".", domain.DeployFaulted)
}

func (s *DeployWatchTestSuite) TestFaultedDeployStaysQuietUntilTheFixIsLive() {
	s.fault()
	s.writeGH("head.json", `{"object":{"sha":"`+mergeSHA+`"}}`)
	s.Clock.Advance(2 * time.Hour)

	s.tick()

	s.Len(s.slack.Posts(), 1, "only the stuck line")
	s.Len(s.typed(), 2, "the fault is handed once")
	s.Equal(domain.DeployFaulted, s.deployState())
}

func (s *DeployWatchTestSuite) TestNewerCommitThatContainsTheMergeFinishesAFaultedRun() {
	s.registry = deployRegistry(config.RepoDeploy{Job: deployJob, Check: "test {sha} = " + fixSHA, Timeout: time.Hour})
	s.fault()
	s.writeGH("head.json", `{"object":{"sha":"`+fixSHA+`"}}`)
	s.writeGH("compare.json", `{"status":"ahead"}`)
	s.writeGH("runs-"+fixSHA+".json", `[{"databaseId":12}]`)
	s.writeGH("run-12.json", `{"url":"https://x/12","jobs":[{"name":"`+deployJob+`","status":"completed","conclusion":"success","url":"https://x/12/1"}]}`)

	s.tick()

	s.Equal([]string{"⛔ ACM-9 is stuck: deploy of acme-api failed: job was skipped.", liveLine}, s.texts())
	s.Equal(domain.DeployDeployed, s.deployState())
	s.Equal([]string{"st-Done"}, s.stateChanges())
	s.Equal([][]string{{"api", "repos/acme/acme-api/compare/" + mergeSHA + "..." + fixSHA}}, s.calls(s.ghDir, "api", "repos/acme/acme-api/compare/"+mergeSHA+"..."+fixSHA))
}

func (s *DeployWatchTestSuite) TestJobNotDoneStaysPendingQuietly() {
	s.tick()
	s.deployJobIs("in_progress", "")
	s.Clock.Advance(59 * time.Minute)
	s.tick()

	s.Empty(s.slack.Posts())
	s.Empty(s.slack.Reactions())
	s.Equal(domain.DeployPending, s.deployState())
}

func (s *DeployWatchTestSuite) TestCheckPlaceholdersAreReplaced() {
	out := filepath.Join(s.T().TempDir(), "out")
	s.registry = deployRegistry(config.RepoDeploy{Check: "echo {sha} {short_sha} > " + out, Timeout: time.Hour})

	s.tick()

	got, err := os.ReadFile(out)
	s.Require().NoError(err)
	s.Equal(mergeSHA+" abc1234\n", string(got))
}

func (s *DeployWatchTestSuite) TestRunWithTwoReposIsLiveWhenBothAre() {
	s.registry.Projects[0].Repos = []config.Repo{
		{Name: prRepo, DefaultBranch: "main", Deploy: config.RepoDeploy{Check: "true", Timeout: time.Hour}},
		{Name: "acme-web", DefaultBranch: "main", Deploy: config.RepoDeploy{Check: "true", Timeout: time.Hour}},
	}
	s.merge("acme-web", "https://github.com/acme/acme-web/pull/8")

	s.tick()

	s.Equal([]string{"✅ ACM-9 is live (acme-api abc1234, acme-web abc1234). Ready to test."}, s.texts())
}
