package audio

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/mjibson/go-dsp/fft"

	"github.com/polyfloyd/shady/renderer"
	"github.com/polyfloyd/shady/shadertoy"
)

func init() {
	shadertoy.RegisterResourceType("audio", func(m shadertoy.Mapping, texIndexEnum *uint32, _ renderer.RenderState) (shadertoy.Resource, error) {
		source, err := parseMappingValue(m.PWD, m.Value)
		if err != nil {
			return nil, err
		}
		r := newAudioTexture(m.Name, source, *texIndexEnum)
		*texIndexEnum++
		return r, nil
	})
}

const audioTexWidth = 512

var (
	audioGenericValueRe = regexp.MustCompile(`^([^;]+)$`)
	audioPCMValueRe     = regexp.MustCompile(`^([^;]+);(\d+):(\d+):([su]\d{1,2}[lb]e)$`)
)

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

func parseMappingValue(pwd, value string) (audioSource, error) {
	if match := audioGenericValueRe.FindStringSubmatch(value); match != nil {
		channels, samplerate, ft, pcmStream := decodeAudioFile(match[1])
		return &rawSource{
			file:       pcmStream,
			sampleRate: samplerate,
			channels:   channels,
			format:     ft,
		}, nil
	}

	match := audioPCMValueRe.FindStringSubmatch(value)
	if match == nil {
		return nil, fmt.Errorf("could not parse audio value: %q (format: %s)", value, audioPCMValueRe)
	}
	filename, err := shadertoy.ResolvePath(pwd, match[1])
	if err != nil {
		return nil, err
	}
	samplerate, err := strconv.Atoi(match[2])
	if err != nil {
		return nil, err
	}
	channels, err := strconv.Atoi(match[3])
	if err != nil {
		return nil, err
	}
	format := format(match[4])
	if format.Bits()%8 != 0 {
		return nil, fmt.Errorf("the number of PCM sample bits must be a multiple of 8, format: %q", format)
	}

	fd, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("could not open audio source: %v", err)
	}
	return &rawSource{
		file:       fd,
		sampleRate: samplerate,
		channels:   channels,
		format:     format,
	}, nil
}

// audioTexture is a mapping of an audio stream.
type audioTexture struct {
	uniformName string
	id          uint32
	index       uint32
	source      audioSource

	prevPeriod []float64
}

func newAudioTexture(uniformName string, source audioSource, texIndex uint32) *audioTexture {
	at := &audioTexture{
		uniformName: uniformName,
		index:       texIndex,
		source:      source,
		prevPeriod:  make([]float64, audioTexWidth),
	}
	gl.GenTextures(1, &at.id)
	gl.BindTexture(gl.TEXTURE_2D, at.id)

	var initialData [audioTexWidth * 2 * 3]uint8
	gl.TexImage2D(
		gl.TEXTURE_2D,          // target
		0,                      // level
		gl.RGBA,                // internalFormat
		audioTexWidth,          // width
		2,                      // height
		0,                      // border
		gl.RGB,                 // format
		gl.UNSIGNED_BYTE,       // type
		gl.Ptr(initialData[:]), // data
	)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	return at
}

func (at *audioTexture) UniformSource() string {
	return fmt.Sprintf(`
		uniform sampler2D %s;
		uniform vec3 %sSize;
		uniform float %sCurTime;
	`, at.uniformName, at.uniformName, at.uniformName)
}

func (at *audioTexture) PreRender(state renderer.RenderState) {
	newPeriod := at.source.ReadSamples(state.Interval)
	at.prevPeriod = append(at.prevPeriod, newPeriod...)[len(newPeriod):]
	period := at.prevPeriod[len(at.prevPeriod)-audioTexWidth:]

	if loc, ok := state.Uniforms[at.uniformName]; ok {
		textureData := make([]uint8, audioTexWidth*2*3)
		freqs := fft.FFTReal(period)
		for x := 0; x < audioTexWidth/2; x++ {
			// FFT
			fft1 := uint8((real(freqs[x])*0.5 + 0.5) * 255.0)
			fft2 := uint8((imag(freqs[x])*0.5 + 0.5) * 255.0)
			textureData[x*2*3+0] = fft1
			textureData[x*2*3+1] = fft1
			textureData[x*2*3+2] = fft1
			textureData[(x*2+1)*3+0] = fft2
			textureData[(x*2+1)*3+1] = fft2
			textureData[(x*2+1)*3+2] = fft2
		}
		for x := 0; x < audioTexWidth; x++ {
			// Wave
			wave := uint8((period[x]*0.5 + 0.5) * 255.0)
			textureData[(audioTexWidth+x)*3+0] = wave
			textureData[(audioTexWidth+x)*3+1] = wave
			textureData[(audioTexWidth+x)*3+2] = wave
		}

		gl.ActiveTexture(gl.TEXTURE0 + at.index)
		gl.BindTexture(gl.TEXTURE_2D, at.id)
		gl.TexSubImage2D(
			gl.TEXTURE_2D,       // target,
			0,                   // level,
			0,                   // xoffset,
			0,                   // yoffset,
			audioTexWidth,       // width,
			2,                   // height,
			gl.RGB,              // format,
			gl.UNSIGNED_BYTE,    // type,
			gl.Ptr(textureData), // data
		)
		gl.Uniform1i(loc.Location, int32(at.index))
	}
	if m := shadertoy.IchannelNumRe.FindStringSubmatch(at.uniformName); m != nil {
		if loc, ok := state.Uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(audioTexWidth), 2.0, 1.0)
		}
	}
	if loc, ok := state.Uniforms[fmt.Sprintf("%sSize", at.uniformName)]; ok {
		gl.Uniform3f(loc.Location, float32(audioTexWidth), 2.0, 1.0)
	}
	if m := shadertoy.IchannelNumRe.FindStringSubmatch(at.uniformName); m != nil {
		if loc, ok := state.Uniforms[fmt.Sprintf("iChannelTime[%s]", m[1])]; ok {
			gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
		}
	}
	if loc, ok := state.Uniforms[fmt.Sprintf("%sCurTime", at.uniformName)]; ok {
		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	}
	if loc, ok := state.Uniforms["iSampleRate"]; ok {
		gl.Uniform1f(loc.Location, at.source.SampleRate())
	}
}

func (at *audioTexture) Close() error {
	gl.DeleteTextures(1, &at.id)
	return nil
}
