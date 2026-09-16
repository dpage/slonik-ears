// Package version carries build metadata stamped in by the linker.
package version

import "runtime/debug"

// Version is set with -ldflags "-X .../internal/version.Version=v1.2.3".
var Version = ""

// Commit is set the same way; it falls back to the VCS stamp Go embeds.
var Commit = ""

// String returns a human readable build identifier.
func String() string {
	v, c := Version, Commit
	if v == "" {
		v = "dev"
	}
	if c == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 7 {
					c = s.Value[:7]
				}
			}
		}
	}
	if c == "" {
		return v
	}
	return v + " (" + c + ")"
}
