package socket

import (
	"context"
	"fmt"

	"github.com/sadensmol/autopilot/internal/domain"
)

// IStatus moves the phase of a run and posts the notes of its workers.
type IStatus interface {
	Move(ctx context.Context, key domain.IssueKey, phase string) error
	Post(ctx context.Context, key domain.IssueKey, note string) error
}

// NewStatusHandler sets the phase of a run.
func NewStatusHandler(status IStatus) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		if err := status.Move(ctx, domain.IssueKey(req.Key), req.Phase); err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot set phase: %v", err)}
		}
		return Response{}
	}
}

// NewPostHandler posts a plain line to the thread of a run.
func NewPostHandler(status IStatus) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		if err := status.Post(ctx, domain.IssueKey(req.Key), req.Text); err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot post: %v", err)}
		}
		return Response{}
	}
}
