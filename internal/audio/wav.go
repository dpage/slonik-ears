package audio

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeWAV renders mono float32 samples as a 16-bit PCM WAV file. Whisper
// backends all accept this, and at 16 kHz it is only 32 kB per second — a
// rounding error next to the time the model spends thinking.
func EncodeWAV(pcm []float32, sampleRate int) []byte {
	if sampleRate <= 0 {
		sampleRate = SampleRate
	}
	const (
		numChannels   = 1
		bitsPerSample = 16
	)
	dataLen := len(pcm) * 2
	buf := make([]byte, 0, 44+dataLen)

	put32 := func(v uint32) { buf = binary.LittleEndian.AppendUint32(buf, v) }
	put16 := func(v uint16) { buf = binary.LittleEndian.AppendUint16(buf, v) }

	buf = append(buf, "RIFF"...)
	put32(uint32(36 + dataLen))
	buf = append(buf, "WAVE"...)
	buf = append(buf, "fmt "...)
	put32(16)                                                   // PCM chunk size
	put16(1)                                                    // format: PCM
	put16(numChannels)                                          //
	put32(uint32(sampleRate))                                   //
	put32(uint32(sampleRate * numChannels * bitsPerSample / 8)) // byte rate
	put16(numChannels * bitsPerSample / 8)                      // block align
	put16(bitsPerSample)                                        //
	buf = append(buf, "data"...)
	put32(uint32(dataLen))

	for _, s := range pcm {
		v := s
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		put16(uint16(int16(math.Round(float64(v) * 32767))))
	}
	return buf
}

// DecodeWAV reads a RIFF/WAVE file into mono float32 samples at its native
// rate. 16-bit PCM and 32-bit float are supported, mono or multi-channel
// (extra channels are averaged down). That covers everything a Mac will hand
// you; anything exotic should go through ffmpeg first.
func DecodeWAV(data []byte) ([]float32, int, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("not a WAV file")
	}
	var (
		format     uint16
		channels   int
		sampleRate int
		bits       int
		raw        []byte
	)
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if size < 0 || body+size > len(data) {
			size = len(data) - body // tolerate a truncated final chunk
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, 0, fmt.Errorf("short fmt chunk")
			}
			format = binary.LittleEndian.Uint16(data[body : body+2])
			channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
			sampleRate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			bits = int(binary.LittleEndian.Uint16(data[body+14 : body+16]))
		case "data":
			raw = data[body : body+size]
		}
		pos = body + size
		if size%2 == 1 {
			pos++ // chunks are word aligned
		}
	}
	if raw == nil || channels <= 0 || sampleRate <= 0 {
		return nil, 0, fmt.Errorf("WAV file has no usable data chunk")
	}

	var mono []float32
	switch {
	case format == 1 && bits == 16:
		n := len(raw) / 2 / channels
		mono = make([]float32, 0, n)
		for i := 0; i+2*channels <= len(raw); i += 2 * channels {
			var sum float32
			for c := 0; c < channels; c++ {
				v := int16(binary.LittleEndian.Uint16(raw[i+2*c : i+2*c+2]))
				sum += float32(v) / 32768
			}
			mono = append(mono, sum/float32(channels))
		}
	case format == 3 && bits == 32:
		n := len(raw) / 4 / channels
		mono = make([]float32, 0, n)
		for i := 0; i+4*channels <= len(raw); i += 4 * channels {
			var sum float32
			for c := 0; c < channels; c++ {
				sum += math.Float32frombits(binary.LittleEndian.Uint32(raw[i+4*c : i+4*c+4]))
			}
			mono = append(mono, sum/float32(channels))
		}
	default:
		return nil, 0, fmt.Errorf("unsupported WAV format (format=%d bits=%d); convert to 16-bit PCM first", format, bits)
	}
	return mono, sampleRate, nil
}

// Resample converts between sample rates with linear interpolation. It is not
// a work of signal processing art, but for speech heading into a model that
// downsamples anyway, it is entirely adequate.
func Resample(in []float32, from, to int) []float32 {
	if from == to || from <= 0 || to <= 0 || len(in) == 0 {
		return in
	}
	ratio := float64(from) / float64(to)
	n := int(float64(len(in)) / ratio)
	out := make([]float32, n)
	for i := range out {
		src := float64(i) * ratio
		j := int(src)
		frac := float32(src - float64(j))
		if j+1 < len(in) {
			out[i] = in[j]*(1-frac) + in[j+1]*frac
		} else {
			out[i] = in[len(in)-1]
		}
	}
	return out
}
