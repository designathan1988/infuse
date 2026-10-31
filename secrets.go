package main

// Secret store: API keys go to the OS vault (Windows Credential Manager, macOS Keychain, libsecret).
// Tests replace secretBackend with an in-memory map.

type secretStore interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
}

var secretBackend secretStore = osSecrets{}

func secretGet(name string) (string, error) { return secretBackend.Get(name) }
func secretSet(name, v string) error        { return secretBackend.Set(name, v) }
func secretDelete(name string) error        { return secretBackend.Delete(name) }

type memSecrets map[string]string

func (m memSecrets) Get(n string) (string, error) { return m[n], nil }
func (m memSecrets) Set(n, v string) error        { m[n] = v; return nil }
func (m memSecrets) Delete(n string) error        { delete(m, n); return nil }
