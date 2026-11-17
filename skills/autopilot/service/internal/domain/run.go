package domain

import "time"

// RunsFolder is the folder of the workspace folder that holds the worktrees of the runs.
const RunsFolder = ".autopilot-runs"

type IssueKey string

// Phase names come from the status map of the workspace registry, so the store holds them as text.
type Phase string

type Loop string

const (
	LoopGate   Loop = "gate"
	LoopReview Loop = "review"
	LoopPR     Loop = "pr"
	// LoopResume counts the owner resumes of a blocked run; it is never reset.
	LoopResume Loop = "resume"
)

type Run struct {
	IssueKey      IssueKey
	Product       string
	ChannelID     string
	RequestTS     string
	ThreadTS      string
	CardTS        string
	SessionID     string
	WorktreeBase  string
	Phase         Phase
	PreviousPhase Phase
	LastPostAt    time.Time
	EndedAt       time.Time
	CreatedAt     time.Time
}

// RedBase binds a repo's red proof to two SHAs: the commit before QA's tests and the commit that adds them.
// TestIDs are the JUnit ids that failed by assertion on the base; the gate needs them to pass on the head.
// QAFiles and CriteriaTests are frozen at the proof, so a later edit of the run's files cannot move what the
// gate compares. ReProofs lists every re-proof with the QA test files it changed, for the PR.
type RedBase struct {
	Repo          string
	BaseSHA       string
	TestsSHA      string
	TestIDs       []string
	QAFiles       []string
	CriteriaTests []CriterionTest
	ReProofs      []string
}

// CriterionTest is one test of the criteria table as a red proof froze it.
type CriterionTest struct {
	ID        string `json:"id"`
	JUnitName string `json:"junit_name"`
	File      string `json:"file"`
}

// PRWatch is the poll state of one PR. State is open, merged or closed. LastCommentID is the time (RFC3339) of the
// newest feedback already pointed at the run session; only feedback newer than it is new.
type PRWatch struct {
	State         string
	LastCommentID string
	CheckedAt     time.Time
}

// DeployState is where the deploy of a merged repo stands. It is empty for a repo that a person deploys by hand and
// for a merge from before the deploy watch; it is pending while the watch waits for the merge to go live, deployed
// when it is live, and faulted when the deploy is stuck and the fault went to the control room: the watch then waits
// for a fix on the default branch. A stuck deploy from before the fault hand-off is failed, skipped or unseen.
type DeployState string

const (
	DeployPending  DeployState = "pending"
	DeployDeployed DeployState = "deployed"
	DeployFailed   DeployState = "failed"
	DeploySkipped  DeployState = "skipped"
	DeployFaulted  DeployState = "faulted"
)

// Watched is true while the deploy watch still waits for the merge to go live.
func (s DeployState) Watched() bool {
	return s == DeployPending || s == DeployFaulted
}

// Merge is what the PR watch stores when the PR of a repo is merged: the PR's branch and merge commit. RetriedAt is
// when the deploy watch re-ran the failed jobs of the merge's CI run; it is zero before that.
type Merge struct {
	Branch      string
	SHA         string
	DeployState DeployState
	At          time.Time
	RetriedAt   time.Time
}

// WaitsSince is when the deploy timeout starts: the rerun of the failed jobs, or else the merge.
func (m Merge) WaitsSince() time.Time {
	if m.RetriedAt.IsZero() {
		return m.At
	}
	return m.RetriedAt
}

// Failing is true when the deploy failed: it is faulted, or pending again after the rerun of its failed jobs.
func (m Merge) Failing() bool {
	return m.DeployState == DeployFaulted || (m.DeployState == DeployPending && !m.RetriedAt.IsZero())
}

// shortSHALength is the length of a short commit SHA, as git and GitHub show it.
const shortSHALength = 7

// ShortSHA is the merge commit in its short form.
func (m Merge) ShortSHA() string {
	return ShortSHA(m.SHA)
}

// ShortSHA is a commit SHA in its short form.
func ShortSHA(sha string) string {
	return sha[:min(len(sha), shortSHALength)]
}

// PendingDeploy is one merged repo whose deploy the deploy watch still waits for.
type PendingDeploy struct {
	Run  Run
	Repo RunRepo
}

type RunRepo struct {
	Repo        string
	RedBaseSHA  string
	RedTestsSHA string
	RedTestIDs  []string
	// RedQAFiles, RedCriteriaTests and RedReProofs are the rest of the red proof, as RedBase holds them.
	RedQAFiles       []string
	RedCriteriaTests []CriterionTest
	RedReProofs      []string
	PRURL            string
	PRCard           PRCard
	Watch            PRWatch
	Merge            Merge
}

// PRCard is the posted Slack card of a repo's PR: its message ts and its text, so the card can be redrawn.
type PRCard struct {
	TS   string
	Text string
}
