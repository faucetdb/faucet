package connector

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Factory is a function that creates a new Connector instance.
type Factory func() Connector

// Errors returned (wrapped) by Registry.Get, so callers can tell a service
// that doesn't exist from one that is paused or failed to connect.
var (
	ErrServiceNotFound     = errors.New("service not found")
	ErrServicePaused       = errors.New("service is paused")
	ErrServiceNotConnected = errors.New("service is not connected")
)

// retireDelay is how long a replaced connector stays open so requests that
// already fetched it can finish before its pool is closed. Zero closes it
// immediately.
var retireDelay = 30 * time.Second

// ServiceState is the live state of a service in the registry.
type ServiceState struct {
	Connected bool
	Paused    bool
	// Error is the last connection error, set when the service is neither
	// connected nor paused because connecting failed.
	Error string
}

// Registry manages connector factories and active connections.
//
// Every change to a service (connect, pause, fail, delete) bumps its
// generation. A Connect only commits its result if no newer change happened
// while the driver was connecting, so a slow connect can't undo a later
// pause or delete, or override a newer edit.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	active    map[string]Connector // keyed by service name
	paused    map[string]bool      // services deliberately taken offline
	failed    map[string]string    // last connection error, by service name
	gen       map[string]uint64    // per-service change counter; never reset
	retiring  map[Connector]*time.Timer
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
		active:    make(map[string]Connector),
		paused:    make(map[string]bool),
		failed:    make(map[string]string),
		gen:       make(map[string]uint64),
		retiring:  make(map[Connector]*time.Timer),
	}
}

// RegisterDriver registers a connector factory for a driver type.
func (r *Registry) RegisterDriver(driver string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[driver] = factory
}

// Connect creates a new connector for the given driver and connects it,
// replacing any existing connection for the service. The driver connects
// without holding the registry lock, so a slow database doesn't block
// requests to other services.
//
// If connecting fails, the previous connection (if any) is dropped as well:
// the service then reports ErrServiceNotConnected instead of silently
// serving the database it was connected to before an edit. If the service
// was paused, deleted or reconnected while this call was connecting, its
// result is discarded.
func (r *Registry) Connect(serviceName string, cfg ConnectionConfig) error {
	r.mu.Lock()
	r.gen[serviceName]++
	g := r.gen[serviceName]
	factory, ok := r.factories[cfg.Driver]
	available := r.availableDrivers()
	r.mu.Unlock()

	var err error
	var conn Connector
	if !ok {
		err = fmt.Errorf("unsupported driver: %s (available: %v)", cfg.Driver, available)
	} else {
		conn = factory()
		if cerr := conn.Connect(cfg); cerr != nil {
			err = fmt.Errorf("failed to connect service %q: %w", serviceName, cerr)
			conn = nil
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gen[serviceName] != g {
		// Superseded by a newer pause, delete or connect.
		if conn != nil {
			conn.Disconnect()
		}
		return err
	}
	if err != nil {
		r.failLocked(serviceName, err)
		return err
	}
	if old, ok := r.active[serviceName]; ok {
		r.retireLocked(old)
	}
	r.active[serviceName] = conn
	delete(r.paused, serviceName)
	delete(r.failed, serviceName)
	return nil
}

// Fail records that a service could not be connected and drops any existing
// connection for it.
func (r *Registry) Fail(serviceName string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gen[serviceName]++
	r.failLocked(serviceName, err)
}

func (r *Registry) failLocked(serviceName string, err error) {
	if old, ok := r.active[serviceName]; ok {
		r.retireLocked(old)
		delete(r.active, serviceName)
	}
	delete(r.paused, serviceName)
	r.failed[serviceName] = err.Error()
}

// Pause disconnects a service and marks it paused, so lookups report
// ErrServicePaused until it is connected again.
func (r *Registry) Pause(serviceName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.gen[serviceName]++
	if conn, ok := r.active[serviceName]; ok {
		conn.Disconnect()
		delete(r.active, serviceName)
	}
	delete(r.failed, serviceName)
	r.paused[serviceName] = true
}

// retireLocked closes a replaced connector after retireDelay. r.mu must be
// held.
func (r *Registry) retireLocked(conn Connector) {
	if retireDelay <= 0 {
		conn.Disconnect()
		return
	}
	r.retiring[conn] = time.AfterFunc(retireDelay, func() {
		r.mu.Lock()
		_, pending := r.retiring[conn]
		delete(r.retiring, conn)
		r.mu.Unlock()
		if pending {
			conn.Disconnect()
		}
	})
}

// Get returns the connector for a service. The error wraps
// ErrServicePaused, ErrServiceNotConnected or ErrServiceNotFound.
func (r *Registry) Get(serviceName string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if conn, ok := r.active[serviceName]; ok {
		return conn, nil
	}
	if r.paused[serviceName] {
		return nil, fmt.Errorf("service %q: %w", serviceName, ErrServicePaused)
	}
	if msg, ok := r.failed[serviceName]; ok {
		return nil, fmt.Errorf("service %q: %w: %s", serviceName, ErrServiceNotConnected, msg)
	}
	return nil, fmt.Errorf("service %q not found (available: %v): %w", serviceName, r.activeServices(), ErrServiceNotFound)
}

// State reports whether a service is connected, paused, or failed.
func (r *Registry) State(serviceName string) ServiceState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, connected := r.active[serviceName]
	return ServiceState{
		Connected: connected,
		Paused:    r.paused[serviceName],
		Error:     r.failed[serviceName],
	}
}

// Failures returns the services whose last connection attempt failed, with
// the error for each.
func (r *Registry) Failures() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]string, len(r.failed))
	for name, msg := range r.failed {
		out[name] = msg
	}
	return out
}

// Disconnect removes and disconnects a service and forgets its state.
func (r *Registry) Disconnect(serviceName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.gen[serviceName]++
	delete(r.paused, serviceName)
	delete(r.failed, serviceName)
	conn, ok := r.active[serviceName]
	if !ok {
		return fmt.Errorf("service %q not found", serviceName)
	}

	err := conn.Disconnect()
	delete(r.active, serviceName)
	return err
}

// CloseAll disconnects all services.
func (r *Registry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, conn := range r.active {
		conn.Disconnect()
		delete(r.active, name)
	}
	for conn, timer := range r.retiring {
		timer.Stop()
		conn.Disconnect()
		delete(r.retiring, conn)
	}
}

// ListServices returns active service names.
func (r *Registry) ListServices() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.active))
	for name := range r.active {
		names = append(names, name)
	}
	return names
}

// Drivers returns the registered driver names in sorted order.
func (r *Registry) Drivers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d := r.availableDrivers()
	sort.Strings(d)
	return d
}

func (r *Registry) availableDrivers() []string {
	drivers := make([]string, 0, len(r.factories))
	for d := range r.factories {
		drivers = append(drivers, d)
	}
	return drivers
}

func (r *Registry) activeServices() []string {
	names := make([]string, 0, len(r.active))
	for n := range r.active {
		names = append(names, n)
	}
	return names
}

// ProbeResult describes a successful trial connection.
type ProbeResult struct {
	Tables []string
}

// Probe opens a throwaway connection with cfg, pings it, lists its tables,
// and closes it again. Nothing is registered, so it is safe to call with
// unsaved settings from the admin UI's "Test connection" button.
func (r *Registry) Probe(ctx context.Context, cfg ConnectionConfig) (*ProbeResult, error) {
	r.mu.RLock()
	factory, ok := r.factories[cfg.Driver]
	available := r.availableDrivers()
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported driver: %s (available: %v)", cfg.Driver, available)
	}

	type outcome struct {
		res *ProbeResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		conn := factory()
		// Keep the trial pool tiny; it only lives for this probe.
		cfg.MaxOpenConns, cfg.MaxIdleConns = 1, 1
		if err := conn.Connect(cfg); err != nil {
			done <- outcome{err: err}
			return
		}
		defer conn.Disconnect()
		if err := conn.Ping(ctx); err != nil {
			done <- outcome{err: err}
			return
		}
		tables, err := conn.GetTableNames(ctx)
		if err != nil {
			done <- outcome{err: fmt.Errorf("connected, but listing tables failed: %w", err)}
			return
		}
		done <- outcome{res: &ProbeResult{Tables: tables}}
	}()

	// Driver Connect calls are not context-aware, so bound them here. A
	// connect that outlives the deadline finishes and cleans up on its own.
	select {
	case o := <-done:
		return o.res, o.err
	case <-ctx.Done():
		return nil, fmt.Errorf("timed out waiting for the database to respond")
	}
}
