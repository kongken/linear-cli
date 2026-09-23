package keyring

import gokeyring "github.com/zalando/go-keyring"

// OSStore uses the platform keyring via zalando/go-keyring.
type OSStore struct{}

func (OSStore) Get(service, user string) (string, error) {
	return gokeyring.Get(service, user)
}
func (OSStore) Set(service, user, password string) error {
	return gokeyring.Set(service, user, password)
}
func (OSStore) Delete(service, user string) error {
	return gokeyring.Delete(service, user)
}

func init() {
	// Prefer OS keyring when available; MemoryStore remains default until UseOS() is called.
}

// UseOS switches the process-wide store to the OS keyring.
func UseOS() {
	SetDefaultStore(OSStore{})
}
