package domain

import "time"

type CheckStatus string

const (
	CheckPass          CheckStatus = "pass"
	CheckFail          CheckStatus = "fail"
	CheckNotApplicable CheckStatus = "n/a"
	CheckCouldNotRun   CheckStatus = "could-not-run"
)

type CheckResult struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

// Verdict is the gate's result for one repo at one SHA. Manual lists the criteria a human must check at the PR;
// RedReProofs lists every red re-proof with the QA test files it changed, for the owner to read at the PR.
type Verdict struct {
	IssueKey    IssueKey
	Repo        string
	SHA         string
	Checks      []CheckResult
	Manual      []string
	RedReProofs []string
	CreatedAt   time.Time
}

// Passed is true when the verdict has checks and every one passed or had nothing to measure.
func (v Verdict) Passed() bool {
	for _, c := range v.Checks {
		if c.Status != CheckPass && c.Status != CheckNotApplicable {
			return false
		}
	}
	return len(v.Checks) > 0
}
