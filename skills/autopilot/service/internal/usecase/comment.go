package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/linear"
)

// ICommentLinear is the part of the Linear client that a comment uses.
type ICommentLinear interface {
	IssueByKey(ctx context.Context, key string) (linear.Issue, error)
	Comment(ctx context.Context, issueID, body string) error
}

// ICommentRuns is the part of the run store that a comment uses.
type ICommentRuns interface {
	FindByKey(ctx context.Context, key domain.IssueKey) (domain.Run, error)
}

type CommenterData struct {
	Linear ICommentLinear
	Runs   ICommentRuns
}

// Commenter posts a markdown body as a Linear comment on the issue of a live run.
type Commenter struct {
	d CommenterData
}

func NewCommenter(d CommenterData) *Commenter {
	return &Commenter{d: d}
}

// Comment fails for an unknown or ended run and for an empty body.
func (c *Commenter) Comment(ctx context.Context, key domain.IssueKey, body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("the comment for %s is empty", key)
	}
	run, err := c.d.Runs.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("cannot find run %s: %w", key, err)
	}
	if !run.EndedAt.IsZero() {
		return fmt.Errorf("run %s has ended", key)
	}
	issue, err := c.d.Linear.IssueByKey(ctx, string(key))
	if err != nil {
		return fmt.Errorf("cannot read issue %s: %w", key, err)
	}
	if err := c.d.Linear.Comment(ctx, issue.ID, body); err != nil {
		return fmt.Errorf("cannot comment on %s: %w", key, err)
	}
	return nil
}
