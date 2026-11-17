package socket

import (
	"context"
	"fmt"
	"slices"
)

type IHandler interface {
	Handle(ctx context.Context, req Request) Response
}

type HandlerFunc func(ctx context.Context, req Request) Response

func (f HandlerFunc) Handle(ctx context.Context, req Request) Response {
	return f(ctx, req)
}

// Mux routes a request to the handler of its verb.
type Mux struct {
	handlers map[string]IHandler
}

func NewMux() *Mux {
	return &Mux{handlers: map[string]IHandler{}}
}

// Register sets the handler of a verb; later steps register theirs in serve.
func (m *Mux) Register(verb string, h HandlerFunc) {
	m.handlers[verb] = h
}

func (m *Mux) Handle(ctx context.Context, req Request) Response {
	if h, ok := m.handlers[req.Verb]; ok {
		return h.Handle(ctx, req)
	}
	if slices.Contains(Verbs, req.Verb) {
		return Response{Code: CodeError, Error: fmt.Sprintf("%s: not implemented yet", req.Verb)}
	}
	return Response{Code: CodeError, Error: fmt.Sprintf("unknown verb %q", req.Verb)}
}
