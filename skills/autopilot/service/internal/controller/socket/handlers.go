package socket

import (
	"context"
	"fmt"
	"strings"

	"github.com/sadensmol/autopilot/internal/domain"
)

func PingHandler(context.Context, Request) Response {
	return Response{Output: "pong"}
}

type IRunLister interface {
	ListActive(ctx context.Context) ([]domain.Run, error)
}

// NewRunsHandler lists the active runs, one line each: key, phase, product.
func NewRunsHandler(runs IRunLister) HandlerFunc {
	return func(ctx context.Context, _ Request) Response {
		active, err := runs.ListActive(ctx)
		if err != nil {
			return Response{Code: CodeError, Error: fmt.Sprintf("cannot list runs: %v", err)}
		}
		var b strings.Builder
		for _, run := range active {
			fmt.Fprintf(&b, "%s\t%s\t%s\n", run.IssueKey, run.Phase, run.Product)
		}
		return Response{Output: b.String()}
	}
}
