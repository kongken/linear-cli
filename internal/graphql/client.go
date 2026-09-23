package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	gqlclient "github.com/Khan/genqlient/graphql"
	linearconst "github.com/kongken/linear-cli/internal/const"
	"github.com/kongken/linear-cli/internal/config"
	"github.com/kongken/linear-cli/internal/credentials"
	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/version"
)

func mustVersion() string {
	return version.Version
}

// Client posts GraphQL requests to Linear.
type Client struct {
	Endpoint string
	APIKey   string
	HTTP     *http.Client
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// Endpoint returns LINEAR_GRAPHQL_ENDPOINT or the default Linear API URL.
func Endpoint() string {
	if ep := os.Getenv("LINEAR_GRAPHQL_ENDPOINT"); ep != "" {
		return ep
	}
	return linearconst.APIEndpoint
}

// ResolvedAPIKey resolves the API key from env, workspace config, and credentials.
func ResolvedAPIKey() (string, error) {
	cliWorkspace := config.CLIWorkspace()
	envAPIKey := os.Getenv("LINEAR_API_KEY")

	if envAPIKey != "" && cliWorkspace != "" {
		return "", errors.NewValidationError(
			"Cannot use --workspace flag when LINEAR_API_KEY environment variable is set.",
			errors.WithSuggestion("Either unset LINEAR_API_KEY or remove the --workspace flag."),
		)
	}

	if envAPIKey != "" {
		return envAPIKey, nil
	}

	if configKey, ok := config.GetOption("api_key"); ok {
		return configKey, nil
	}

	if cliWorkspace != "" {
		if key, ok := credentials.GetAPIKey(cliWorkspace); ok {
			return key, nil
		}
		return "", errors.NewValidationError(
			fmt.Sprintf("Workspace %q not found in credentials.", cliWorkspace),
			errors.WithSuggestion("Run `linear auth login` to add it, or `linear auth list` to see configured workspaces."),
		)
	}

	if projectWorkspace, ok := config.GetOption("workspace"); ok {
		if key, ok := credentials.GetAPIKey(projectWorkspace); ok {
			return key, nil
		}
	}

	// 5: default workspace from credentials file
	if key, ok := credentials.GetAPIKey(""); ok {
		return key, nil
	}

	return "", errors.NewAuthError(
		"No API key configured. Set LINEAR_API_KEY, add api_key to .linear.toml, or run `linear auth login`.",
	)
}

// NewClient builds a GraphQL client using resolved credentials.
func NewClient() (*Client, error) {
	key, err := ResolvedAPIKey()
	if err != nil {
		return nil, err
	}
	return NewClientWithAPIKey(key), nil
}

// NewClientWithAPIKey builds a client for an explicit API key (e.g. auth login).
func NewClientWithAPIKey(apiKey string) *Client {
	return &Client{
		Endpoint: Endpoint(),
		APIKey:   apiKey,
		HTTP:     &http.Client{Timeout: 60 * time.Second},
	}
}

// Request executes a GraphQL query and decodes data into result.
func (c *Client) Request(ctx context.Context, query string, variables map[string]any, result any) error {
	body, err := json.Marshal(gqlRequest{Query: query, Variables: variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.APIKey)
	req.Header.Set("User-Agent", "kongken-linear-cli/"+mustVersion())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var parsed gqlResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("decode graphql response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		msg := parsed.Errors[0].Message
		if ext := parsed.Errors[0].Extensions; ext != nil {
			if upm, ok := ext["userPresentableMessage"].(string); ok && upm != "" {
				msg = upm
			}
		}
		return errors.NewCliError(msg)
	}
	if result == nil || len(parsed.Data) == 0 || string(parsed.Data) == "null" {
		return nil
	}
	return json.Unmarshal(parsed.Data, result)
}

// RequestRaw executes a GraphQL query and returns the raw `data` JSON object.
func (c *Client) RequestRaw(ctx context.Context, query string, variables map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(gqlRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.APIKey)
	req.Header.Set("User-Agent", "kongken-linear-cli/"+mustVersion())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed gqlResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode graphql response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		msg := parsed.Errors[0].Message
		if ext := parsed.Errors[0].Extensions; ext != nil {
			if upm, ok := ext["userPresentableMessage"].(string); ok && upm != "" {
				msg = upm
			}
		}
		return nil, errors.NewCliError(msg)
	}
	return parsed.Data, nil
}

// MakeRequest implements github.com/Khan/genqlient/graphql.Client so generated
// operations can share this HTTP client and credential resolution.
func (c *Client) MakeRequest(ctx context.Context, req *gqlclient.Request, resp *gqlclient.Response) error {
	if req == nil || resp == nil {
		return fmt.Errorf("graphql: nil request or response")
	}
	vars, _ := req.Variables.(map[string]any)
	if req.Variables != nil && vars == nil {
		// genqlient may pass a typed variables struct; re-marshal into map.
		b, err := json.Marshal(req.Variables)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &vars); err != nil {
			return err
		}
	}
	if err := c.Request(ctx, req.Query, vars, resp.Data); err != nil {
		return err
	}
	return nil
}
