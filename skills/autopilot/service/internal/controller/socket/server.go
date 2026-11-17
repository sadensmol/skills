package socket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

// maxPathBytes is the AF_UNIX path limit of macOS (104 with the terminating zero).
const maxPathBytes = 103

const (
	// MaxRequestBytes caps one request on the wire.
	MaxRequestBytes = 4 << 20
	// MaxBodyBytes caps a body the CLI sends; it leaves room for the other fields under MaxRequestBytes.
	MaxBodyBytes = 1 << 20

	defaultReadTimeout = 10 * time.Second
	probeTimeout       = time.Second

	// tempOverhead is the longest "/.s<uint32>/s" the private folder adds to the socket's folder path.
	tempOverhead = len("/.s4294967295/s")
)

// ErrAlreadyRunning marks a start that found a live service on the socket.
var ErrAlreadyRunning = errors.New("already running")

type Server struct {
	path        string
	listener    *net.UnixListener
	lock        *os.File
	handler     IHandler
	readTimeout time.Duration
}

type ServerOption func(*Server)

// WithReadTimeout sets the absolute time a client has to send its whole request.
func WithReadTimeout(d time.Duration) ServerOption {
	return func(s *Server) { s.readTimeout = d }
}

// NewServer binds the socket at path with mode 600 and no window where it is wider: the socket is
// created in a private folder (mode 700), chmod-ed, then renamed to path. A lock file next to the
// socket, held for the life of the server, makes a second start fail. A leftover file that refuses
// connections is stale and replaced; any other probe result, a non-socket file or a held lock is an error.
func NewServer(path string, handler IHandler, opts ...ServerOption) (*Server, error) {
	dir := filepath.Dir(path)
	if err := checkPathLength(path, dir); err != nil {
		return nil, err
	}
	lock, err := acquireLock(path)
	if err != nil {
		return nil, err
	}
	listener, err := bind(path, dir)
	if err != nil {
		releaseLock(lock)
		return nil, err
	}
	s := &Server{path: path, listener: listener, lock: lock, handler: handler, readTimeout: defaultReadTimeout}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func bind(path, dir string) (*net.UnixListener, error) {
	if err := checkNoLiveSocket(path); err != nil {
		return nil, err
	}
	private, err := os.MkdirTemp(dir, ".s")
	if err != nil {
		return nil, fmt.Errorf("cannot create the private folder for socket %s: %w", path, err)
	}
	defer os.RemoveAll(private) //nolint:errcheck // the folder is empty after the rename; a leftover is harmless
	tmpPath := filepath.Join(private, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: tmpPath, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("cannot bind socket %s: %w", path, err)
	}
	listener.SetUnlinkOnClose(false)
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		closeQuietly(listener)
		return nil, fmt.Errorf("cannot set mode 600 on socket %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		closeQuietly(listener)
		return nil, fmt.Errorf("cannot place socket %s: %w", path, err)
	}
	return listener, nil
}

// acquireLock takes an exclusive lock on <path>.lock; the kernel drops it when the service dies.
func acquireLock(path string) (*os.File, error) {
	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot open lock %s: %w", lockPath, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		releaseLock(f)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("a service is already running on socket %s (lock %s is held): %w", path, lockPath, ErrAlreadyRunning)
		}
		return nil, fmt.Errorf("cannot lock %s: %w", lockPath, err)
	}
	return f, nil
}

func releaseLock(f *os.File) {
	if err := f.Close(); err != nil {
		log.Warn().Err(err).Msg("cannot close the lock file")
	}
}

func closeQuietly(l *net.UnixListener) {
	if err := l.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Warn().Err(err).Msg("cannot close the socket listener")
	}
}

// checkPathLength refuses a socket whose bind path, including the private folder, passes the AF_UNIX limit.
func checkPathLength(socketPath, dir string) error {
	if len(socketPath) > maxPathBytes {
		return fmt.Errorf("socket path %s is too long: %d bytes, the limit is %d", socketPath, len(socketPath), maxPathBytes)
	}
	if need := len(dir) + tempOverhead; need > maxPathBytes {
		return fmt.Errorf("socket path %s is too long: the bind needs %d bytes (the private folder adds %d), the limit is %d, so its folder may be at most %d bytes",
			socketPath, need, tempOverhead, maxPathBytes, maxPathBytes-tempOverhead)
	}
	return nil
}

// checkNoLiveSocket accepts a missing file or a stale socket (connection refused); anything else is an error.
func checkNoLiveSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot check socket %s: %w", path, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists and is not a socket", path)
	}
	conn, err := net.DialTimeout("unix", path, probeTimeout)
	if err == nil {
		if err := conn.Close(); err != nil {
			log.Warn().Err(err).Msg("cannot close the probe connection")
		}
		return fmt.Errorf("a service is already running on socket %s: %w", path, ErrAlreadyRunning)
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT) {
		return nil // nobody listens: a stale file from a crashed service
	}
	return fmt.Errorf("cannot tell whether socket %s is stale: %w", path, err)
}

func (s *Server) Path() string { return s.path }

// Close frees a server that never served: it closes the listener, removes the socket file and drops the lock.
// A server that Serve has run needs no Close.
func (s *Server) Close() error {
	closeQuietly(s.listener)
	defer releaseLock(s.lock)
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot remove socket %s: %w", s.path, err)
	}
	return nil
}

// Serve answers requests until ctx ends, then closes the listener, waits for open requests and removes the socket file.
func (s *Server) Serve(ctx context.Context) error {
	var wg sync.WaitGroup
	connCtx, cancelConns := context.WithCancel(ctx)
	defer cancelConns()
	go func() {
		<-ctx.Done()
		closeQuietly(s.listener)
	}()
	var serveErr error
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() == nil {
				serveErr = fmt.Errorf("cannot accept on socket %s: %w", s.path, err)
			}
			break
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.serveConn(connCtx, conn)
		}()
	}
	closeQuietly(s.listener)
	cancelConns()
	wg.Wait()
	defer releaseLock(s.lock)
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) && serveErr == nil {
		serveErr = fmt.Errorf("cannot remove socket %s: %w", s.path, err)
	}
	return serveErr
}

func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Debug().Err(err).Msg("cannot close a socket connection")
		}
	}()
	// a read that blocks must not hold up shutdown
	stop := context.AfterFunc(ctx, func() { closeConn(conn) })
	defer stop()
	if err := conn.SetReadDeadline(time.Now().Add(s.readTimeout)); err != nil {
		return
	}
	var req Request
	if err := json.NewDecoder(io.LimitReader(conn, MaxRequestBytes)).Decode(&req); err != nil {
		write(conn, Response{Code: CodeError, Error: fmt.Sprintf("bad request: %v", err)})
		return
	}
	// a waiting verb ends when the client goes away
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go watchClose(conn, cancel)
	log.Debug().Ctx(ctx).Msgf("socket verb %s", req.Verb)
	write(conn, s.handle(reqCtx, req))
}

// handle turns a handler panic into an error answer, so one bad request cannot stop the service.
func (s *Server) handle(ctx context.Context, req Request) (resp Response) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Ctx(ctx).Msgf("handler of verb %s panicked: %v", req.Verb, r)
			resp = Response{Code: CodeError, Error: fmt.Sprintf("%s: internal error", req.Verb)}
		}
	}()
	return s.handler.Handle(ctx, req)
}

// watchClose cancels the request when the client closes its side: the client sends nothing more after its request.
func watchClose(conn net.Conn, cancel context.CancelFunc) {
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return
	}
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != nil {
		cancel()
	}
}

func closeConn(conn net.Conn) {
	if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Debug().Err(err).Msg("cannot close a socket connection")
	}
}

func write(conn net.Conn, resp Response) {
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		log.Debug().Err(err).Msg("cannot write a socket response")
	}
}
