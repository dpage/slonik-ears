//go:build cgo

package audio

import (
	"math"
	"testing"
)

// interleave builds a multichannel buffer from per-channel constant levels.
func interleave(frames int, levels ...float32) []float32 {
	out := make([]float32, 0, frames*len(levels))
	for f := 0; f < frames; f++ {
		out = append(out, levels...)
	}
	return out
}

func TestMixDownKeepsTheFullLevelOfOneLiveChannel(t *testing.T) {
	// The bug, in one test. A microphone on input 1 of a sixteen-input
	// interface, the other fifteen unplugged and silent. Averaging every
	// channel — which is what a driver does when asked for mono it cannot
	// provide — returns a sixteenth of the signal.
	levels := make([]float32, 16)
	levels[0] = 0.42
	in := interleave(100, levels...)

	got := MixDown(in, 16, []int{0}, nil)
	if len(got) != 100 {
		t.Fatalf("got %d mono samples from 100 frames", len(got))
	}
	for _, v := range got {
		if math.Abs(float64(v-0.42)) > 1e-6 {
			t.Fatalf("sample is %v, want the full 0.42 rather than an average", v)
		}
	}

	// And for contrast, what the old behaviour amounted to.
	all := make([]int, 16)
	for i := range all {
		all[i] = i
	}
	averaged := MixDown(in, 16, all, nil)
	if want := float32(0.42 / 16); math.Abs(float64(averaged[0]-want)) > 1e-6 {
		t.Fatalf("averaging all channels gave %v, expected %v", averaged[0], want)
	}
}

func TestMixDownAveragesSeveralSelectedChannels(t *testing.T) {
	// A genuine stereo microphone on inputs 1 and 2, given as --channel 1,2.
	in := interleave(50, 0.4, 0.2, 0, 0)
	got := MixDown(in, 4, []int{0, 1}, nil)
	if want := float32(0.3); math.Abs(float64(got[0]-want)) > 1e-6 {
		t.Fatalf("got %v, want the mean of the two selected channels (%v)", got[0], want)
	}
}

func TestMixDownPicksTheRightChannel(t *testing.T) {
	// A microphone on input 3 is the case that looks like a broken microphone
	// if the code silently assumes input 1.
	in := interleave(10, 0, 0, 0.5, 0)
	if got := MixDown(in, 4, []int{2}, nil); math.Abs(float64(got[0]-0.5)) > 1e-6 {
		t.Errorf("channel 3 gave %v, want 0.5", got[0])
	}
	if got := MixDown(in, 4, []int{0}, nil); got[0] != 0 {
		t.Errorf("channel 1 gave %v, want silence", got[0])
	}
}

func TestMixDownHandlesAwkwardInput(t *testing.T) {
	// A trailing partial frame must be ignored rather than read past.
	in := []float32{0.1, 0.2, 0.3}
	if got := MixDown(in, 2, []int{0}, nil); len(got) != 1 || got[0] != 0.1 {
		t.Errorf("got %v, want one sample of 0.1", got)
	}
	// Nothing selected falls back to the first channel rather than silence.
	if got := MixDown(interleave(4, 0.7, 0), 2, nil, nil); got[0] != 0.7 {
		t.Errorf("got %v, want the first channel", got[0])
	}
	// A channel beyond the device is skipped, not read out of bounds.
	if got := MixDown(interleave(4, 0.7, 0), 2, []int{9}, nil); len(got) != 4 || got[0] != 0 {
		t.Errorf("got %v, want four silent samples", got)
	}
	// Mono passes straight through, which is the laptop microphone case.
	if got := MixDown(interleave(3, 0.25), 1, []int{0}, nil); len(got) != 3 || got[0] != 0.25 {
		t.Errorf("got %v, want 0.25 unchanged", got)
	}
}

func TestResolveChannels(t *testing.T) {
	for _, tc := range []struct {
		name    string
		wanted  []int
		have    int
		want    []int
		wantErr bool
	}{
		{name: "nothing asked for means work it out", wanted: nil, have: 16, want: nil},
		{name: "one channel, converted to 0-based", wanted: []int{3}, have: 16, want: []int{2}},
		{name: "a stereo pair", wanted: []int{1, 2}, have: 2, want: []int{0, 1}},
		{name: "duplicates collapse", wanted: []int{1, 1, 2}, have: 4, want: []int{0, 1}},
		{name: "beyond the device is refused", wanted: []int{17}, have: 16, wantErr: true},
		{name: "zero is refused, since channels are 1-based", wanted: []int{0}, have: 16, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveChannels(tc.wanted, tc.have)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}
