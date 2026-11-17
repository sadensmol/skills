package socket

import (
	"context"
	"fmt"
	"strings"

	"github.com/sadensmol/autopilot/internal/domain"
)

// IGate proves QA's tests red and runs the hard gate for one repo of a run.
type IGate interface {
	Red(ctx context.Context, key domain.IssueKey, repo, sha string) (domain.RedBase, error)
	Run(ctx context.Context, key domain.IssueKey, repo, sha string) (domain.Verdict, error)
}

// NewRedHandler answers with the test ids proven red, or with the reasons the proof failed.
func NewRedHandler(g IGate) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		red, err := g.Red(ctx, domain.IssueKey(req.Key), req.Repo, req.SHA)
		if err != nil {
			return Response{Code: CodeError, Error: err.Error()}
		}
		return Response{Output: fmt.Sprintf("red proven at %s on base %s for %d tests:\n  %s",
			red.TestsSHA, red.BaseSHA, len(red.TestIDs), strings.Join(red.TestIDs, "\n  "))}
	}
}

// NewGateHandler answers with the verdict, one line per check; any verdict but a pass answers with an error code.
func NewGateHandler(g IGate) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		verdict, err := g.Run(ctx, domain.IssueKey(req.Key), req.Repo, req.SHA)
		if err != nil {
			return Response{Code: CodeError, Error: err.Error()}
		}
		result := "FAIL"
		if verdict.Passed() {
			result = "PASS"
		}
		var b strings.Builder
		fmt.Fprintf(&b, "gate %s for %s at %s\n", result, verdict.Repo, verdict.SHA)
		for _, c := range verdict.Checks {
			fmt.Fprintf(&b, "%s\t%s\t%s\n", c.Name, c.Status, c.Detail)
		}
		for _, m := range verdict.Manual {
			fmt.Fprintf(&b, "manual\t%s\n", m)
		}
		for _, p := range verdict.RedReProofs {
			fmt.Fprintf(&b, "re-proof\t%s\n", p)
		}
		if !verdict.Passed() {
			return Response{Code: CodeError, Output: b.String(), Error: fmt.Sprintf("the gate did not pass at %s", verdict.SHA)}
		}
		return Response{Output: b.String()}
	}
}
