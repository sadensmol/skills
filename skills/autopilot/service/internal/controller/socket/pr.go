package socket

import (
	"context"
	"fmt"

	"github.com/sadensmol/autopilot/internal/domain"
)

// IPROpen records the PR of a run and posts the manual checklists.
type IPROpen interface {
	Open(ctx context.Context, key domain.IssueKey, prURL string) error
	Manual(ctx context.Context, key domain.IssueKey, checklist string) error
}

// IFixMerge merges a fix PR of the default branch once its checks are green.
type IFixMerge interface {
	Merge(ctx context.Context, key domain.IssueKey, prURL string) error
}

// NewPRHandler stores the PR of a run for watching and posts it to the thread.
func NewPRHandler(p IPROpen) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		if err := p.Open(ctx, domain.IssueKey(req.Key), req.URL); err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot register the PR: %v", err)}
		}
		return Response{}
	}
}

// NewManualHandler posts a manual checklist to the thread with a mention of the owner.
func NewManualHandler(p IPROpen) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		if err := p.Manual(ctx, domain.IssueKey(req.Key), req.Body); err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot post the checklist: %v", err)}
		}
		return Response{}
	}
}

// NewFixHandler waits for the checks of a fix PR and merges it; it answers when the PR is merged or cannot be.
func NewFixHandler(f IFixMerge) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		if err := f.Merge(ctx, domain.IssueKey(req.Key), req.URL); err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot merge the fix: %v", err)}
		}
		return Response{Output: fmt.Sprintf("merged %s", req.URL)}
	}
}
