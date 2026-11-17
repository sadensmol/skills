package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/controller/cli"
	"github.com/sadensmol/autopilot/internal/controller/socket"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/repository"
)

type CLISuite struct {
	Suite
	workspace string
	sockPath  string
	cancel    context.CancelFunc
	done      chan error
}

func TestCLISuite(t *testing.T) {
	suite.Run(t, new(CLISuite))
}

func (s *CLISuite) SetupTest() {
	s.Suite.SetupTest()
	// the unix socket path limit is 103 bytes on macOS, so the base is short
	base, err := os.MkdirTemp("/tmp", "ap")
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(os.RemoveAll(base)) })
	s.workspace = base
	dir := filepath.Join(base, ".workspace", "autopilot")
	s.Require().NoError(os.MkdirAll(dir, 0o700))
	s.sockPath = filepath.Join(dir, "autopilot.sock")
}

func (s *CLISuite) TearDownTest() {
	s.stop()
	s.Suite.TearDownTest()
}

func (s *CLISuite) start(h socket.IHandler, opts ...socket.ServerOption) {
	server, err := socket.NewServer(s.sockPath, h, opts...)
	s.Require().NoError(err)
	ctx, cancel := context.WithCancel(s.Ctx)
	s.cancel = cancel
	s.done = make(chan error, 1)
	go func() { s.done <- server.Serve(ctx) }()
}

func (s *CLISuite) stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	s.Require().NoError(<-s.done)
	s.cancel = nil
}

func (s *CLISuite) store() *socket.Mux {
	mux := socket.NewMux()
	mux.Register(socket.VerbPing, socket.PingHandler)
	mux.Register(socket.VerbRuns, socket.NewRunsHandler(repository.NewRun(s.DB)))
	return mux
}

func (s *CLISuite) cliRun(env map[string]string, cwd string, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := cli.Run(args, cli.Env{
		Getenv: func(k string) string { return env[k] },
		Getwd:  func() (string, error) { return cwd, nil },
	}, &out, &errOut)
	return code, out.String(), errOut.String()
}

func (s *CLISuite) viaEnv(args ...string) (int, string, string) {
	return s.cliRun(map[string]string{"AUTOPILOT_SOCKET": s.sockPath}, "/", args...)
}

func (s *CLISuite) TestSocketIsMode600() {
	s.start(s.store())
	info, err := os.Stat(s.sockPath)
	s.Require().NoError(err)
	s.Equal(os.FileMode(0o600), info.Mode().Perm())
	s.NotZero(info.Mode() & os.ModeSocket)
	entries, err := os.ReadDir(filepath.Dir(s.sockPath))
	s.Require().NoError(err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	s.ElementsMatch([]string{"autopilot.sock", "autopilot.sock.lock"}, names, "no temp folder stays behind")
}

func (s *CLISuite) TestPingRoundTrip() {
	s.start(s.store())
	code, out, errOut := s.viaEnv("ping")
	s.Equal(0, code, errOut)
	s.Equal("pong", strings.TrimSpace(out))
}

func (s *CLISuite) TestRunsRoundTrip() {
	s.start(s.store())
	_, err := repository.NewRun(s.DB).Create(s.Ctx, domain.Run{IssueKey: "ACM-7", Product: "widgets", ChannelID: "C1", RequestTS: "1.1", Phase: "Todo"}, s.Clock.Now())
	s.Require().NoError(err)
	code, out, errOut := s.viaEnv("runs")
	s.Equal(0, code, errOut)
	s.Contains(out, "ACM-7")
	s.Contains(out, "Todo")
}

func (s *CLISuite) TestNoServiceExits3() {
	code, _, errOut := s.viaEnv("ping")
	s.Equal(3, code)
	s.Contains(errOut, "service not running")
}

func (s *CLISuite) TestStaleSocketFileExits3() {
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: s.sockPath, Net: "unix"})
	s.Require().NoError(err)
	l.SetUnlinkOnClose(false)
	s.Require().NoError(l.Close())
	code, _, errOut := s.viaEnv("ping")
	s.Equal(3, code)
	s.Contains(errOut, "service not running")
}

func (s *CLISuite) TestWalkUpDiscovery() {
	s.start(s.store())
	deep := filepath.Join(s.workspace, ".autopilot-runs", "ACM-7", "api", "internal")
	s.Require().NoError(os.MkdirAll(deep, 0o755))
	code, out, errOut := s.cliRun(map[string]string{}, deep, "ping")
	s.Equal(0, code, errOut)
	s.Equal("pong", strings.TrimSpace(out))
}

func (s *CLISuite) TestNoWorkspaceFolderExits1() {
	other, err := os.MkdirTemp("/tmp", "ap")
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(os.RemoveAll(other)) })
	code, _, errOut := s.cliRun(map[string]string{}, other, "ping")
	s.Equal(1, code)
	s.Contains(errOut, "AUTOPILOT_SOCKET")
	s.Contains(errOut, ".workspace/autopilot")
}

func (s *CLISuite) TestEnvWinsOverWalkUp() {
	s.start(s.store())
	code, _, errOut := s.cliRun(map[string]string{"AUTOPILOT_SOCKET": filepath.Join(s.workspace, "none.sock")}, filepath.Join(s.workspace, ".workspace", "autopilot"), "ping")
	s.Equal(3, code)
	s.Contains(errOut, "service not running")
}

func (s *CLISuite) TestSocketPathOver103BytesRefused() {
	long := filepath.Join(s.workspace, strings.Repeat("d", 100), "autopilot.sock")
	s.Require().NoError(os.MkdirAll(filepath.Dir(long), 0o700))
	_, err := socket.NewServer(long, s.store())
	s.Require().Error(err)
	s.Contains(err.Error(), long)
	s.Contains(err.Error(), "103")
}

func (s *CLISuite) TestStaleSocketDoesNotBlockStart() {
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: s.sockPath, Net: "unix"})
	s.Require().NoError(err)
	l.SetUnlinkOnClose(false)
	s.Require().NoError(l.Close())
	_, statErr := os.Stat(s.sockPath)
	s.Require().NoError(statErr, "the crashed service left its file")
	s.start(s.store())
	code, _, errOut := s.viaEnv("ping")
	s.Equal(0, code, errOut)
}

func (s *CLISuite) TestLiveSocketBlocksSecondStart() {
	s.start(s.store())
	_, err := socket.NewServer(s.sockPath, s.store())
	s.Require().Error(err)
	s.Contains(err.Error(), "already running")
}

func (s *CLISuite) TestRegularFileAtPathRefused() {
	s.Require().NoError(os.WriteFile(s.sockPath, []byte("x"), 0o600))
	_, err := socket.NewServer(s.sockPath, s.store())
	s.Require().Error(err)
	s.Contains(err.Error(), "not a socket")
}

func (s *CLISuite) TestSocketRemovedOnShutdown() {
	s.start(s.store())
	s.stop()
	_, err := os.Stat(s.sockPath)
	s.True(os.IsNotExist(err))
}

func (s *CLISuite) TestUnimplementedVerbExits1() {
	s.start(s.store())
	code, _, errOut := s.viaEnv("cancel", "ACM-7")
	s.Equal(1, code)
	s.Contains(errOut, "not implemented yet")
}

func (s *CLISuite) TestPendingExits75() {
	mux := socket.NewMux()
	mux.Register(socket.VerbAskWait, func(context.Context, socket.Request) socket.Response {
		return socket.Response{Code: socket.CodePending, Output: "ask 4 is still open"}
	})
	s.start(mux)
	code, out, _ := s.viaEnv("ask", "--wait", "4")
	s.Equal(75, code)
	s.Contains(out, "still open")
}

func (s *CLISuite) TestHandlerErrorExits1() {
	mux := socket.NewMux()
	mux.Register(socket.VerbPost, func(context.Context, socket.Request) socket.Response {
		return socket.Response{Code: socket.CodeError, Error: "run ACM-9: not found"}
	})
	s.start(mux)
	code, _, errOut := s.viaEnv("post", "ACM-9", "hello")
	s.Equal(1, code)
	s.Contains(errOut, "run ACM-9: not found")
}

type recorder struct {
	mu   sync.Mutex
	reqs []socket.Request
}

func (r *recorder) Handle(_ context.Context, req socket.Request) socket.Response {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, req)
	return socket.Response{Output: "ok"}
}

func (r *recorder) last() socket.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reqs[len(r.reqs)-1]
}

func (s *CLISuite) TestEveryVerbRoundTripsItsArguments() {
	rec := new(recorder)
	mux := socket.NewMux()
	for _, v := range socket.Verbs {
		mux.Register(v, rec.Handle)
	}
	s.start(mux)
	file := filepath.Join(s.workspace, "note.md")
	s.Require().NoError(os.WriteFile(file, []byte("# body\n"), 0o600))

	tests := []struct {
		name string
		args []string
		want socket.Request
	}{
		{"ping", []string{"ping"}, socket.Request{Verb: socket.VerbPing}},
		{"runs", []string{"runs"}, socket.Request{Verb: socket.VerbRuns}},
		{"post", []string{"post", "ACM-7", "a note"}, socket.Request{Verb: socket.VerbPost, Key: "ACM-7", Text: "a note"}},
		{"status", []string{"status", "ACM-7", "In QA"}, socket.Request{Verb: socket.VerbStatus, Key: "ACM-7", Phase: "In QA"}},
		{"ask", []string{"ask", "ACM-7", "which?"}, socket.Request{Verb: socket.VerbAsk, Key: "ACM-7", Text: "which?"}},
		{"ask options", []string{"ask", "ACM-7", "which?", "--option", "a=Alpha", "--option=b=Beta=2"}, socket.Request{
			Verb: socket.VerbAsk, Key: "ACM-7", Text: "which?",
			Options: []socket.Option{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta=2"}},
		}},
		{"ask wait", []string{"ask", "--wait", "12"}, socket.Request{Verb: socket.VerbAskWait, ID: 12}},
		{"red", []string{"red", "ACM-7", "--repo", "api", "--sha", "abc123"}, socket.Request{Verb: socket.VerbRed, Key: "ACM-7", Repo: "api", SHA: "abc123"}},
		{"gate", []string{"gate", "ACM-7", "--sha", "abc123", "--repo=api"}, socket.Request{Verb: socket.VerbGate, Key: "ACM-7", Repo: "api", SHA: "abc123"}},
		{"comment", []string{"comment", "ACM-7", "--file", file}, socket.Request{Verb: socket.VerbComment, Key: "ACM-7", Body: "# body\n"}},
		{"pr", []string{"pr", "ACM-7", "https://example.test/pr/1"}, socket.Request{Verb: socket.VerbPR, Key: "ACM-7", URL: "https://example.test/pr/1"}},
		{"manual", []string{"manual", "ACM-7", "--file", file}, socket.Request{Verb: socket.VerbManual, Key: "ACM-7", Body: "# body\n"}},
		{"cancel", []string{"cancel", "ACM-7"}, socket.Request{Verb: socket.VerbCancel, Key: "ACM-7"}},
		{"resume", []string{"resume", "ACM-7"}, socket.Request{Verb: socket.VerbResume, Key: "ACM-7"}},
		{"merge-fix", []string{"merge-fix", "ACM-7", "https://example.test/pr/2"}, socket.Request{Verb: socket.VerbFix, Key: "ACM-7", URL: "https://example.test/pr/2"}},
	}
	for _, tt := range tests {
		code, _, errOut := s.viaEnv(tt.args...)
		s.Equal(0, code, "%s: %s", tt.name, errOut)
		s.Equal(tt.want, rec.last(), tt.name)
	}
}

func (s *CLISuite) TestResumeFromARunWorktreeIsRefusedWithoutCallingService() {
	rec := new(recorder)
	mux := socket.NewMux()
	for _, v := range socket.Verbs {
		mux.Register(v, rec.Handle)
	}
	s.start(mux)
	env := map[string]string{"AUTOPILOT_SOCKET": s.sockPath}

	code, _, errOut := s.cliRun(env, "/ws/.autopilot-runs/ACM-7/acme-api", "resume", "ACM-7")

	s.Equal(1, code)
	s.Contains(errOut, "owner")
	s.Empty(rec.reqs)
	code, _, errOut = s.cliRun(env, "/ws", "resume", "ACM-7")
	s.Equal(0, code, errOut)
	code, _, errOut = s.cliRun(env, "/ws/.autopilot-runs/ACM-7/acme-api", "post", "ACM-7", "a note")
	s.Equal(0, code, "a run's own verbs still work in a run worktree: %s", errOut)
}

func (s *CLISuite) TestCancelFromARunFolderIsRefusedWithoutCallingService() {
	rec := new(recorder)
	mux := socket.NewMux()
	for _, v := range socket.Verbs {
		mux.Register(v, rec.Handle)
	}
	s.start(mux)
	env := map[string]string{"AUTOPILOT_SOCKET": s.sockPath}

	baseCode, _, baseErr := s.cliRun(env, "/ws/.autopilot-runs/ACM-7", "cancel", "ACM-7")
	repoCode, _, repoErr := s.cliRun(env, "/ws/.autopilot-runs/ACM-7/acme-api", "cancel", "ACM-7")

	s.Equal(1, baseCode)
	s.Contains(baseErr, "owner")
	s.Equal(1, repoCode)
	s.Contains(repoErr, "owner")
	s.Empty(rec.reqs)
	code, _, errOut := s.cliRun(env, "/ws", "cancel", "ACM-7")
	s.Equal(0, code, errOut)
	s.Equal(socket.Request{Verb: socket.VerbCancel, Key: "ACM-7"}, rec.last())
}

func (s *CLISuite) TestBadArgumentsExit2WithoutCallingService() {
	rec := new(recorder)
	mux := socket.NewMux()
	for _, v := range socket.Verbs {
		mux.Register(v, rec.Handle)
	}
	s.start(mux)
	file := filepath.Join(s.workspace, "note.md")
	s.Require().NoError(os.WriteFile(file, []byte("x"), 0o600))
	bad := [][]string{
		{"ping", "extra"},
		{"runs", "extra"},
		{"post"},
		{"post", "ACM-7"},
		{"post", "ACM-7", "a", "b"},
		{"status", "ACM-7"},
		{"ask"},
		{"ask", "ACM-7"},
		{"ask", "ACM-7", "q", "--option", "noequals"},
		{"ask", "ACM-7", "q", "--option", "=label"},
		{"ask", "ACM-7", "q", "--option"},
		{"ask", "--wait"},
		{"ask", "--wait", "x"},
		{"ask", "--wait", "1", "ACM-7"},
		{"red", "ACM-7", "--repo", "api"},
		{"red", "ACM-7", "--sha", "abc"},
		{"gate", "ACM-7"},
		{"comment", "ACM-7"},
		{"comment", "ACM-7", "--file", filepath.Join(s.workspace, "missing.md")},
		{"manual", "--file", file},
		{"pr", "ACM-7"},
		{"cancel"},
		{"cancel", "ACM-7", "x"},
		{"resume"},
		{"resume", "ACM-7", "x"},
		{"post", "ACM-7", "a", "--bogus", "1"},
	}
	for _, args := range bad {
		code, _, errOut := s.viaEnv(args...)
		s.Equal(2, code, "%v: %s", args, errOut)
		s.Contains(errOut, "usage", "%v", args)
	}
	s.Empty(rec.reqs)
}

func (s *CLISuite) TestUnknownVerbIsNotAVerb() {
	s.False(cli.IsVerb("serve"))
	s.True(cli.IsVerb("ping"))
}

func (s *CLISuite) TestServerAnswersUnknownVerbWithError() {
	s.start(s.store())
	resp, err := socket.Call(s.Ctx, s.sockPath, socket.Request{Verb: "frobnicate"})
	s.Require().NoError(err)
	s.Equal(socket.CodeError, resp.Code)
	s.Contains(resp.Error, "unknown verb")
}

func (s *CLISuite) TestCallToMissingSocketIsServiceDown() {
	ctx, cancel := context.WithTimeout(s.Ctx, time.Second)
	defer cancel()
	_, err := socket.Call(ctx, s.sockPath, socket.Request{Verb: socket.VerbPing})
	s.ErrorIs(err, socket.ErrServiceDown)
}

func (s *CLISuite) TestSocketPathWhosePrivateFolderPassesLimitRefused() {
	// the final path fits in 103 bytes, the bind path with its private folder does not
	dir := filepath.Join(s.workspace, strings.Repeat("d", 95-len(s.workspace)-1))
	s.Require().NoError(os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, "a.sock")
	s.Require().LessOrEqual(len(path), 103)
	_, err := socket.NewServer(path, s.store())
	s.Require().Error(err)
	s.Contains(err.Error(), path)
	s.Contains(err.Error(), "may be at most")
}

func (s *CLISuite) TestUnreadableLiveSocketIsNotReplaced() {
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: s.sockPath, Net: "unix"})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = l.Close() })
	s.Require().NoError(os.Chmod(s.sockPath, 0o000))
	before, err := os.Stat(s.sockPath)
	s.Require().NoError(err)
	_, err = socket.NewServer(s.sockPath, s.store())
	s.Require().Error(err)
	after, statErr := os.Stat(s.sockPath)
	s.Require().NoError(statErr)
	s.True(os.SameFile(before, after), "the live socket file stays")
}

func (s *CLISuite) TestSecondStartFailsWhileTheFirstHoldsTheLock() {
	s.start(s.store())
	// a probe alone would pass: the file is gone, yet the first service lives
	s.Require().NoError(os.Remove(s.sockPath))
	_, err := socket.NewServer(s.sockPath, s.store())
	s.Require().Error(err)
	s.Contains(err.Error(), "already running")
}

func (s *CLISuite) TestOversizedFileRefusedByCLI() {
	s.start(s.store())
	big := filepath.Join(s.workspace, "big.md")
	s.Require().NoError(os.WriteFile(big, bytes.Repeat([]byte("a"), socket.MaxBodyBytes+1), 0o600))
	code, _, errOut := s.viaEnv("comment", "ACM-7", "--file", big)
	s.Equal(2, code)
	s.Contains(errOut, "larger than")
	ok := filepath.Join(s.workspace, "ok.md")
	s.Require().NoError(os.WriteFile(ok, bytes.Repeat([]byte("a"), socket.MaxBodyBytes), 0o600))
	rec := new(recorder)
	mux := socket.NewMux()
	mux.Register(socket.VerbComment, rec.Handle)
	s.stop()
	s.start(mux)
	code, _, errOut = s.viaEnv("comment", "ACM-7", "--file", ok)
	s.Equal(0, code, errOut)
	s.Len(rec.last().Body, socket.MaxBodyBytes)
}

func (s *CLISuite) TestNonRegularFileRefusedByCLI() {
	fifo := filepath.Join(s.workspace, "pipe.md")
	s.Require().NoError(syscall.Mkfifo(fifo, 0o600))
	code, _, errOut := s.viaEnv("manual", "ACM-7", "--file", fifo)
	s.Equal(2, code)
	s.Contains(errOut, "not a regular file")
	code, _, errOut = s.viaEnv("manual", "ACM-7", "--file", "/dev/zero")
	s.Equal(2, code)
	s.Contains(errOut, "not a regular file")
}

func (s *CLISuite) rawDial() *net.UnixConn {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: s.sockPath, Net: "unix"})
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = conn.Close() })
	return conn
}

func (s *CLISuite) requireStillServing() {
	code, out, errOut := s.viaEnv("ping")
	s.Equal(0, code, errOut)
	s.Equal("pong", strings.TrimSpace(out))
}

func (s *CLISuite) TestOversizedRequestNeverReachesHandler() {
	rec := new(recorder)
	mux := socket.NewMux()
	mux.Register(socket.VerbPost, rec.Handle)
	mux.Register(socket.VerbPing, socket.PingHandler)
	s.start(mux)
	conn := s.rawDial()
	payload := append([]byte(`{"verb":"post","key":"ACM-7","text":"`), bytes.Repeat([]byte("a"), socket.MaxRequestBytes+1024)...)
	payload = append(payload, []byte(`"}`)...)
	_, _ = conn.Write(payload) // the server may close first; the write error is expected
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.ReadAll(conn)
	s.Empty(rec.reqs, "an oversized request is never handled")
	s.requireStillServing()
}

func (s *CLISuite) TestMalformedRequestAnswersBadRequest() {
	s.start(s.store())
	conn := s.rawDial()
	_, err := conn.Write([]byte("{not json"))
	s.Require().NoError(err)
	s.Require().NoError(conn.CloseWrite())
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp socket.Response
	s.Require().NoError(json.NewDecoder(conn).Decode(&resp))
	s.Equal(socket.CodeError, resp.Code)
	s.Contains(resp.Error, "bad request")
	s.requireStillServing()
}

func (s *CLISuite) TestSilentClientIsCutAtTheReadDeadline() {
	s.start(s.store(), socket.WithReadTimeout(200*time.Millisecond))
	conn := s.rawDial()
	started := time.Now()
	_ = conn.SetReadDeadline(started.Add(3 * time.Second))
	_, err := io.ReadAll(conn)
	s.Require().NoError(err, "the server closes the connection; the client deadline must not fire")
	s.Less(time.Since(started), 2*time.Second)
	s.requireStillServing()
}

func (s *CLISuite) TestTricklingClientIsCutAtTheReadDeadline() {
	s.start(s.store(), socket.WithReadTimeout(300*time.Millisecond))
	conn := s.rawDial()
	started := time.Now()
	_, err := conn.Write([]byte(`{"verb":"`))
	s.Require().NoError(err)
	cut := false
	for time.Since(started) < 3*time.Second {
		time.Sleep(50 * time.Millisecond)
		if _, err := conn.Write([]byte(" ")); err != nil {
			cut = true
			break
		}
	}
	s.True(cut, "the server closes a client that keeps the request open past the deadline")
	s.Less(time.Since(started), 2*time.Second)
	s.requireStillServing()
}

func (s *CLISuite) TestHandlerPanicAnswersErrorAndServiceKeepsServing() {
	mux := socket.NewMux()
	mux.Register(socket.VerbPing, socket.PingHandler)
	mux.Register(socket.VerbPost, func(context.Context, socket.Request) socket.Response { panic("boom") })
	s.start(mux)
	code, _, errOut := s.viaEnv("post", "ACM-7", "x")
	s.Equal(1, code)
	s.Contains(errOut, "internal error")
	s.requireStillServing()
}

type cannedGate struct {
	verdict domain.Verdict
}

func (g cannedGate) Red(context.Context, domain.IssueKey, string, string) (domain.RedBase, error) {
	return domain.RedBase{}, errors.New("red not proven: acme.add.TestAdd passes on the red base")
}

func (g cannedGate) Run(context.Context, domain.IssueKey, string, string) (domain.Verdict, error) {
	return g.verdict, nil
}

func (s *CLISuite) TestGateVerdictRoundTripsWithItsExitCode() {
	failing := domain.Verdict{Repo: "acme-api", SHA: "abc1234", Checks: []domain.CheckResult{
		{Name: "tests-pass", Status: domain.CheckPass},
		{Name: "mutation", Status: domain.CheckFail, Detail: "mutation timed out after 15m0s"},
	}, Manual: []string{"AC2: open the page"}}
	passing := domain.Verdict{Repo: "acme-api", SHA: "abc1234", Checks: []domain.CheckResult{{Name: "tests-pass", Status: domain.CheckPass}}}
	for _, tt := range []struct {
		name    string
		verdict domain.Verdict
		code    int
		want    []string
	}{
		{"fail", failing, 1, []string{"gate FAIL for acme-api at abc1234", "mutation\tfail\tmutation timed out", "manual\tAC2: open the page"}},
		{"pass", passing, 0, []string{"gate PASS for acme-api at abc1234"}},
	} {
		mux := socket.NewMux()
		mux.Register(socket.VerbGate, socket.NewGateHandler(cannedGate{verdict: tt.verdict}))
		mux.Register(socket.VerbRed, socket.NewRedHandler(cannedGate{}))
		s.start(mux)
		code, out, _ := s.viaEnv("gate", "ACM-7", "--repo", "acme-api", "--sha", "abc1234")
		s.Equal(tt.code, code, tt.name)
		for _, w := range tt.want {
			s.Contains(out, w, tt.name)
		}
		code, _, errOut := s.viaEnv("red", "ACM-7", "--repo", "acme-api", "--sha", "abc1234")
		s.Equal(1, code, tt.name)
		s.Contains(errOut, "passes on the red base", tt.name)
		s.stop()
	}
}
