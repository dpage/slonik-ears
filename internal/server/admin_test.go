package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dpage/slonik-ears/internal/protocol"
	"github.com/dpage/slonik-ears/internal/server"
	"github.com/dpage/slonik-ears/internal/store"
)

// adminServer is the test harness with a real transcript directory behind it,
// because the point of most of these tests is what happens on disk.
func adminServer(t *testing.T) (*httptest.Server, string, store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewFile(dir, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := server.DefaultConfig()
	cfg.Auth.PublishToken = "publish-secret"
	cfg.Auth.AdminToken = "admin-secret"
	cfg.Rooms = []server.RoomConfig{{ID: "main-hall", Title: "Main Hall"}}

	srv := server.New(cfg, st, discardLogger())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, dir, st
}

func do(t *testing.T, ts *httptest.Server, method, path, token string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, ts.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func publishFinal(t *testing.T, conn *websocket.Conn, text string) {
	t.Helper()
	if err := conn.WriteJSON(protocol.Message{
		Type:    protocol.TypeFinal,
		Segment: &protocol.Segment{Text: text},
	}); err != nil {
		t.Fatal(err)
	}
}

// waitForCursor blocks until the room has caught up, since publishing is
// asynchronous and the assertions that follow are about what was stored.
func waitForCursor(t *testing.T, ts *httptest.Server, room string, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(ts.URL + "/api/rooms/" + room)
		if err == nil {
			var got struct {
				Room protocol.Room `json:"room"`
			}
			err = json.NewDecoder(resp.Body).Decode(&got)
			_ = resp.Body.Close()
			if err == nil && got.Room.Cursor >= want {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("room %s never reached cursor %d", room, want)
}

func transcriptText(t *testing.T, ts *httptest.Server, room string) string {
	t.Helper()
	resp, err := http.Get(ts.URL + "/api/rooms/" + room + "/transcript?format=txt")
	if err != nil {
		t.Fatal(err)
	}
	return readAll(t, resp)
}

func newJar(t *testing.T) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return jar
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

func TestAdminEndpointsRefuseTheWrongToken(t *testing.T) {
	ts, _, _ := adminServer(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/rooms"},
		{http.MethodPut, "/api/admin/rooms/main-hall"},
		{http.MethodPost, "/api/admin/rooms/main-hall/reset"},
		{http.MethodDelete, "/api/admin/rooms/main-hall"},
	} {
		for _, token := range []string{"", "not-the-token"} {
			resp := do(t, ts, tc.method, tc.path, token, map[string]string{})
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s %s with token %q: got %d, want 401", tc.method, tc.path, token, resp.StatusCode)
			}
		}
	}
}

func TestResetEmptiesTheRoomAndArchivesTheTranscript(t *testing.T) {
	ts, dir, _ := adminServer(t)

	// A talk happens.
	conn := dialPublisher(t, ts, "main-hall", "publish-secret")
	publishFinal(t, conn, "the first talk about replication")
	_ = conn.Close()
	waitForCursor(t, ts, "main-hall", 1)

	resp := do(t, ts, http.MethodPost, "/api/admin/rooms/main-hall/reset", "admin-secret", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset: got %d, want 200", resp.StatusCode)
	}

	// The room is empty and numbering starts again.
	var got struct {
		Room protocol.Room `json:"room"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Room.Cursor != 0 {
		t.Errorf("cursor after reset is %d, want 0", got.Room.Cursor)
	}
	if body := transcriptText(t, ts, "main-hall"); contains(body, "the first talk") {
		t.Errorf("the previous talk is still in the room:\n%s", body)
	}

	// But it is still on disk, which is the difference between resetting a
	// room and losing somebody's session.
	archives, _ := filepath.Glob(filepath.Join(dir, store.ArchiveDir, "main-hall-*.jsonl"))
	if len(archives) != 1 {
		t.Fatalf("expected the transcript to be archived, found %v", archives)
	}
}

func TestTheNextTalkDoesNotInheritTheLastOne(t *testing.T) {
	// The bug this whole endpoint exists for: before archiving, a reset room
	// carried on appending to the same file, and both talks came back merged
	// at the next restart.
	ts, _, st := adminServer(t)

	conn := dialPublisher(t, ts, "main-hall", "publish-secret")
	publishFinal(t, conn, "the first talk")
	_ = conn.Close()
	waitForCursor(t, ts, "main-hall", 1)

	if resp := do(t, ts, http.MethodPost, "/api/admin/rooms/main-hall/reset", "admin-secret", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("reset: got %d", resp.StatusCode)
	}

	conn2 := dialPublisher(t, ts, "main-hall", "publish-secret")
	publishFinal(t, conn2, "the second talk")
	_ = conn2.Close()
	waitForCursor(t, ts, "main-hall", 1)

	segs, err := st.Load("main-hall")
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 || !contains(segs[0].Text, "second") {
		t.Fatalf("the stored transcript should hold only the second talk, got %+v", segs)
	}
}

func TestDeleteAlsoTakesTheTranscriptAway(t *testing.T) {
	// Removing a room used to leave its transcript on disk, so it reappeared
	// at the next restart with the talk still in it.
	ts, dir, st := adminServer(t)

	conn := dialPublisher(t, ts, "main-hall", "publish-secret")
	publishFinal(t, conn, "a talk nobody wants to keep listed")
	_ = conn.Close()
	waitForCursor(t, ts, "main-hall", 1)

	if resp := do(t, ts, http.MethodDelete, "/api/admin/rooms/main-hall", "admin-secret", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: got %d, want 204", resp.StatusCode)
	}

	rooms, err := st.Rooms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 0 {
		t.Errorf("the room would come back at the next restart: %v", rooms)
	}
	archives, _ := filepath.Glob(filepath.Join(dir, store.ArchiveDir, "main-hall-*.jsonl"))
	if len(archives) != 1 {
		t.Errorf("the transcript should still exist in the archive, found %v", archives)
	}
}

func TestRetitlingARoomLeavesItsTranscriptAlone(t *testing.T) {
	ts, _, _ := adminServer(t)

	conn := dialPublisher(t, ts, "main-hall", "publish-secret")
	publishFinal(t, conn, "words that must survive a rename")
	_ = conn.Close()
	waitForCursor(t, ts, "main-hall", 1)

	resp := do(t, ts, http.MethodPut, "/api/admin/rooms/main-hall", "admin-secret",
		protocol.Room{Title: "Second Talk", Speaker: "Another Speaker", Track: "Track B"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upsert: got %d, want 200", resp.StatusCode)
	}

	body := transcriptText(t, ts, "main-hall")
	if !contains(body, "words that must survive a rename") {
		t.Errorf("retitling lost the transcript:\n%s", body)
	}
	if !contains(body, "Second Talk") || !contains(body, "Another Speaker") {
		t.Errorf("the new title and speaker are not in the transcript header:\n%s", body)
	}
}

func TestAdminSessionCookieWorksInPlaceOfABearerToken(t *testing.T) {
	ts, _, _ := adminServer(t)
	jar := newJar(t)
	client := &http.Client{Jar: jar}

	body, _ := json.Marshal(map[string]string{"token": "admin-secret"})
	resp, err := client.Post(ts.URL+"/api/admin/session", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session: got %d, want 200", resp.StatusCode)
	}

	// No Authorization header this time: the cookie has to carry it.
	listed, err := client.Get(ts.URL + "/api/admin/rooms")
	if err != nil {
		t.Fatal(err)
	}
	_ = listed.Body.Close()
	if listed.StatusCode != http.StatusOK {
		t.Errorf("listing rooms with only the cookie: got %d, want 200", listed.StatusCode)
	}
}

func TestAdminSessionRefusesTheWrongToken(t *testing.T) {
	ts, _, _ := adminServer(t)
	body, _ := json.Marshal(map[string]string{"token": "guessing"})
	resp, err := http.Post(ts.URL+"/api/admin/session", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", resp.StatusCode)
	}
}
