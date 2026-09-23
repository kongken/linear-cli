package keyring

import "fmt"

// Store is an OS keyring abstraction (injectable for tests).
type Store interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

// MemoryStore is an in-memory Store for tests.
type MemoryStore struct {
	data map[string]string
}

// NewMemoryStore creates an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: map[string]string{}}
}

func key(service, user string) string {
	return service + "\x00" + user
}

func (m *MemoryStore) Get(service, user string) (string, error) {
	v, ok := m.data[key(service, user)]
	if !ok {
		return "", fmt.Errorf("keyring: not found")
	}
	return v, nil
}

func (m *MemoryStore) Set(service, user, password string) error {
	m.data[key(service, user)] = password
	return nil
}

func (m *MemoryStore) Delete(service, user string) error {
	delete(m.data, key(service, user))
	return nil
}

var defaultStore Store = NewMemoryStore()

// SetDefaultStore replaces the process-wide store (tests / platform wiring).
func SetDefaultStore(s Store) {
	defaultStore = s
}

// Default returns the process-wide store.
func Default() Store {
	return defaultStore
}
