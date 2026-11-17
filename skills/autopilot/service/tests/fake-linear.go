package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// LinearRequest is one GraphQL call the fake received.
type LinearRequest struct {
	Operation     string
	Variables     map[string]any
	Authorization string
}

// LinearReply is what the fake sends back: data, GraphQL errors, or a bare HTTP status.
type LinearReply struct {
	Data   any
	Errors []string
	Status int
	Body   string
}

// FakeLinear is an httptest GraphQL server that routes by operation name.
type FakeLinear struct {
	Server *httptest.Server

	mu       sync.Mutex
	handlers map[string]func(vars map[string]any) LinearReply
	requests []LinearRequest
}

func NewFakeLinear(t *testing.T) *FakeLinear {
	f := &FakeLinear{handlers: map[string]func(map[string]any) LinearReply{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Server.Close)
	return f
}

func (f *FakeLinear) URL() string {
	return f.Server.URL
}

// On sets the reply for one operation name.
func (f *FakeLinear) On(operation string, fn func(vars map[string]any) LinearReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[operation] = fn
}

// Requests returns the calls received so far.
func (f *FakeLinear) Requests() []LinearRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]LinearRequest(nil), f.requests...)
}

func (f *FakeLinear) serve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, LinearRequest{Operation: body.OperationName, Variables: body.Variables, Authorization: r.Header.Get("Authorization")})
	fn := f.handlers[body.OperationName]
	f.mu.Unlock()
	if fn == nil {
		http.Error(w, "no handler for "+body.OperationName, http.StatusNotImplemented)
		return
	}
	reply := fn(body.Variables)
	if reply.Status != 0 {
		w.WriteHeader(reply.Status)
		if _, err := w.Write([]byte(reply.Body)); err != nil {
			return
		}
		return
	}
	out := map[string]any{}
	if reply.Data != nil {
		out["data"] = reply.Data
	}
	if len(reply.Errors) > 0 {
		errs := make([]map[string]string, 0, len(reply.Errors))
		for _, m := range reply.Errors {
			errs = append(errs, map[string]string{"message": m})
		}
		out["errors"] = errs
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		http.Error(w, "encode", http.StatusInternalServerError)
	}
}
