package audio

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"time"
)

type source struct {
	SampleRate int
	Channels   int
	Format     format

	file io.ReadCloser
}

func decodeAudioFile(filename string) (*source, error) {
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
	return &source{
		SampleRate: 22000,
		Channels:   1,
		Format:     "s16le",
		file:       r,
	}, nil
}

func (s *source) ReadSamples(period time.Duration) []float64 {
	numBytes := s.Format.Bits() / 8
	buf := make([]byte, s.SampleRate*s.Channels*int(period)/int(time.Second)*numBytes)
	n, err := io.ReadAtLeast(s.file, buf, len(buf))
	if err != nil {
		return make([]float64, time.Duration(s.SampleRate)*period/time.Second)
	}

	samples := make([]float64, n/numBytes)
	switch s.Format {
	case "s16le":
		for i := range samples {
			offset := i * s.Channels * numBytes
			bytes := buf[offset : offset+numBytes]
			sample := int16(bytes[0]) | int16(bytes[1])<<8
			samples[i] = float64(sample) / float64(0x7fff)
		}
	default:
		panic(fmt.Sprintf("Unimplemented format %q", s.Format))
	}
	return samples
}

func (s *source) Close() error {
	return s.file.Close()
}

type format string

func (f format) Bits() int {
	s := f[1:3]
	if s[1] < '0' || '9' < s[1] {
		s = s[:1]
	}
	b, err := strconv.Atoi(string(s))
	if err != nil {
		panic(err)
	}
	return b
}
