package templates

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kongken/linear-cli/internal/errors"
	"github.com/kongken/linear-cli/internal/ids"
	"github.com/kongken/linear-cli/internal/graphql"
)

const getTemplatesQuery = `
query GetTemplates {
  templates {
    id name description type
    team { id key name }
  }
}
`

const getTemplateQuery = `
query GetTemplate($id: String!) {
  template(id: $id) {
    id name description type
    team { id key name }
  }
}
`

// Template is a Linear template summary.
type Template struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Team *struct {
		ID   string `json:"id"`
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"team"`
}

// Scope limits which templates qualify for resolve.
type Scope struct {
	Type    string
	TeamIDs []string
}

func availableTo(t Template, teamIDs []string) bool {
	if t.Team == nil {
		return true
	}
	for _, id := range teamIDs {
		if t.Team.ID == id {
			return true
		}
	}
	return false
}

func inScope(t Template, scope *Scope) bool {
	if scope == nil {
		return true
	}
	return strings.EqualFold(t.Type, scope.Type) && availableTo(t, scope.TeamIDs)
}

// Resolve finds a template by UUID or exact case-insensitive name.
func Resolve(ctx context.Context, reference string, scope *Scope) (*Template, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, errors.NewValidationError("template reference is empty")
	}
	client, err := graphql.NewClient()
	if err != nil {
		return nil, err
	}

	if ids.IsUUID(reference) {
		data, err := client.RequestRaw(ctx, getTemplateQuery, map[string]any{"id": reference})
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "no template found") {
				return nil, errors.NewNotFoundError("Template", reference,
					errors.WithSuggestion("Run `linear template list` to see every template."))
			}
			return nil, err
		}
		var parsed struct {
			Template *Template `json:"template"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		if parsed.Template == nil {
			return nil, errors.NewNotFoundError("Template", reference)
		}
		if scope != nil && !inScope(*parsed.Template, scope) {
			return nil, scopeMismatchError([]Template{*parsed.Template}, *scope)
		}
		return parsed.Template, nil
	}

	data, err := client.RequestRaw(ctx, getTemplatesQuery, nil)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Templates []Template `json:"templates"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	wanted := strings.ToLower(reference)
	var byName []Template
	for _, t := range parsed.Templates {
		if strings.ToLower(t.Name) == wanted {
			byName = append(byName, t)
		}
	}
	var candidates []Template
	for _, t := range byName {
		if inScope(t, scope) {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 1 {
		return &candidates[0], nil
	}
	if len(candidates) == 0 {
		if len(byName) > 0 && scope != nil {
			return nil, scopeMismatchError(byName, *scope)
		}
		what := "templates"
		if scope != nil {
			what = scope.Type + " templates"
		}
		names := uniqueSortedNames(parsed.Templates, func(t Template) bool { return inScope(t, scope) })
		suggestion := fmt.Sprintf("No %s are available here. Run `linear template list` to see every template.", what)
		if len(names) > 0 {
			quoted := make([]string, len(names))
			for i, n := range names {
				quoted[i] = `"` + n + `"`
			}
			suggestion = fmt.Sprintf("Available %s: %s. Run `linear template list` to see every template.", what, strings.Join(quoted, ", "))
		}
		return nil, errors.NewNotFoundError("Template", reference, errors.WithSuggestion(suggestion))
	}
	parts := make([]string, 0, len(candidates))
	for _, t := range candidates {
		label := "Workspace"
		if t.Team != nil {
			label = t.Team.Key
		}
		parts = append(parts, fmt.Sprintf("%s (%s, %s)", t.ID, t.Type, label))
	}
	return nil, errors.NewValidationError(
		fmt.Sprintf("Template name %q is ambiguous: it matches %d templates", reference, len(candidates)),
		errors.WithSuggestion("Pass the template ID instead: "+strings.Join(parts, ", ")),
	)
}

func scopeMismatchError(matches []Template, scope Scope) error {
	var sameType []Template
	for _, t := range matches {
		if strings.EqualFold(t.Type, scope.Type) {
			sameType = append(sameType, t)
		}
	}
	if len(sameType) > 0 {
		keys := map[string]struct{}{}
		for _, t := range sameType {
			if t.Team != nil {
				keys[t.Team.Key] = struct{}{}
			}
		}
		if len(keys) > 0 {
			list := make([]string, 0, len(keys))
			for k := range keys {
				list = append(list, k)
			}
			sort.Strings(list)
			return errors.NewValidationError(
				fmt.Sprintf("Template %q belongs to team %s and cannot be applied here", sameType[0].Name, strings.Join(list, ", ")),
				errors.WithSuggestion(fmt.Sprintf("Pass --team %s, or pick a workspace template or one from the target team.", list[0])),
			)
		}
	}
	return errors.NewValidationError(
		fmt.Sprintf("Template %q is a %s template, not a %s template", matches[0].Name, matches[0].Type, scope.Type),
		errors.WithSuggestion(fmt.Sprintf("Run `linear template list --type %s` to see the %s templates.", scope.Type, scope.Type)),
	)
}

func uniqueSortedNames(all []Template, keep func(Template) bool) []string {
	seen := map[string]struct{}{}
	var names []string
	for _, t := range all {
		if !keep(t) {
			continue
		}
		if _, ok := seen[t.Name]; ok {
			continue
		}
		seen[t.Name] = struct{}{}
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names
}
