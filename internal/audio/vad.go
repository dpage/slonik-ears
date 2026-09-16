package audio

import "math"

// VADConfig tunes voice activity detection.
type VADConfig struct {
	// StartFactor is how far above the estimated noise floor a frame must be
	// before it counts as speech. Conference rooms have air conditioning,
	// laptop fans and a permanent low rumble, so a simple fixed threshold
	// does not survive contact with reality.
	StartFactor float64
	// StopFactor is the lower threshold used once speech has started, giving
	// hysteresis so quiet syllables do not chop a sentence in half.
	StopFactor float64
	// MinRMS is an absolute floor; below this nothing is ever speech.
	MinRMS float64
	// AdaptRate is how quickly the noise floor follows the room, per frame.
	AdaptRate float64
}

// DefaultVADConfig is tuned for a lapel or lectern microphone in a room with
// an audience in it.
func DefaultVADConfig() VADConfig {
	return VADConfig{
		StartFactor: 3.0,
		StopFactor:  1.8,
		MinRMS:      0.004,
		AdaptRate:   0.02,
	}
}

// VAD is a cheap energy-based voice activity detector with an adaptive noise
// floor. It is not trying to be Silero; it only needs to decide where an
// utterance ends so the model gets a sensible chunk.
type VAD struct {
	cfg        VADConfig
	noiseFloor float64
	speaking   bool
	primed     bool
}

// NewVAD returns a detector. A zero config gets the defaults.
func NewVAD(cfg VADConfig) *VAD {
	def := DefaultVADConfig()
	if cfg.StartFactor <= 0 {
		cfg.StartFactor = def.StartFactor
	}
	if cfg.StopFactor <= 0 {
		cfg.StopFactor = def.StopFactor
	}
	if cfg.MinRMS <= 0 {
		cfg.MinRMS = def.MinRMS
	}
	if cfg.AdaptRate <= 0 {
		cfg.AdaptRate = def.AdaptRate
	}
	return &VAD{cfg: cfg}
}

// NoiseFloor exposes the current estimate, which is handy in logs when
// somebody asks why the transcript has gone quiet.
func (v *VAD) NoiseFloor() float64 { return v.noiseFloor }

// Push classifies one frame and returns whether it contains speech along with
// the frame's RMS level (0..1).
func (v *VAD) Push(frame []float32) (speech bool, level float64) {
	level = rms(frame)
	if !v.primed {
		v.noiseFloor = level
		v.primed = true
	}

	start := math.Max(v.noiseFloor*v.cfg.StartFactor, v.cfg.MinRMS)
	stop := math.Max(v.noiseFloor*v.cfg.StopFactor, v.cfg.MinRMS*0.6)

	if v.speaking {
		v.speaking = level >= stop
	} else {
		v.speaking = level >= start
	}

	// Only adapt the noise floor while nothing is being said, otherwise a
	// long monologue slowly convinces the detector that speech is silence.
	if !v.speaking {
		v.noiseFloor += (level - v.noiseFloor) * v.cfg.AdaptRate
	}
	return v.speaking, level
}

func rms(frame []float32) float64 {
	if len(frame) == 0 {
		return 0
	}
	var sum float64
	for _, s := range frame {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(frame)))
}

// Peak returns the largest absolute sample in a frame.
func Peak(frame []float32) float64 {
	var p float64
	for _, s := range frame {
		p = math.Max(p, math.Abs(float64(s)))
	}
	return p
}
