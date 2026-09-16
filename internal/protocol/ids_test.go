package protocol

import "testing"

func TestValidRoomID(t *testing.T) {
	valid := []string{"main-hall", "track_a", "room2", "a"}
	for _, id := range valid {
		if !ValidRoomID(id) {
			t.Errorf("%q should be valid", id)
		}
	}
	invalid := []string{"", "Main-Hall", "-leading", "has space", "../etc", "a/b", string(make([]byte, 100))}
	for _, id := range invalid {
		if ValidRoomID(id) {
			t.Errorf("%q should be invalid", id)
		}
	}
}

func TestNormaliseRoomID(t *testing.T) {
	cases := map[string]string{
		"Track A — Main Hall": "track-a-main-hall",
		"  Room 2!  ":         "room-2",
		"already-fine":        "already-fine",
		"!!!":                 "",
	}
	for in, want := range cases {
		if got := NormaliseRoomID(in); got != want {
			t.Errorf("NormaliseRoomID(%q) = %q, want %q", in, got, want)
		}
		if want != "" && !ValidRoomID(NormaliseRoomID(in)) {
			t.Errorf("NormaliseRoomID(%q) produced an invalid id", in)
		}
	}
}
