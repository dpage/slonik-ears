package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/dpage/slonik-ears/internal/protocol"
	qrcode "github.com/skip2/go-qrcode"
)

// handleQR renders a QR code pointing at a room's viewer page. The stage
// display shows it so attendees can follow along on their own phone without
// anybody having to read a URL out loud.
func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !protocol.ValidRoomID(id) {
		writeError(w, http.StatusBadRequest, "invalid room id")
		return
	}
	size := 512
	if v, err := strconv.Atoi(r.URL.Query().Get("size")); err == nil && v >= 64 && v <= 2048 {
		size = v
	}

	png, err := qrcode.Encode(s.roomURL(r, id), qrcode.Medium, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not render QR code")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(png)
}

// roomURL builds the attendee-facing link for a room, preferring the
// configured base URL and falling back to the request's own host.
func (s *Server) roomURL(r *http.Request, id string) string {
	base := strings.TrimRight(s.cfg.Server.BaseURL, "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + "/r/" + id
}
