// Package planeconfig resolves Plane configuration without reading Linear state.
package planeconfig

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

const CloudURL = "https://api.plane.so"

type Profile struct {
	BaseURL    string `toml:"base_url" json:"base_url"`
	Workspace  string `toml:"workspace" json:"workspace"`
	APIVersion string `toml:"api_version" json:"api_version"`
	ProjectID  string `toml:"project_id,omitempty" json:"project_id,omitempty"`
	AllowHTTP  bool   `toml:"allow_http,omitempty" json:"allow_http,omitempty"`
}

type Context struct {
	Profile
	ProfileName string            `json:"profile,omitempty"`
	Sources     map[string]string `json:"sources,omitempty"`
}

type Options struct {
	Profile    string
	NoProfile  bool
	BaseURL    string
	Workspace  string
	APIVersion string
	ProjectID  string
	AllowHTTP  bool
}

type UserConfig struct {
	SchemaVersion int `toml:"schema_version"`
	Plane         struct {
		DefaultProfile string             `toml:"default_profile,omitempty"`
		Profiles       map[string]Profile `toml:"profiles"`
	} `toml:"plane"`
}

type projectConfig struct {
	SchemaVersion int `toml:"schema_version"`
	Plane         struct {
		Profile   string `toml:"profile"`
		Workspace string `toml:"workspace"`
		ProjectID string `toml:"project_id"`
	} `toml:"plane"`
}

type Resolver struct {
	UserFile   string
	ProjectDir string
	Getenv     func(string) string
}

func ConfigPath() (string, error) {
	if runtime.GOOS == "windows" {
		if p := os.Getenv("APPDATA"); p != "" {
			return filepath.Join(p, "workctl", "config.toml"), nil
		}
		return "", fmt.Errorf("APPDATA is not set")
	}
	if p := os.Getenv("XDG_CONFIG_HOME"); p != "" {
		return filepath.Join(p, "workctl", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "workctl", "config.toml"), nil
}

func (r Resolver) Path() (string, error) {
	if r.UserFile != "" {
		return r.UserFile, nil
	}
	return ConfigPath()
}
func (r Resolver) env(key string) string {
	if r.Getenv != nil {
		return r.Getenv(key)
	}
	return os.Getenv(key)
}
func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
func decodeFile(path string, value interface{}) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read config: %w", err)
	}
	meta, err := toml.Decode(string(data), value)
	// TOML errors can quote input; do not echo file contents (possibly secrets).
	if err != nil {
		return false, fmt.Errorf("invalid TOML configuration in %s", path)
	}
	if keys := meta.Undecoded(); len(keys) > 0 {
		return false, fmt.Errorf("unknown configuration key %s in %s", keys[0], path)
	}
	return true, nil
}
func (r Resolver) Load() (UserConfig, error) {
	cfg := UserConfig{SchemaVersion: 1}
	path, err := r.Path()
	if err != nil {
		return cfg, err
	}
	// Distinguish absent schema_version from an absent file.
	cfg.SchemaVersion = 0
	exists, err := decodeFile(path, &cfg)
	if err != nil {
		return cfg, err
	}
	if !exists {
		cfg.SchemaVersion = 1
	}
	if cfg.SchemaVersion != 1 {
		return cfg, fmt.Errorf("unsupported workctl schema_version (expected 1)")
	}
	if cfg.Plane.Profiles == nil {
		cfg.Plane.Profiles = map[string]Profile{}
	}
	return cfg, nil
}
func (r Resolver) project() (projectConfig, error) {
	var cfg projectConfig
	dir := r.ProjectDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return cfg, err
		}
	}
	for _, name := range []string{"workctl.toml", ".workctl.toml"} {
		exists, err := decodeFile(filepath.Join(dir, name), &cfg)
		if err != nil {
			return cfg, err
		}
		if exists {
			break
		}
	}
	if cfg.SchemaVersion != 0 && cfg.SchemaVersion != 1 {
		return cfg, fmt.Errorf("unsupported project schema_version")
	}
	return cfg, nil
}

// Normalize validates an identity before any credential is resolved.
func Normalize(p Profile) (Profile, error) {
	if p.BaseURL == "" {
		p.BaseURL = CloudURL
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return p, fmt.Errorf("base URL must be an absolute instance URL without credentials, query or fragment")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && p.AllowHTTP) {
		return p, fmt.Errorf("base URL requires HTTPS; use --allow-http explicitly for local HTTP")
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return p, fmt.Errorf("base URL cannot contain traversal segments")
		}
	}
	u.Host = strings.ToLower(u.Host)
	p.BaseURL = strings.TrimRight(u.String(), "/")
	if p.APIVersion == "" {
		if p.BaseURL != CloudURL {
			return p, fmt.Errorf("self-hosted Plane requires an explicit --api-version v1 or v2")
		}
		p.APIVersion = "v2"
	}
	if p.APIVersion != "v1" && p.APIVersion != "v2" {
		return p, fmt.Errorf("api version must be v1 or v2")
	}
	if p.Workspace == "" {
		return p, fmt.Errorf("workspace is required (use --workspace or PLANE_WORKSPACE)")
	}
	if strings.ContainsAny(p.Workspace, "/\\?#\x00\r\n") || p.Workspace == "." || p.Workspace == ".." {
		return p, fmt.Errorf("invalid workspace slug")
	}
	return p, nil
}

func (r Resolver) Resolve(o Options) (Context, error) {
	if o.NoProfile && o.Profile != "" {
		return Context{}, fmt.Errorf("--profile and --no-profile are mutually exclusive")
	}
	cfg, err := r.Load()
	if err != nil {
		return Context{}, err
	}
	project, err := r.project()
	if err != nil {
		return Context{}, err
	}
	name := first(o.Profile, r.env("PLANE_PROFILE"), project.Plane.Profile, cfg.Plane.DefaultProfile)
	if o.NoProfile {
		name = ""
	}
	overrides := Profile{BaseURL: first(o.BaseURL, r.env("PLANE_BASE_URL")), Workspace: first(o.Workspace, r.env("PLANE_WORKSPACE")), APIVersion: first(o.APIVersion, r.env("PLANE_API_VERSION")), AllowHTTP: o.AllowHTTP}
	var p Profile
	if name != "" {
		var exists bool
		p, exists = cfg.Plane.Profiles[name]
		if !exists {
			return Context{}, fmt.Errorf("Plane profile %q does not exist; run workctl plane auth login --profile %s", name, name)
		}
		p, err = Normalize(p)
		if err != nil {
			return Context{}, fmt.Errorf("invalid profile %q: %w", name, err)
		}
		if overrides.BaseURL != "" && strings.TrimRight(overrides.BaseURL, "/") != p.BaseURL || overrides.Workspace != "" && overrides.Workspace != p.Workspace || overrides.APIVersion != "" && overrides.APIVersion != p.APIVersion || project.Plane.Workspace != "" && project.Plane.Workspace != p.Workspace {
			return Context{}, fmt.Errorf("Plane context conflicts with profile %q; create another profile or use --no-profile", name)
		}
	} else {
		p, err = Normalize(overrides)
		if err != nil {
			return Context{}, err
		}
	}
	p.ProjectID = first(o.ProjectID, r.env("PLANE_PROJECT_ID"), project.Plane.ProjectID, p.ProjectID)
	sources := map[string]string{}
	if name != "" {
		for _, key := range []string{"base_url", "workspace", "api_version"} {
			sources[key] = "profile " + name
		}
	} else {
		for _, field := range []struct{ key, flag, env string }{{"base_url", o.BaseURL, "PLANE_BASE_URL"}, {"workspace", o.Workspace, "PLANE_WORKSPACE"}, {"api_version", o.APIVersion, "PLANE_API_VERSION"}} {
			source := "Cloud default"
			if field.flag != "" {
				source = "flag"
			} else if r.env(field.env) != "" {
				source = field.env
			}
			sources[field.key] = source
		}
	}
	sources["project_id"] = "unset"
	if o.ProjectID != "" {
		sources["project_id"] = "flag"
	} else if r.env("PLANE_PROJECT_ID") != "" {
		sources["project_id"] = "PLANE_PROJECT_ID"
	} else if project.Plane.ProjectID != "" {
		sources["project_id"] = "project config"
	} else if p.ProjectID != "" {
		sources["project_id"] = "profile " + name
	}
	return Context{Profile: p, ProfileName: name, Sources: sources}, nil
}

// LoginContext allows creation of a profile, but never mutates an existing identity.
func (r Resolver) LoginContext(o Options) (Context, error) {
	cfg, err := r.Load()
	if err != nil {
		return Context{}, err
	}
	if o.Profile != "" && !o.NoProfile {
		if _, exists := cfg.Plane.Profiles[o.Profile]; !exists {
			c, err := r.Resolve(Options{NoProfile: true, BaseURL: o.BaseURL, Workspace: o.Workspace, APIVersion: o.APIVersion, ProjectID: o.ProjectID, AllowHTTP: o.AllowHTTP})
			c.ProfileName = o.Profile
			return c, err
		}
	}
	return r.Resolve(o)
}

func (r Resolver) SaveProfile(name string, p Profile) error {
	if name == "" || strings.ContainsAny(name, "\x00\r\n") {
		return fmt.Errorf("profile name is required")
	}
	p, err := Normalize(p)
	if err != nil {
		return err
	}
	cfg, err := r.Load()
	if err != nil {
		return err
	}
	if old, ok := cfg.Plane.Profiles[name]; ok {
		old, err = Normalize(old)
		if err != nil {
			return err
		}
		if old.BaseURL != p.BaseURL || old.Workspace != p.Workspace || old.APIVersion != p.APIVersion {
			return fmt.Errorf("profile identity cannot change; choose a new profile name")
		}
	}
	cfg.Plane.Profiles[name] = p
	if cfg.Plane.DefaultProfile == "" {
		cfg.Plane.DefaultProfile = name
	}
	return r.save(cfg)
}
func (r Resolver) SetDefault(name string) error {
	cfg, err := r.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Plane.Profiles[name]; !ok {
		return fmt.Errorf("unknown Plane profile %q", name)
	}
	cfg.Plane.DefaultProfile = name
	return r.save(cfg)
}
func (r Resolver) save(cfg UserConfig) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	path, err := r.Path()
	if err != nil {
		return err
	}
	return WritePrivate(path, buf.Bytes())
}

// WritePrivate atomically replaces a private configuration/credential file.
func WritePrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".workctl-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
