package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dpage/slonik-ears/internal/protocol"
)

// handleTranscript serves the whole retained transcript for a room in a
// choice of formats, so organisers can hand it to the speaker afterwards.
func (s *Server) handleTranscript(w http.ResponseWriter, r *http.Request) {
	if !s.authoriseViewer(r) {
		writeError(w, http.StatusUnauthorized, "a passcode is required")
		return
	}
	id := r.PathValue("id")
	room, ok := s.hub.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "no such room")
		return
	}
	segs := room.All()
	info := room.Info()

	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "txt"
	}
	filename := id + "-" + time.Now().Format("2006-01-02")

	switch format {
	case "json":
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.json"`)
		writeJSON(w, http.StatusOK, map[string]any{"room": info, "segments": segs})
	case "txt", "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.txt"`)
		writeText(w, info, segs, r.URL.Query().Get("timestamps") != "0")
	case "srt":
		w.Header().Set("Content-Type", "application/x-subrip; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.srt"`)
		writeSRT(w, segs)
	case "vtt":
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.vtt"`)
		writeVTT(w, segs)
	default:
		writeError(w, http.StatusBadRequest, "unknown format: use txt, json, srt or vtt")
	}
}

func writeText(w http.ResponseWriter, info protocol.Room, segs []protocol.Segment, timestamps bool) {
	title := info.Title
	if title == "" {
		title = info.ID
	}
	fmt.Fprintf(w, "%s\n", title)
	if info.Speaker != "" {
		fmt.Fprintf(w, "Speaker: %s\n", info.Speaker)
	}
	if info.Track != "" {
		fmt.Fprintf(w, "Track: %s\n", info.Track)
	}
	if info.StartedAt > 0 {
		fmt.Fprintf(w, "Started: %s\n", time.UnixMilli(info.StartedAt).UTC().Format(time.RFC1123))
	}
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("-", 60))
	for _, seg := range segs {
		if timestamps {
			fmt.Fprintf(w, "[%s] %s\n", clock(seg.StartMs), seg.Text)
		} else {
			fmt.Fprintf(w, "%s\n", seg.Text)
		}
	}
}

func writeSRT(w http.ResponseWriter, segs []protocol.Segment) {
	for i, seg := range segs {
		fmt.Fprintf(w, "%d\n%s --> %s\n%s\n\n", i+1,
			srtTime(seg.StartMs), srtTime(maxMs(seg.EndMs, seg.StartMs+1000)), seg.Text)
	}
}

func writeVTT(w http.ResponseWriter, segs []protocol.Segment) {
	fmt.Fprint(w, "WEBVTT\n\n")
	for _, seg := range segs {
		fmt.Fprintf(w, "%s --> %s\n%s\n\n",
			vttTime(seg.StartMs), vttTime(maxMs(seg.EndMs, seg.StartMs+1000)), seg.Text)
	}
}

func maxMs(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func clock(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func srtTime(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d,%03d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60, ms%1000)
}

func vttTime(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d.%03d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60, ms%1000)
}
