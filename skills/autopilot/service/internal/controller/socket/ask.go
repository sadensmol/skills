package socket

import (
	"context"
	"errors"
	"fmt"

	"github.com/sadensmol/autopilot/internal/domain"
)

// IAsker opens asks and waits for their answers.
type IAsker interface {
	Open(ctx context.Context, key domain.IssueKey, question string, options []domain.AskOption) (domain.Ask, error)
	Wait(ctx context.Context, id domain.AskID) (domain.Ask, error)
}

// NewAskHandler opens an ask, then waits like ask-wait does.
func NewAskHandler(asker IAsker) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		options := make([]domain.AskOption, 0, len(req.Options))
		for _, o := range req.Options {
			options = append(options, domain.AskOption{ID: o.ID, Label: o.Label})
		}
		ask, err := asker.Open(ctx, domain.IssueKey(req.Key), req.Text, options)
		if err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot open the ask: %v", err)}
		}
		return waitResponse(ctx, asker, ask.ID)
	}
}

// NewAskWaitHandler waits for an ask that is already open.
func NewAskWaitHandler(asker IAsker) HandlerFunc {
	return func(ctx context.Context, req Request) Response {
		return waitResponse(ctx, asker, domain.AskID(req.ID))
	}
}

// waitResponse answers with the answer text, or with the ask id and code 75 when the ask is still open.
func waitResponse(ctx context.Context, asker IAsker, id domain.AskID) Response {
	ask, err := asker.Wait(ctx, id)
	if errors.Is(err, domain.ErrAskPending) {
		return Response{Code: CodePending, Output: fmt.Sprintf("%d", id)}
	}
	if err != nil {
		return Response{Code: CodeError, Error: fmt.Sprintf("ask %d: %v", id, err)}
	}
	return Response{Output: ask.Answer}
}
