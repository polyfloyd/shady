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
	shadertoy.RegisterResourceType("audio", func(m shadertoy.Mapping, genTexID shadertoy.GenTexFunc, _ renderer.RenderState) (shadertoy.Resource, error) {
		source, err := parseMappingValue(m.PWD, m.Value)
		if err != nil {
			return nil, err
		}
		r := newAudioTexture(m.Name, source, genTexID())
		return r, nil
	})
}

const (
	texWidth  = 512
	texHeight = 4
)

var (
	genericValueRe = regexp.MustCompile(`^([^;]+)$`)
	pcmValueRe     = regexp.MustCompile(`^([^;]+);(\d+):(\d+):([su]\d{1,2}[lb]e)$`)
)

func parseMappingValue(pwd, value string) (*source, error) {
	if match := genericValueRe.FindStringSubmatch(value); match != nil {
		return newAudioFileSource(match[1])
	}

	match := pcmValueRe.FindStringSubmatch(value)
	if match == nil {
		return nil, fmt.Errorf("could not parse audio value: %q (format: %s)", value, pcmValueRe)
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
	return &source{
		SampleRate: samplerate,
		Channels:   channels,
		Format:     format,
		file:       fd,
	}, nil
}

// texture is a mapping of an audio stream.
type texture struct {
	uniformName string
	id          uint32
	index       uint32
	source      *source

	prevPeriod     []float64
	stabilizedWave []float64
}

func newAudioTexture(uniformName string, source *source, texIndex uint32) *texture {
	at := &texture{
		uniformName:    uniformName,
		index:          texIndex,
		source:         source,
		prevPeriod:     make([]float64, texWidth),
		stabilizedWave: make([]float64, texWidth),
	}
	gl.GenTextures(1, &at.id)
	gl.BindTexture(gl.TEXTURE_2D, at.id)

	var initialData [texWidth * texHeight * 3]uint8
	gl.TexImage2D(
		gl.TEXTURE_2D,          // target
		0,                      // level
		gl.RGBA,                // internalFormat
		texWidth,               // width
		texHeight,              // height
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

func (at *texture) UniformSource() string {
	return fmt.Sprintf(`
		uniform sampler2D %s;
		uniform vec3 %sSize;
		uniform float %sCurTime;
	`, at.uniformName, at.uniformName, at.uniformName)
}

func (at *texture) PreRender(state renderer.RenderState) {
	newPeriod := at.source.ReadSamples(state.Interval)
	prevPeriod := at.prevPeriod[len(at.prevPeriod)-texWidth:]
	at.prevPeriod = append(at.prevPeriod, newPeriod...)[len(newPeriod):]
	period := at.prevPeriod[len(at.prevPeriod)-texWidth:]

	if loc, ok := state.Uniforms[at.uniformName]; ok {
		textureData := make([]uint8, texWidth*texHeight*3)
		// FFT
		freqs := fft.FFTReal(period)
		for x := 0; x < texWidth/2; x++ {
			fft1 := uint8((real(freqs[x])*0.5 + 0.5) * 255.0)
			fft2 := uint8((imag(freqs[x])*0.5 + 0.5) * 255.0)
			textureData[x*2*3+0] = fft1
			textureData[x*2*3+1] = fft1
			textureData[x*2*3+2] = fft1
			textureData[(x*2+1)*3+0] = fft2
			textureData[(x*2+1)*3+1] = fft2
			textureData[(x*2+1)*3+2] = fft2
		}
		// Wave
		for x := 0; x < texWidth; x++ {
			wave := uint8((period[x]*0.5 + 0.5) * 255.0)
			textureData[(texWidth+x)*3+0] = wave
			textureData[(texWidth+x)*3+1] = wave
			textureData[(texWidth+x)*3+2] = wave
		}
		// Stabilized Wave
		corrPeriod := period
		// Search the newly read samples for a window of samples that
		// resebles the previous period.
		best := -1.0
		for i := 0; i < len(newPeriod)-texWidth; i++ {
			p := newPeriod[i : i+texWidth]
			w := correlate(p, prevPeriod)
			if w > best {
				best = w
				corrPeriod = p
			}
		}
		const n = 0.35
		for x := 0; x < texWidth; x++ {
			at.stabilizedWave[x] = at.stabilizedWave[x]*(1-n) + corrPeriod[x]*n
			wave := uint8((at.stabilizedWave[x]*0.5 + 0.5) * 255.0)
			textureData[(texWidth*2+x)*3+0] = wave
			textureData[(texWidth*2+x)*3+1] = wave
			textureData[(texWidth*2+x)*3+2] = wave
		}

		gl.ActiveTexture(gl.TEXTURE0 + at.index)
		gl.BindTexture(gl.TEXTURE_2D, at.id)
		gl.TexSubImage2D(
			gl.TEXTURE_2D,       // target,
			0,                   // level,
			0,                   // xoffset,
			0,                   // yoffset,
			texWidth,            // width,
			texHeight,           // height,
			gl.RGB,              // format,
			gl.UNSIGNED_BYTE,    // type,
			gl.Ptr(textureData), // data
		)
		gl.Uniform1i(loc.Location, int32(at.index))
	}
	if m := shadertoy.IchannelNumRe.FindStringSubmatch(at.uniformName); m != nil {
		if loc, ok := state.Uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(texWidth), float32(texHeight), 1.0)
		}
	}
	if loc, ok := state.Uniforms[fmt.Sprintf("%sSize", at.uniformName)]; ok {
		gl.Uniform3f(loc.Location, float32(texWidth), float32(texHeight), 1.0)
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
		gl.Uniform1f(loc.Location, float32(at.source.SampleRate))
	}
}

func (at *texture) Close() error {
	at.source.Close()
	gl.DeleteTextures(1, &at.id)
	return nil
}

func correlate(a, b []float64) float64 {
	if len(a) != len(b) {
		panic("mismatched slice lengths")
	}
	w := 0.0
	for i := range a {
		w += a[i] * b[i]
	}
	return w
}
