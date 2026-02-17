package connector

import (
	"fmt"
	"sync"
)

// Factory is a function that creates a new Connector instance.
type Factory func() Connector

// Registry manages connector factories and active connections.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	active    map[string]Connector // keyed by service name
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
		active:    make(map[string]Connector),
	}
}

// RegisterDriver registers a connector factory for a driver type.
func (r *Registry) RegisterDriver(driver string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[driver] = factory
}

// Connect creates a new connector for the given driver and connects it.
func (r *Registry) Connect(serviceName string, cfg ConnectionConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	factory, ok := r.factories[cfg.Driver]
	if !ok {
		return fmt.Errorf("unsupported driver: %s (available: %v)", cfg.Driver, r.availableDrivers())
	}

	conn := factory()
	if err := conn.Connect(cfg); err != nil {
		return fmt.Errorf("failed to connect service %q: %w", serviceName, err)
	}

	// Close existing connection if any
	if existing, ok := r.active[serviceName]; ok {
		existing.Disconnect()
	}

	r.active[serviceName] = conn
	return nil
}

// Get returns the connector for a service.
func (r *Registry) Get(serviceName string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn, ok := r.active[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %q not found (available: %v)", serviceName, r.activeServices())
	}
	return conn, nil
}

// Disconnect removes and disconnects a service.
func (r *Registry) Disconnect(serviceName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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
