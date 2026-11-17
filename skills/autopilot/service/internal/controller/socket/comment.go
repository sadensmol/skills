package socket

import (
	"context"
	"fmt"

	"github.com/sadensmol/autopilot/internal/domain"
)

// IComment posts a comment on the issue of a run.
type IComment interface {
	Comment(ctx context.Context, key domain.IssueKey, body string) error
}

// NewCommentHandler posts the body of the request as a Linear comment on the run's issue.
func NewCommentHandler(c IComment) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		if err := c.Comment(ctx, domain.IssueKey(req.Key), req.Body); err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot comment: %v", err)}
		}
		return Response{}
	}
}
