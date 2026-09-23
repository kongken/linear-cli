package ids

import "regexp"

var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// IsUUID reports whether value looks like a Linear UUID.
func IsUUID(value string) bool {
	return uuidRe.MatchString(value)
}
