// Package planeauth stores Plane tokens separately from all Linear credentials.
package planeauth

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/kongken/linear-cli/internal/keyring"
	"github.com/kongken/linear-cli/internal/planeconfig"
	gokeyring "github.com/zalando/go-keyring"
)

const service = "workctl-plane"

type Manager struct {
	Resolver planeconfig.Resolver
	Store    keyring.Store
}
type secrets struct {
	SchemaVersion  int               `toml:"schema_version"`
	Tokens         map[string]string `toml:"tokens"`
	Storage        map[string]string `toml:"storage"`
	KeyringEntries map[string]bool   `toml:"keyring_entries"`
}

func identity(c planeconfig.Context) (string, error) {
	p, err := planeconfig.Normalize(c.Profile)
	if err != nil {
		return "", err
	}
	if c.ProfileName == "" {
		return "", fmt.Errorf("a named Plane profile is required for stored credentials")
	}
	sum := sha256.Sum256([]byte(p.BaseURL + "\x00" + p.Workspace + "\x00" + c.ProfileName))
	return hex.EncodeToString(sum[:]), nil
}
func (m Manager) store() keyring.Store {
	if m.Store != nil {
		return m.Store
	}
	return keyring.OSStore{}
}
func (m Manager) path() (string, error) {
	p, e := m.Resolver.Path()
	return filepath.Join(filepath.Dir(p), "credentials.toml"), e
}
func (m Manager) load() (secrets, error) {
	s := secrets{SchemaVersion: 1, Tokens: map[string]string{}, Storage: map[string]string{}, KeyringEntries: map[string]bool{}}
	path, err := m.path()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("read Plane credentials: %w", err)
	}
	s.SchemaVersion = 0
	meta, err := toml.Decode(string(data), &s)
	if err != nil || len(meta.Undecoded()) > 0 || s.SchemaVersion != 1 {
		return s, fmt.Errorf("invalid Plane credentials file")
	}
	if s.Tokens == nil {
		s.Tokens = map[string]string{}
	}
	if s.Storage == nil {
		s.Storage = map[string]string{}
	}
	if s.KeyringEntries == nil {
		s.KeyringEntries = map[string]bool{}
	}
	for _, storage := range s.Storage {
		if storage != "keyring" && storage != "plaintext" && storage != "logged_out" {
			return s, fmt.Errorf("invalid Plane credential storage metadata")
		}
	}
	return s, nil
}
func (m Manager) write(s secrets) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(s); err != nil {
		return err
	}
	path, err := m.path()
	if err != nil {
		return err
	}
	return planeconfig.WritePrivate(path, buf.Bytes())
}
func (m Manager) Token(c planeconfig.Context) (string, error) {
	id, err := identity(c)
	if err != nil {
		return "", err
	}
	s, err := m.load()
	if err != nil {
		return "", err
	}
	if s.Storage[id] == "logged_out" {
		return "", fmt.Errorf("Plane profile %q is logged out; run workctl plane auth login", c.ProfileName)
	}
	if token := s.Tokens[id]; token != "" {
		return token, nil
	}
	if s.Storage[id] == "plaintext" {
		return "", fmt.Errorf("plaintext Plane credential is missing; run workctl plane auth login")
	}
	token, err := m.store().Get(service, id)
	if err != nil || token == "" {
		return "", fmt.Errorf("no stored Plane credential for profile %q; use PLANE_API_KEY or workctl plane auth login", c.ProfileName)
	}
	return token, nil
}
func (m Manager) Save(c planeconfig.Context, token string, plaintext bool) error {
	id, err := identity(c)
	if err != nil {
		return err
	}
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("invalid Plane token")
	}
	s, err := m.load()
	if err != nil {
		return err
	}
	if plaintext {
		s.Tokens[id] = token
		s.Storage[id] = "plaintext"
		return m.write(s)
	}
	if err := m.store().Set(service, id, token); err != nil {
		return fmt.Errorf("cannot save Plane token in system keyring; use PLANE_API_KEY or explicitly choose --plaintext")
	}
	delete(s.Tokens, id)
	s.Storage[id] = "keyring"
	s.KeyringEntries[id] = true
	if err := m.write(s); err != nil {
		return fmt.Errorf("keyring saved but unable to update credential metadata: %w", err)
	}
	return nil
}

func (m Manager) Logout(c planeconfig.Context) error {
	id, err := identity(c)
	if err != nil {
		return err
	}
	s, err := m.load()
	if err != nil {
		return err
	}
	_, plain := s.Tokens[id]
	keyringEntry := s.KeyringEntries[id] || s.Storage[id] == "keyring" || (s.Storage[id] == "" && !plain)
	delete(s.Tokens, id)
	// Remember logout so a stale credential from a previous storage method is never reused.
	s.Storage[id] = "logged_out"
	if err := m.write(s); err != nil {
		return err
	}
	if keyringEntry {
		err = m.store().Delete(service, id)
		if err != nil && !errors.Is(err, gokeyring.ErrNotFound) {
			return fmt.Errorf("profile is logged out, but an old system-keyring copy could not be removed; retry logout when the keyring is available")
		}
		delete(s.KeyringEntries, id)
		return m.write(s)
	}
	return nil
}
