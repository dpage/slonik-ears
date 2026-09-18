package audio

import "testing"

// The devices a PulseAudio host actually offers, in the order it offers them.
// The monitors come first, which is what makes selecting by name a trap.
var pulseDevices = []Device{
	{ID: "alsa_output.usb-ANKER_Anker_PowerConf_S330.monitor", Name: "Monitor of Anker PowerConf S330 Analog Stereo", Monitor: true},
	{ID: "alsa_input.usb-ANKER_Anker_PowerConf_S330", Name: "Anker PowerConf S330 Analog Stereo", Default: true},
	{ID: "alsa_output.platform-builtin.monitor", Name: "Monitor of Built-in Audio Stereo", Monitor: true},
	{ID: "echo_cancel_source", Name: "EchoCancelSource"},
}

func TestSelectDevicePrefersARealInputOverAMonitor(t *testing.T) {
	// The bug this exists for: "Anker" matched "Monitor of Anker PowerConf
	// S330" because monitors are listed first. That device opens without
	// complaint and is silent, so the listener ran for ten minutes and
	// transcribed nothing at all.
	got, err := SelectDevice(pulseDevices, "Anker")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("selected %d (%q), want the microphone at 1", got, pulseDevices[got].Name)
	}
}

func TestSelectDeviceHonoursAnExplicitRequestForAMonitor(t *testing.T) {
	// Capturing what the machine is playing is a legitimate thing to want; it
	// is how you transcribe a video call. Saying "monitor" gets you one.
	got, err := SelectDevice(pulseDevices, "Monitor of Anker")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("selected %d (%q), want the monitor at 0", got, pulseDevices[got].Name)
	}
}

func TestSelectDeviceFallsBackToAMonitorWhenNothingElseMatches(t *testing.T) {
	// "Built-in" only matches a monitor here. Better to use it than to refuse:
	// the operator asked for something that exists.
	got, err := SelectDevice(pulseDevices, "Built-in")
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("selected %d, want 2", got)
	}
}

func TestSelectDeviceByIndexAndID(t *testing.T) {
	for _, tc := range []struct {
		selector string
		want     int
	}{
		{"0", 0},
		{"3", 3},
		{"echo_cancel_source", 3},
		{"EchoCancel", 3},
	} {
		got, err := SelectDevice(pulseDevices, tc.selector)
		if err != nil {
			t.Errorf("%q: %v", tc.selector, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q selected %d, want %d", tc.selector, got, tc.want)
		}
	}
}

func TestSelectDeviceRefusesWhatDoesNotExist(t *testing.T) {
	if _, err := SelectDevice(pulseDevices, "Scarlett"); err == nil {
		t.Error("a name matching nothing should be an error, not a silent default")
	}
	if _, err := SelectDevice(pulseDevices, "9"); err == nil {
		t.Error("an index beyond the list should be an error")
	}
}

func TestSelectDeviceDefaultsWhenNothingIsAsked(t *testing.T) {
	got, err := SelectDevice(pulseDevices, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != -1 {
		t.Errorf("got %d, want -1 meaning the system default", got)
	}
}

func TestIsMonitorName(t *testing.T) {
	for _, name := range []string{
		"Monitor of Anker PowerConf S330 Analog Stereo",
		"monitor of built-in audio",
	} {
		if !IsMonitorName(name) {
			t.Errorf("%q should be recognised as a monitor", name)
		}
	}
	for _, name := range []string{
		"Anker PowerConf S330 Analog Stereo",
		"EchoCancelSource",
		"MacBook Pro Microphone",
		// A microphone that happens to have the word in its name is not a
		// loopback; the giveaway is "monitor of".
		"Studio Monitor Controller Input",
	} {
		if IsMonitorName(name) {
			t.Errorf("%q should not be recognised as a monitor", name)
		}
	}
}
