package audio

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"
)

type audioSource interface {
	SampleRate() float32
	ReadSamples(period time.Duration) []float64
}

type rawSource struct {
	file                 io.Reader
	sampleRate, channels int
	format               format
}

func (s rawSource) SampleRate() float32 {
	return float32(s.sampleRate)
}

func (s *rawSource) ReadSamples(period time.Duration) []float64 {
	numBytes := s.format.Bits() / 8
	buf := make([]byte, s.sampleRate*s.channels*int(period)/int(time.Second)*numBytes)
	n, err := io.ReadAtLeast(s.file, buf, len(buf))
	if err != nil {
		return make([]float64, time.Duration(s.sampleRate)*period/time.Second)
	}

	samples := make([]float64, n/numBytes)
	switch s.format {
	case "s16le":
		for i := range samples {
			offset := i * s.channels * numBytes
			bytes := buf[offset : offset+numBytes]
			sample := int16(bytes[0]) | int16(bytes[1])<<8
			samples[i] = float64(sample) / float64(0x7fff)
		}
	default:
		panic(fmt.Sprintf("Unimplemented format %q", s.format))
	}
	return samples
}

func decodeAudioFile(filename string) (channels, samplerate int, ft format, stream io.Reader) {
	// TODO: Close ffmpeg
	r, w := io.Pipe()
	go func() {
		cmd := exec.Command(
			"ffmpeg",
			"-i", filename,
			"-f", "s16le",
			"-acodec", "pcm_s16le",
			"-ac", "1",
			"-ar", "22000",
			"-",
		)
		cmd.Stdout = w
		if err := cmd.Run(); err != nil {
			if err := w.CloseWithError(err); err != nil {
				log.Print(err)
			}
			return
		}
		w.Close()
	}()
	return 1, 22000, "s16le", r
}
