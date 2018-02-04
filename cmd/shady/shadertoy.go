package main

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/mjibson/go-dsp/fft"
	"github.com/polyfloyd/shady"
)

// Wtf, this is not defined by go-gl?
const glLUMINANCE = 0x1909

const audioTexWidth = 512

var (
	inputMappingRe = regexp.MustCompile("(?m)^\\/\\/\\s+map\\s+(\\w+)=([^:]+):(.+)$")
	ichannelNumRe  = regexp.MustCompile("^iChannel(\\d+)$")
	audioValueRe   = regexp.MustCompile("^([^,]+);(\\d+):(\\d+):(\\d+)$")
)

var texIndexEnum uint32

// ShaderToy implements a shader environment similar to the one on
// shadertoy.com.
type ShaderToy struct {
	Source string

	resources []resource
}

func (st ShaderToy) Sources() map[glsl.Stage][]string {
	mappings := extractMappings(st.Source)
	mappedUniforms := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		if typ, ok := mapping.SamplerType(); ok {
			mappedUniforms = append(mappedUniforms, fmt.Sprintf("uniform %s %s;", typ, mapping.Name))
		}
	}

	return map[glsl.Stage][]string{
		glsl.StageVertex: {`
			#version 130
			attribute vec3 vert;
			void main(void) {
				gl_Position = vec4(vert, 1.0);
			}
		`},
		glsl.StageFragment: {`
			#version 130
			uniform vec3 iResolution;
			uniform float iTime;
			uniform float iTimeDelta;
			uniform float iFrame;
			uniform float iChannelTime[4];
			uniform vec4 iMouse;
			uniform vec4 iDate;
			uniform float iSampleRate;
			uniform vec3 iChannelResolution[4];
		`,
			strings.Join(mappedUniforms, "\n"),
			st.Source,
			`
			void main(void) {
				mainImage(gl_FragColor, gl_FragCoord.xy);
			}
		`},
	}
}

func (st *ShaderToy) Setup() error {
	mappings := extractMappings(st.Source)
	for _, mapping := range mappings {
		res, err := mapping.Resource()
		if err != nil {
			return err
		}
		st.resources = append(st.resources, res)
	}
	// If no mappings are found, we're good to go. If iChannels are referenced
	// anyway we'll let OpenGL decide if we should abort.
	return nil
}

func (st ShaderToy) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	// https://shadertoyunofficial.wordpress.com/2016/07/20/special-shadertoy-features/
	if loc, ok := uniforms["iResolution"]; ok {
		gl.Uniform3f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasHeight), 0.0)
	}
	if loc, ok := uniforms["iTime"]; ok {
		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	}
	if loc, ok := uniforms["iTimeDelta"]; ok {
		gl.Uniform1f(loc.Location, float32(state.Interval)/float32(time.Second))
	}
	if loc, ok := uniforms["iDate"]; ok {
		t := time.Now()
		sinceMidnight := t.Sub(t.Truncate(time.Hour * 24))
		gl.Uniform4f(loc.Location,
			float32(t.Year()-1),
			float32(t.Month()-1),
			float32(t.Day()),
			float32(sinceMidnight)/float32(time.Second),
		)
	}
	if loc, ok := uniforms["iFrame"]; ok {
		gl.Uniform1f(loc.Location, float32(state.FramesProcessed))
	}
	for _, resource := range st.resources {
		resource.PreRender(uniforms, state)
	}
}

type resource interface {
	PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState)
}

// A mapping is a parsed representation of a "map <name>=<namespace>:<value>"
// directive.
type mapping struct {
	Name      string
	Namespace string
	Value     string
}

func extractMappings(shaderSource string) []mapping {
	matches := inputMappingRe.FindAllStringSubmatch(shaderSource, -1)
	mappings := make([]mapping, 0, len(matches))
	for _, match := range matches {
		mappings = append(mappings, mapping{
			Name:      match[1],
			Namespace: match[2],
			Value:     match[3],
		})
	}
	return mappings
}

func (m mapping) SamplerType() (string, bool) {
	if m.Namespace == "builtin" {
		switch m.Value {
		case "RGBA Noise Small":
			return "sampler2D", true
		case "RGBA Noise Medium":
			return "sampler2D", true
		default:
			return "", false
		}
	}
	switch m.Namespace {
	case "audio":
		return "sampler2D", true
	case "image":
		return "sampler2D", true
	default:
		return "", false
	}
}

func (m mapping) Resource() (resource, error) {
	if m.Namespace == "builtin" {
		switch m.Value {
		case "RGBA Noise Small": // 64x64 4channels uint8
			return newImageTexture(noise(image.Rect(0, 0, 64, 64)), m.Name)
		case "RGBA Noise Medium": // 256x256 4channels uint8
			return newImageTexture(noise(image.Rect(0, 0, 256, 256)), m.Name)
		default:
			return nil, fmt.Errorf("unknown builtin mapping %q", m.Value)
		}
	}
	switch m.Namespace {
	case "audio":
		match := audioValueRe.FindStringSubmatch(m.Value)
		if match == nil {
			return nil, fmt.Errorf("could not parse audio value: %q (format: %s)", m.Value, audioValueRe)
		}
		filename := resolvePath(match[1])
		samplerate, err := strconv.Atoi(match[2])
		if err != nil {
			return nil, err
		}
		bits, err := strconv.Atoi(match[3])
		if err != nil {
			return nil, err
		}
		channels, err := strconv.Atoi(match[4])
		if err != nil {
			return nil, err
		}

		fd, err := os.Open(filename)
		if err != nil {
			return nil, fmt.Errorf("could not open audio source: %v", err)
		}
		source := &rawSource{
			file:       fd,
			sampleRate: samplerate,
			channels:   channels,
			bits:       bits,
		}
		return newAudioTexture(m.Name, source)

	case "image":
		fd, err := os.Open(resolvePath(m.Value))
		if err != nil {
			return nil, err
		}
		defer fd.Close()
		img, _, err := image.Decode(fd)
		if err != nil {
			return nil, err
		}
		return newImageTexture(img, m.Name)
	default:
		return nil, fmt.Errorf("don't know how to map %s", m.Namespace)
	}
}

// imageTexture is a mapping of a static image texture.
type imageTexture struct {
	uniformName string
	id          uint32
	index       uint32
	rect        image.Rectangle
}

func newImageTexture(img image.Image, uniformName string) (*imageTexture, error) {
	tex := &imageTexture{
		uniformName: uniformName,
		index:       texIndexEnum,
		rect:        img.Bounds(),
	}
	texIndexEnum++
	gl.GenTextures(1, &tex.id)
	gl.BindTexture(gl.TEXTURE_2D, tex.id)

	var rgbaImg *image.RGBA
	if i, ok := img.(*image.RGBA); ok {
		rgbaImg = i
	} else {
		rgbaImg = image.NewRGBA(img.Bounds())
		draw.Draw(rgbaImg, img.Bounds(), img, image.Point{X: 0, Y: 0}, draw.Over)
	}

	gl.TexImage2D(
		gl.TEXTURE_2D,            // target
		0,                        // level
		gl.RGBA,                  // internalFormat
		int32(img.Bounds().Dx()), // width
		int32(img.Bounds().Dy()), // height
		0,                   // border
		gl.RGBA,             // format
		gl.UNSIGNED_BYTE,    // type
		gl.Ptr(rgbaImg.Pix), // data
	)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.BindTexture(gl.TEXTURE_2D, 0)
	return tex, nil
}

func (tex *imageTexture) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	if loc, ok := uniforms[tex.uniformName]; ok {
		gl.BindTexture(gl.TEXTURE_2D, tex.id)
		gl.ActiveTexture(gl.TEXTURE0 + tex.index)
		gl.Uniform1i(loc.Location, int32(tex.index))
	}
	if m := ichannelNumRe.FindStringSubmatch(tex.uniformName); m != nil {
		if loc, ok := uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(tex.rect.Dx()), float32(tex.rect.Dy()), 1.0)
		}
	}
}

func noise(rect image.Rectangle) image.Image {
	img := image.NewRGBA(rect)
	rng := rand.New(rand.NewSource(1337))
	rng.Read(img.Pix)
	return img
}

// audioTexture is a mapping of an audio stream.
type audioTexture struct {
	uniformName string
	id          uint32
	index       uint32
	source      audioSource
}

func newAudioTexture(uniformName string, source audioSource) (*audioTexture, error) {
	at := &audioTexture{
		uniformName: uniformName,
		index:       texIndexEnum,
		source:      source,
	}
	texIndexEnum++
	gl.GenTextures(1, &at.id)
	gl.BindTexture(gl.TEXTURE_2D, at.id)

	var initialData [audioTexWidth * 2]byte
	gl.TexImage2D(
		gl.TEXTURE_2D,          // target
		0,                      // level
		glLUMINANCE,            // internalFormat
		audioTexWidth,          // width
		2,                      // height
		0,                      // border
		glLUMINANCE,            // format
		gl.UNSIGNED_BYTE,       // type
		gl.Ptr(initialData[:]), // data
	)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	return at, nil
}

func (at *audioTexture) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	period := at.source.ReadSamples(state.Interval)
	if len(period) < audioTexWidth {
		period = make([]float64, audioTexWidth)
	} else {
		period = period[len(period)-audioTexWidth:]
	}

	if loc, ok := uniforms[at.uniformName]; ok {
		freqs := fft.FFTReal(period)
		textureData := make([]uint8, audioTexWidth*2)
		for x := 0; x < audioTexWidth/2; x++ {
			// FFT
			textureData[x*2] = uint8((real(freqs[x])*0.5 + 0.5) * 255.0)
			textureData[x*2+1] = uint8((imag(freqs[x])*0.5 + 0.5) * 255.0)
		}
		for x := 0; x < audioTexWidth; x++ {
			// Wave
			textureData[audioTexWidth+x] = uint8((period[x]*0.5 + 0.5) * 255.0)
		}
		gl.BindTexture(gl.TEXTURE_2D, at.id)
		gl.TexSubImage2D(
			gl.TEXTURE_2D,       // target,
			0,                   // level,
			0,                   // xoffset,
			0,                   // yoffset,
			audioTexWidth,       // width,
			2,                   // height,
			glLUMINANCE,         // format,
			gl.UNSIGNED_BYTE,    // type,
			gl.Ptr(textureData), // data
		)
		gl.ActiveTexture(gl.TEXTURE0 + at.index)
		gl.Uniform1i(loc.Location, int32(at.index))
	}
	if m := ichannelNumRe.FindStringSubmatch(at.uniformName); m != nil {
		if loc, ok := uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(audioTexWidth), float32(2), 1.0)
		}
	}
	if m := ichannelNumRe.FindStringSubmatch(at.uniformName); m != nil {
		if loc, ok := uniforms[fmt.Sprintf("iChannelTime[%s]", m[1])]; ok {
			gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
		}
	}
	if loc, ok := uniforms["iSampleRate"]; ok {
		gl.Uniform1f(loc.Location, at.source.SampleRate())
	}
}

type audioSource interface {
	SampleRate() float32

	ReadSamples(period time.Duration) []float64
}

type rawSource struct {
	file                       io.Reader
	sampleRate, channels, bits int
}

func (s rawSource) SampleRate() float32 {
	return float32(s.sampleRate)
}

func (s *rawSource) ReadSamples(period time.Duration) []float64 {
	numBytes := s.bits / 8
	buf := make([]byte, (time.Duration(s.sampleRate*s.channels*numBytes)*period)/time.Second)
	n, err := io.ReadAtLeast(s.file, buf, len(buf))
	if err != nil {
		return make([]float64, time.Duration(s.sampleRate)*period/time.Second)
	}

	samples := make([]float64, n/numBytes)
	for i := range samples {
		offset := i * s.channels * numBytes
		bytes := buf[offset : offset+numBytes]

		switch numBytes {
		case 2:
			// 16 bit little endian signed
			sample := int16(bytes[0]) | int16(bytes[1])<<8
			samples[i] = float64(sample) / float64(0x7fff)
		default:
			panic(fmt.Sprintf("UNIMPLEMENTED: bits=%d", s.bits))
		}
	}
	return samples
}

func resolvePath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home := os.Getenv("HOME")
		if home == "" {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
