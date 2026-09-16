// Package web embeds the built attendee web app so that a single binary can
// be copied to a server with nothing else alongside it.
//
// The dist directory is produced by `npm run build` (see the Makefile). A
// placeholder is committed so that `go build` works in a checkout where the
// front end has not been built yet; the server then serves a short page
// explaining what to do about it.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built front end rooted at dist, and whether a real build is
// present.
func FS() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return sub, false
	}
	return sub, true
}
