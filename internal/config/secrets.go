package config

// Secret store: API keys go to the OS vault (Windows Credential Manager, macOS Keychain, libsecret).
// Tests replace SecretBackend with an in-memory map.

type SecretStore interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
}

var SecretBackend SecretStore = osSecrets{}

func SecretGet(name string) (string, error) { return SecretBackend.Get(name) }
func SecretSet(name, v string) error        { return SecretBackend.Set(name, v) }
func SecretDelete(name string) error        { return SecretBackend.Delete(name) }

type MemSecrets map[string]string

func (m MemSecrets) Get(n string) (string, error) { return m[n], nil }
func (m MemSecrets) Set(n, v string) error        { m[n] = v; return nil }
func (m MemSecrets) Delete(n string) error        { delete(m, n); return nil }
