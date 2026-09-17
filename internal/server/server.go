// Package server implements the Slonik Ears relay: it accepts transcript
// streams from room listeners over WebSockets, fans them out to viewers, and
// serves the attendee web app.
package server

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dpage/slonik-ears/internal/hub"
	"github.com/dpage/slonik-ears/internal/protocol"
	"github.com/dpage/slonik-ears/internal/store"
	"github.com/dpage/slonik-ears/internal/version"
)

// Server ties the hub, the store and the HTTP surface together.
type Server struct {
	cfg   Config
	log   *slog.Logger
	hub   *hub.Hub
	store store.Store
	// roomTokens holds per-room publish token overrides.
	roomTokens map[string]string
	started    time.Time
}

// New builds a server. The caller owns the store's lifetime.
func New(cfg Config, st store.Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	if st == nil {
		st = store.Null{}
	}
	s := &Server{
		cfg:        cfg,
		log:        log,
		store:      st,
		roomTokens: make(map[string]string),
		started:    time.Now(),
	}
	s.hub = hub.New(hub.Options{History: cfg.Server.History, Sink: sinkFunc(st.Append)})

	// Pre-declare configured rooms so the lobby is populated before any
	// listener connects, and restore anything we already have on disk.
	for _, rc := range cfg.Rooms {
		r := s.hub.Ensure(rc.ID)
		r.SetMetadata(protocol.Room{
			Title:    rc.Title,
			Track:    rc.Track,
			Speaker:  rc.Speaker,
			Language: rc.Language,
		})
		if rc.PublishToken != "" {
			s.roomTokens[rc.ID] = rc.PublishToken
		}
	}
	s.restore()
	return s
}

type sinkFunc func(roomID string, seg protocol.Segment)

func (f sinkFunc) Append(roomID string, seg protocol.Segment) { f(roomID, seg) }

// restore reloads persisted transcripts into the hub so a restarted server
// still has the morning's talks.
func (s *Server) restore() {
	ids, err := s.store.Rooms()
	if err != nil {
		s.log.Warn("could not list stored transcripts", "error", err)
		return
	}
	for _, id := range ids {
		segs, err := s.store.Load(id)
		if err != nil {
			s.log.Warn("could not load transcript", "room", id, "error", err)
			continue
		}
		if len(segs) == 0 {
			continue
		}
		s.hub.Ensure(id).Restore(segs)
		s.log.Info("restored transcript", "room", id, "segments", len(segs))
	}
}

// Hub exposes the hub, mainly for tests.
func (s *Server) Hub() *hub.Hub { return s.hub }

// Handler returns the fully wired HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("POST /api/session", s.handleSession)
	mux.HandleFunc("GET /api/rooms", s.handleRooms)
	mux.HandleFunc("GET /api/rooms/{id}", s.handleRoom)
	mux.HandleFunc("GET /api/rooms/{id}/transcript", s.handleTranscript)
	mux.HandleFunc("GET /api/rooms/{id}/qr.png", s.handleQR)
	mux.HandleFunc("GET /api/watch", s.handleWatch)
	mux.HandleFunc("GET /api/lobby", s.handleLobby)
	mux.HandleFunc("GET /api/publish", s.handlePublish)
	mux.HandleFunc("POST /api/admin/session", s.handleAdminSession)
	mux.HandleFunc("GET /api/admin/rooms", s.handleAdminRooms)
	mux.HandleFunc("PUT /api/admin/rooms/{id}", s.handleAdminUpsertRoom)
	mux.HandleFunc("POST /api/admin/rooms/{id}/reset", s.handleAdminResetRoom)
	mux.HandleFunc("DELETE /api/admin/rooms/{id}", s.handleAdminDeleteRoom)

	mux.Handle("/", s.webHandler())

	return s.withCommonHeaders(mux)
}

func (s *Server) withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			// The API is read-mostly and consumed by the bundled SPA. Allowing
			// cross-origin GETs means a venue can embed the transcript in its
			// own site without a proxy.
			h.Set("Access-Control-Allow-Origin", s.corsOrigin(r))
			h.Set("Vary", "Origin")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) corsOrigin(r *http.Request) string {
	origin := r.Header.Get("Origin")
	if len(s.cfg.Auth.AllowedOrigins) == 0 {
		if origin != "" {
			return origin
		}
		return "*"
	}
	for _, a := range s.cfg.Auth.AllowedOrigins {
		if a == "*" || strings.EqualFold(a, origin) {
			return origin
		}
	}
	return ""
}

// ---------------------------------------------------------------- auth

const viewerCookie = "ears_key"

// adminCookie carries the admin token for the /admin page. A browser cannot
// put an Authorization header on a plain navigation, and asking somebody to
// paste a token between every talk is not a serious proposition, so the page
// trades the token for a cookie exactly as attendees do with the passcode.
const adminCookie = "ears_admin"

// authoriseViewer reports whether the request may watch. When no passcode is
// configured, everyone may.
func (s *Server) authoriseViewer(r *http.Request) bool {
	want := s.cfg.Auth.ViewerPasscode
	if want == "" {
		return true
	}
	candidates := []string{
		r.URL.Query().Get("k"),
		r.Header.Get("X-Ears-Key"),
	}
	if c, err := r.Cookie(viewerCookie); err == nil {
		candidates = append(candidates, c.Value)
	}
	for _, got := range candidates {
		if got != "" && secretEqual(got, want) {
			return true
		}
	}
	return false
}

// authorisePublisher checks the publish token for a room. A per-room token, if
// configured, is accepted in addition to the global one.
func (s *Server) authorisePublisher(r *http.Request, roomID string) bool {
	global := s.cfg.Auth.PublishToken
	perRoom := s.roomTokens[roomID]
	if global == "" && perRoom == "" {
		// No token configured at all: refuse rather than silently running an
		// open relay that anyone on the venue Wi-Fi can scribble on.
		return false
	}
	got := bearerToken(r)
	if got == "" {
		got = r.URL.Query().Get("token")
	}
	if got == "" {
		return false
	}
	if global != "" && secretEqual(got, global) {
		return true
	}
	if perRoom != "" && secretEqual(got, perRoom) {
		return true
	}
	return false
}

// authoriseAdmin reports whether the request may change rooms. An unset admin
// token means the admin API is off entirely rather than open to all: this is
// the interface that can wipe a talk off the screen.
func (s *Server) authoriseAdmin(r *http.Request) bool {
	want := s.cfg.Auth.AdminToken
	if want == "" {
		return false
	}
	if got := bearerToken(r); got != "" && secretEqual(got, want) {
		return true
	}
	if c, err := r.Cookie(adminCookie); err == nil && secretEqual(c.Value, want) {
		return true
	}
	return false
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func secretEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// ---------------------------------------------------------------- handlers

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": version.String(),
		"uptime":  time.Since(s.started).Round(time.Second).String(),
		"rooms":   len(s.hub.Rooms()),
	})
}

// publicConfig is what the web app needs to render itself.
type publicConfig struct {
	Event         EventConfig `json:"event"`
	Version       string      `json:"version"`
	RequiresKey   bool        `json:"requiresKey"`
	Authenticated bool        `json:"authenticated"`
	BaseURL       string      `json:"baseUrl,omitempty"`
	ProtocolVer   int         `json:"protocolVersion"`
	// AdminEnabled says whether this server has an admin token set at all, so
	// /admin can explain itself rather than rejecting a token that was never
	// going to work. AdminAuthed saves the page having to provoke a 401 to
	// find out whether it is still signed in.
	AdminEnabled bool `json:"adminEnabled"`
	AdminAuthed  bool `json:"adminAuthenticated"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, publicConfig{
		Event:         s.cfg.Event,
		Version:       version.String(),
		RequiresKey:   s.cfg.Auth.ViewerPasscode != "",
		Authenticated: s.authoriseViewer(r),
		BaseURL:       s.cfg.Server.BaseURL,
		ProtocolVer:   protocol.Version,
		AdminEnabled:  s.cfg.Auth.AdminToken != "",
		AdminAuthed:   s.authoriseAdmin(r),
	})
}

// handleSession exchanges a passcode for a cookie, so attendees only type it
// once even if they reload or open a different room.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if s.cfg.Auth.ViewerPasscode == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if !secretEqual(body.Key, s.cfg.Auth.ViewerPasscode) {
		writeError(w, http.StatusUnauthorized, "that passcode is not right")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     viewerCookie,
		Value:    body.Key,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		MaxAge:   int((12 * time.Hour).Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	if !s.authoriseViewer(r) {
		writeError(w, http.StatusUnauthorized, "a passcode is required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": s.hub.Rooms()})
}

func (s *Server) handleRoom(w http.ResponseWriter, r *http.Request) {
	if !s.authoriseViewer(r) {
		writeError(w, http.StatusUnauthorized, "a passcode is required")
		return
	}
	room, ok := s.hub.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such room")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"room": room.Info(), "status": room.Status()})
}

func (s *Server) handleAdminUpsertRoom(w http.ResponseWriter, r *http.Request) {
	if !s.authoriseAdmin(r) {
		writeError(w, http.StatusUnauthorized, "admin token required")
		return
	}
	id := r.PathValue("id")
	if !protocol.ValidRoomID(id) {
		writeError(w, http.StatusBadRequest, "invalid room id")
		return
	}
	var meta protocol.Room
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&meta); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	room := s.hub.Ensure(id)
	room.SetOperatorMetadata(meta)
	writeJSON(w, http.StatusOK, map[string]any{"room": room.Info()})
}

// handleAdminRooms lists rooms for the admin page. It exists alongside
// GET /api/rooms so the page has one request that answers both "what is
// there" and "am I still signed in": the viewer list is behind the attendee
// passcode, which is a different question and often not set at all.
func (s *Server) handleAdminRooms(w http.ResponseWriter, r *http.Request) {
	if !s.authoriseAdmin(r) {
		writeError(w, http.StatusUnauthorized, "admin token required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": s.hub.Rooms()})
}

// handleAdminSession trades the admin token for a cookie, so the /admin page
// is usable from a browser without pasting a bearer token into every request.
func (s *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if s.cfg.Auth.AdminToken == "" {
		writeError(w, http.StatusForbidden, "no admin token is configured on this server")
		return
	}
	if !secretEqual(body.Token, s.cfg.Auth.AdminToken) {
		writeError(w, http.StatusUnauthorized, "that token is not right")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookie,
		Value:    body.Token,
		Path:     "/",
		HttpOnly: true,
		// Strict rather than Lax: nothing should be able to reset a room on
		// the strength of a link somebody followed from elsewhere.
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
		MaxAge:   int((12 * time.Hour).Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminResetRoom empties a room for the next talk, setting the previous
// transcript aside rather than destroying it.
func (s *Server) handleAdminResetRoom(w http.ResponseWriter, r *http.Request) {
	if !s.authoriseAdmin(r) {
		writeError(w, http.StatusUnauthorized, "admin token required")
		return
	}
	id := r.PathValue("id")
	room, ok := s.hub.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "no such room")
		return
	}
	// Archive first. If setting the old transcript aside fails there is no
	// safe way to continue, because resetting the room would leave the next
	// talk appending to the last one's file and the two would be merged back
	// together at the next restart.
	if err := s.store.Archive(id); err != nil {
		s.log.Error("could not archive the transcript; the room has been left alone", "room", id, "error", err)
		writeError(w, http.StatusInternalServerError, "could not archive the existing transcript")
		return
	}
	room.Reset()
	s.log.Info("room reset for the next talk", "room", id)
	writeJSON(w, http.StatusOK, map[string]any{"room": room.Info()})
}

func (s *Server) handleAdminDeleteRoom(w http.ResponseWriter, r *http.Request) {
	if !s.authoriseAdmin(r) {
		writeError(w, http.StatusUnauthorized, "admin token required")
		return
	}
	id := r.PathValue("id")
	if !s.hub.Remove(id) {
		writeError(w, http.StatusNotFound, "no such room")
		return
	}
	// The stored transcript has to go too, or the room reappears at the next
	// restart with the talk still in it, which is exactly what deleting it was
	// meant to prevent. Archived rather than deleted, so the transcript itself
	// survives for whoever gave the talk.
	if err := s.store.Archive(id); err != nil {
		s.log.Error("room removed, but its transcript could not be archived", "room", id, "error", err)
	}
	s.log.Info("room removed", "room", id)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- helpers

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.Server.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i > 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func queryInt64(r *http.Request, key string) int64 {
	v, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
