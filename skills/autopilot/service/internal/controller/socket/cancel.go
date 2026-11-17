package socket

import (
	"context"
	"fmt"

	"github.com/sadensmol/autopilot/internal/domain"
)

// ICancel cancels a run on the owner's verb.
type ICancel interface {
	CancelRun(ctx context.Context, key domain.IssueKey) error
}

// IResume takes a blocked run out of the block on the owner's verb.
type IResume interface {
	Resume(ctx context.Context, key domain.IssueKey) error
}

// NewCancelHandler cancels a run; the worktree stays.
func NewCancelHandler(c ICancel) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		if err := c.CancelRun(ctx, domain.IssueKey(req.Key)); err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot cancel: %v", err)}
		}
		return Response{}
	}
}

// NewResumeHandler resumes a blocked run.
func NewResumeHandler(r IResume) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		if err := r.Resume(ctx, domain.IssueKey(req.Key)); err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot resume: %v", err)}
		}
		return Response{}
	}
}
