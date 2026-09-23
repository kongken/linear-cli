package issueid

import (
	"regexp"
	"strings"
)

var (
	linearIdentifierRe       = regexp.MustCompile(`^([a-zA-Z0-9]+)-([1-9][0-9]*)$`)
	linearIdentifierInTextRe = regexp.MustCompile(`\b([a-zA-Z0-9]+)-([1-9][0-9]*)\b`)
)

// ParsedIssueIdentifier is a normalized Linear issue id.
type ParsedIssueIdentifier struct {
	Identifier  string
	TeamKey     string
	IssueNumber string
}

func build(teamKey, issueNumber string) ParsedIssueIdentifier {
	normalized := strings.ToUpper(teamKey)
	return ParsedIssueIdentifier{
		Identifier:  normalized + "-" + issueNumber,
		TeamKey:     normalized,
		IssueNumber: issueNumber,
	}
}

// ParseIssueIdentifier parses a full identifier like ENG-123.
func ParseIssueIdentifier(value string) (ParsedIssueIdentifier, bool) {
	m := linearIdentifierRe.FindStringSubmatch(value)
	if len(m) != 3 {
		return ParsedIssueIdentifier{}, false
	}
	return build(m[1], m[2]), true
}

// FindIssueIdentifierInText finds the first issue id in free text (e.g. a branch name).
func FindIssueIdentifierInText(value string) (ParsedIssueIdentifier, bool) {
	m := linearIdentifierInTextRe.FindStringSubmatch(value)
	if len(m) != 3 {
		return ParsedIssueIdentifier{}, false
	}
	return build(m[1], m[2]), true
}

// NormalizeIssueIdentifier returns TEAM-123 or empty if invalid.
func NormalizeIssueIdentifier(value string) (string, bool) {
	p, ok := ParseIssueIdentifier(value)
	if !ok {
		return "", false
	}
	return p.Identifier, true
}

// GetTeamKeyFromIssueIdentifier returns the team key portion, or empty.
func GetTeamKeyFromIssueIdentifier(value string) string {
	p, ok := ParseIssueIdentifier(value)
	if !ok {
		return ""
	}
	return p.TeamKey
}
