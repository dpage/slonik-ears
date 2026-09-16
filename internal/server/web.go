package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/dpage/slonik-ears/web"
)

const placeholderPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Slonik Ears</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
 body{font:16px/1.6 system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1.5rem;color:#111}
 code{background:#f2f2f2;padding:.15em .4em;border-radius:4px}
 @media (prefers-color-scheme:dark){body{background:#111;color:#eee}code{background:#222}}
</style></head><body>
<h1>Slonik Ears</h1>
<p>The server is running, but the web app has not been built into this binary.</p>
<p>Build it with <code>make web</code> (or <code>npm --prefix web ci &amp;&amp; npm --prefix web run build</code>)
and then rebuild the server with <code>make server</code>.</p>
<p>The API is live regardless: try <code>/api/rooms</code>.</p>
</body></html>`

// webHandler serves the embedded single page app, falling back to index.html
// for client-side routes such as /r/main-hall.
func (s *Server) webHandler() http.Handler {
	assets, built := web.FS()
	if !built {
		s.log.Warn("web app not embedded; serving placeholder page (run `make web`)")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/r/") {
				w.WriteHeader(http.StatusNotFound)
			}
			_, _ = w.Write([]byte(placeholderPage))
		})
	}

	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			serveIndex(w, r, assets)
			return
		}
		f, err := assets.Open(name)
		if err != nil {
			// Unknown path: hand it to the SPA router rather than 404ing, so
			// deep links like /r/main-hall/stage work on a refresh.
			serveIndex(w, r, assets)
			return
		}
		st, err := f.Stat()
		_ = f.Close()
		if err != nil || st.IsDir() {
			serveIndex(w, r, assets)
			return
		}
		// Vite emits content-hashed asset names, so they can be cached hard.
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.Error(w, "index.html missing from build", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}
