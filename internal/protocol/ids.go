package protocol

import (
	"regexp"
	"strings"
)

var roomIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidRoomID reports whether id is safe to use as a room identifier. Room ids
// end up in URLs and (optionally) on disk as filenames, so they are kept to a
// deliberately dull character set.
func ValidRoomID(id string) bool { return roomIDRe.MatchString(id) }

// NormaliseRoomID lowercases and replaces runs of unsuitable characters with a
// hyphen, so "Track A — Main Hall" becomes "track-a-main-hall".
func NormaliseRoomID(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '_':
			b.WriteRune('_')
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-")
	}
	return out
}
