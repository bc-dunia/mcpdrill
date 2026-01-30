package plugin

import (
	"sort"
	"sync"
)

// Registry manages registered operations.
type Registry struct {
	operations map[string]Operation
	mu         sync.RWMutex
}

// NewRegistry creates a new operation registry.
func NewRegistry() *Registry {
	return &Registry{
		operations: make(map[string]Operation),
	}
}

// Register adds an operation to the registry.
// Returns an error if an operation with the same name is already registered.
func (r *Registry) Register(op Operation) error {
	if op == nil {
		return NewRegistrationError("", "operation cannot be nil")
	}

	name := op.Name()
	if name == "" {
		return NewRegistrationError("", "operation name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.operations[name]; exists {
		return NewRegistrationError(name, "operation already registered")
	}

	r.operations[name] = op
	return nil
}

// MustRegister adds an operation to the registry, panicking on error.
// This is intended for use in init() functions.
func (r *Registry) MustRegister(op Operation) {
	if err := r.Register(op); err != nil {
		panic(err)
	}
}

// Get retrieves an operation by name.
// Returns the operation and true if found, or nil and false if not found.
func (r *Registry) Get(name string) (Operation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	op, exists := r.operations[name]
	return op, exists
}

// List returns a sorted list of all registered operation names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.operations))
	for name := range r.operations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Unregister removes an operation from the registry.
// Returns true if the operation was removed, false if it wasn't registered.
func (r *Registry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.operations[name]; !exists {
		return false
	}

	delete(r.operations, name)
	return true
}

// Count returns the number of registered operations.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.operations)
}

// DefaultRegistry is the global default registry used by the VU engine.
var DefaultRegistry = NewRegistry()

// Register adds an operation to the default registry.
func Register(op Operation) error {
	return DefaultRegistry.Register(op)
}

// MustRegister adds an operation to the default registry, panicking on error.
func MustRegister(op Operation) {
	DefaultRegistry.MustRegister(op)
}

// Get retrieves an operation from the default registry.
func Get(name string) (Operation, bool) {
	return DefaultRegistry.Get(name)
}

// List returns all operation names from the default registry.
func List() []string {
	return DefaultRegistry.List()
}
