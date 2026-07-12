// Package daemon provides cross-platform daemon lifecycle management.
//
// It handles process locking (flock / LockFileEx), IPC listeners
// (Unix sockets / Windows named pipes), info file management,
// and remote daemon control (start, stop, dial).
//
// Basic usage for the daemon process itself:
//
//	d := daemon.New(daemon.Config{Name: "myapp"})
//	if err := d.Lock(); err != nil { log.Fatal(err) }
//	defer d.Shutdown()
//
//	ln, err := d.Listen(nil)
//	// use ln with your server ...
//
// Remote operations from a CLI:
//
//	cfg := daemon.Config{Name: "myapp"}
//	daemon.Stop(cfg)
//	daemon.Start(cfg, "daemon", "run")
//	conn, err := daemon.Dial(cfg, 5*time.Second)
package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// Sentinel errors for daemon lifecycle.
var (
	ErrNotLocked        = errors.New("daemon: not locked")
	ErrAlreadyLocked    = errors.New("daemon: already locked")
	ErrAlreadyListening = errors.New("daemon: already listening")
)

// Daemon manages the lifecycle of a running daemon process.
// Call [New] to create, then [Lock], [Listen], and eventually [Shutdown].
type Daemon struct {
	config   Config
	lock     *os.File
	listener net.Listener
	info     Info
	mu       sync.Mutex
	locked   bool
}

// New creates a daemon manager with the given configuration.
func New(config Config) *Daemon {
	return &Daemon{config: config}
}

// Config returns the daemon's configuration.
func (d *Daemon) Config() Config { return d.config }

// Info returns the daemon's advertised info. Only valid after [Listen].
func (d *Daemon) Info() Info { return d.info }

// Lock acquires an exclusive file lock to ensure only one daemon instance runs.
// Must be called before [Listen]. The lock is released by [Shutdown] or process death.
func (d *Daemon) Lock() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.locked {
		return ErrAlreadyLocked
	}

	if err := ensureDir(d.config.runtimeDir); err != nil {
		return err
	}

	lockPath, err := d.config.lockPath()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return wrapErr("open lock file", err)
	}

	if err := lockFile(f); err != nil {
		_ = f.Close()
		return wrapErr("acquire lock (another daemon may be running)", err)
	}

	// Write PID into lock file for diagnostics
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())

	d.lock = f
	d.locked = true
	return nil
}

// Listen creates a platform-appropriate listener (Unix socket or Windows named pipe),
// then writes the info file so clients can discover this daemon.
//
// The metadata parameter is stored as-is in the info file—use it to advertise
// app-specific fields (e.g., protocol version). Pass nil if no metadata is needed.
func (d *Daemon) Listen(metadata json.RawMessage) (net.Listener, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.locked {
		return nil, ErrNotLocked
	}
	if d.listener != nil {
		return nil, ErrAlreadyListening
	}

	if err := ensureDir(d.config.runtimeDir); err != nil {
		return nil, err
	}

	endpoint, err := d.config.resolveEndpoint()
	if err != nil {
		return nil, err
	}

	listener, err := createListener(endpoint)
	if err != nil {
		return nil, wrapErr("create listener", err)
	}

	info := Info{
		PID:       os.Getpid(),
		Transport: endpoint.Transport,
		Endpoint:  endpoint.Endpoint,
		StartedAt: time.Now().UnixMilli(),
		Metadata:  metadata,
	}

	if err := writeInfo(d.config, info); err != nil {
		_ = listener.Close()
		return nil, wrapErr("write info file", err)
	}

	d.listener = listener
	d.info = info
	return listener, nil
}

// Shutdown cleans up all daemon resources: removes the info file,
// closes the listener, and releases the file lock.
// Safe to call multiple times.
func (d *Daemon) Shutdown() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_ = removeInfo(d.config)

	if d.listener != nil {
		_ = d.listener.Close()
		d.listener = nil
	}

	if d.lock != nil {
		_ = d.lock.Close()
		d.lock = nil
		d.locked = false
	}

	return nil
}

func ensureDir(dirFn func() (string, error)) error {
	dir, err := dirFn()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

func wrapErr(msg string, err error) error {
	return fmt.Errorf("%s: %w", msg, err)
}
