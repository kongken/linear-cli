package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/kongken/linear-cli/internal/keyring"
)

// Credentials holds workspace credentials metadata.
type Credentials struct {
	Default    string
	Workspaces []string
}

var (
	mu             sync.Mutex
	apiKeyCache    = map[string]string{}
	loaded         Credentials
	inlineFormat   bool
	pathOverride   string
	keyringService = "linear-cli"
)

// SetPathOverride forces credentials path (tests).
func SetPathOverride(path string) {
	mu.Lock()
	defer mu.Unlock()
	pathOverride = path
}

// CredentialsPath returns the path to credentials.toml (XDG on Unix, APPDATA on Windows).
func CredentialsPath() (string, error) {
	mu.Lock()
	override := pathOverride
	mu.Unlock()
	if override != "" {
		return override, nil
	}
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA is not set")
		}
		return filepath.Join(appData, "linear", "credentials.toml"), nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "linear", "credentials.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "linear", "credentials.toml"), nil
}

// Load reads credentials from the default path. Missing file yields empty credentials.
func Load() (Credentials, error) {
	path, err := CredentialsPath()
	if err != nil {
		return Credentials{}, err
	}
	creds, err := LoadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			mu.Lock()
			apiKeyCache = map[string]string{}
			loaded = Credentials{}
			inlineFormat = false
			mu.Unlock()
			return Credentials{}, nil
		}
		return Credentials{}, err
	}
	return creds, nil
}

// LoadFile parses a credentials.toml.
func LoadFile(path string) (Credentials, error) {
	mu.Lock()
	defer mu.Unlock()

	apiKeyCache = map[string]string{}
	loaded = Credentials{}
	inlineFormat = false

	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, err
	}

	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return Credentials{}, fmt.Errorf("parse credentials at %s: %w", path, err)
	}

	if hasInlineKeys(raw) {
		inlineFormat = true
		loaded = parseInline(raw)
		return loaded, nil
	}

	inlineFormat = false
	loaded = parseKeyringFormat(raw)
	for _, ws := range loaded.Workspaces {
		key, err := keyring.Default().Get(keyringService, ws)
		if err == nil && key != "" {
			apiKeyCache[ws] = key
		}
	}
	return loaded, nil
}

func hasInlineKeys(raw map[string]any) bool {
	for k, v := range raw {
		if k == "default" {
			continue
		}
		if k == "workspaces" {
			return false
		}
		if _, ok := v.(string); ok {
			return true
		}
	}
	return false
}

func parseInline(raw map[string]any) Credentials {
	var workspaces []string
	var def string
	if d, ok := raw["default"].(string); ok {
		def = d
	}
	for k, v := range raw {
		if k == "default" {
			continue
		}
		if s, ok := v.(string); ok {
			workspaces = append(workspaces, k)
			apiKeyCache[k] = s
		}
	}
	sort.Strings(workspaces)
	return Credentials{Default: def, Workspaces: workspaces}
}

func parseKeyringFormat(raw map[string]any) Credentials {
	var def string
	if d, ok := raw["default"].(string); ok {
		def = d
	}
	var workspaces []string
	if arr, ok := raw["workspaces"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				workspaces = append(workspaces, s)
			}
		}
	}
	sort.Strings(workspaces)
	return Credentials{Default: def, Workspaces: workspaces}
}

// GetAPIKey returns a cached API key for a workspace.
// If workspace is empty, the default workspace key is returned when set.
func GetAPIKey(workspace string) (string, bool) {
	mu.Lock()
	defer mu.Unlock()
	if workspace == "" {
		workspace = loaded.Default
	}
	if workspace == "" {
		return "", false
	}
	key, ok := apiKeyCache[workspace]
	return key, ok && key != ""
}

// DefaultWorkspace returns the default workspace slug.
func DefaultWorkspace() string {
	mu.Lock()
	defer mu.Unlock()
	return loaded.Default
}

// Workspaces returns configured workspace slugs.
func Workspaces() []string {
	mu.Lock()
	defer mu.Unlock()
	out := make([]string, len(loaded.Workspaces))
	copy(out, loaded.Workspaces)
	return out
}

// HasWorkspace reports whether workspace is configured.
func HasWorkspace(workspace string) bool {
	mu.Lock()
	defer mu.Unlock()
	for _, ws := range loaded.Workspaces {
		if ws == workspace {
			return true
		}
	}
	return false
}

// UsingInlineFormat reports whether credentials are stored as plaintext TOML.
func UsingInlineFormat() bool {
	mu.Lock()
	defer mu.Unlock()
	return inlineFormat
}

// AddCredential stores an API key. plaintext=true forces inline TOML storage.
func AddCredential(workspace, apiKey string, plaintext bool) error {
	mu.Lock()
	defer mu.Unlock()

	useInline := plaintext || inlineFormat
	if plaintext {
		useInline = true
	}

	apiKeyCache[workspace] = apiKey
	isNew := true
	for _, ws := range loaded.Workspaces {
		if ws == workspace {
			isNew = false
			break
		}
	}
	if isNew {
		loaded.Workspaces = append(loaded.Workspaces, workspace)
		sort.Strings(loaded.Workspaces)
	}
	if isNew && len(loaded.Workspaces) == 1 {
		loaded.Default = workspace
	}

	if useInline {
		inlineFormat = true
		return saveInlineLocked()
	}

	if err := keyring.Default().Set(keyringService, workspace, apiKey); err != nil {
		return fmt.Errorf("store keyring for %q: %w", workspace, err)
	}
	inlineFormat = false
	return saveKeyringMetaLocked()
}

// RemoveCredential deletes a workspace credential.
func RemoveCredential(workspace string) error {
	mu.Lock()
	defer mu.Unlock()

	if !inlineFormat {
		_ = keyring.Default().Delete(keyringService, workspace)
	}
	delete(apiKeyCache, workspace)
	filtered := loaded.Workspaces[:0]
	for _, ws := range loaded.Workspaces {
		if ws != workspace {
			filtered = append(filtered, ws)
		}
	}
	loaded.Workspaces = filtered
	if loaded.Default == workspace {
		loaded.Default = ""
		if len(loaded.Workspaces) > 0 {
			loaded.Default = loaded.Workspaces[0]
		}
	}
	if inlineFormat {
		return saveInlineLocked()
	}
	return saveKeyringMetaLocked()
}

// SetDefaultWorkspace sets the default workspace slug.
func SetDefaultWorkspace(workspace string) error {
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ws := range loaded.Workspaces {
		if ws == workspace {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("Workspace %q not found in credentials", workspace)
	}
	loaded.Default = workspace
	if inlineFormat {
		return saveInlineLocked()
	}
	return saveKeyringMetaLocked()
}

func saveInlineLocked() error {
	path, err := credentialsPathLocked()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	ordered := map[string]string{}
	if loaded.Default != "" {
		ordered["default"] = loaded.Default
	}
	for _, ws := range loaded.Workspaces {
		key, ok := apiKeyCache[ws]
		if !ok {
			return fmt.Errorf("missing API key cache for workspace %q", ws)
		}
		ordered[ws] = key
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(ordered)
}

func saveKeyringMetaLocked() error {
	path, err := credentialsPathLocked()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	type meta struct {
		Default    string   `toml:"default,omitempty"`
		Workspaces []string `toml:"workspaces"`
	}
	m := meta{Default: loaded.Default, Workspaces: append([]string{}, loaded.Workspaces...)}
	sort.Strings(m.Workspaces)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(m)
}

func credentialsPathLocked() (string, error) {
	if pathOverride != "" {
		return pathOverride, nil
	}
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA is not set")
		}
		return filepath.Join(appData, "linear", "credentials.toml"), nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "linear", "credentials.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "linear", "credentials.toml"), nil
}

// MigrateToKeyring moves inline credentials into the keyring store and rewrites metadata.
func MigrateToKeyring() ([]string, error) {
	mu.Lock()
	defer mu.Unlock()
	if !inlineFormat {
		return nil, nil
	}
	migrated := make([]string, 0, len(loaded.Workspaces))
	for _, ws := range loaded.Workspaces {
		key, ok := apiKeyCache[ws]
		if !ok || key == "" {
			continue
		}
		if err := keyring.Default().Set(keyringService, ws, key); err != nil {
			return nil, fmt.Errorf("store keyring for %q: %w", ws, err)
		}
		migrated = append(migrated, ws)
	}
	inlineFormat = false
	if err := saveKeyringMetaLocked(); err != nil {
		return nil, err
	}
	return migrated, nil
}

// ResetForTest clears in-memory credential state (tests only).
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	apiKeyCache = map[string]string{}
	loaded = Credentials{}
	inlineFormat = false
	pathOverride = ""
}
