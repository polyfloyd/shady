package shadertoy

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady"
)

func init() {
	glsl.RegisterEnvironmentDetector(func(shaderSource string) string {
		// The mainImage function should always be present in ShaderToy image
		// shaders.
		reShaderToy := regexp.MustCompile("void\\s+mainImage\\s*\\(\\s*out\\s+vec4\\s+\\w+\\s*,\\s*(?:in)?\\s+vec2\\s+\\w+\\s*\\)")
		if reShaderToy.MatchString(shaderSource) {
			return "shadertoy"
		}
		return ""
	})
}

// Wtf, this is not defined by go-gl?
const glLUMINANCE = 0x1909

var (
	inputMappingSourceRe = regexp.MustCompile(`(?m)^#pragma\s+map\s+(\w+)=([^:]+):(.+)$`)
	inputMappingRe       = regexp.MustCompile(`^(\w+)=([^:]+):(.+)$`)
	ichannelNumRe        = regexp.MustCompile(`^iChannel(\d+)$`)
)

var texIndexEnum uint32

// ShaderToy implements a shader environment similar to the one on
// shadertoy.com.
type ShaderToy struct {
	ShaderSources []glsl.SourceFile
	ResolveDir    string
	Mappings      []Mapping

	resources []resource
}

func (st ShaderToy) Sources() (map[glsl.Stage][]glsl.Source, error) {
	ss := make([]glsl.Source, 0, len(st.ShaderSources))
	for _, s := range st.ShaderSources {
		ss = append(ss, s)
	}
	mappings, err := extractMappings(ss)
	if err != nil {
		return nil, err
	}

	mappedUniforms := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		if typ, ok := mapping.samplerType(); ok {
			mappedUniforms = append(mappedUniforms, fmt.Sprintf("uniform %s %s;", typ, mapping.Name))
		}
	}

	return map[glsl.Stage][]glsl.Source{
		glsl.StageVertex: {glsl.SourceBuf(`
			#version 130
			attribute vec3 vert;
			void main(void) {
				gl_Position = vec4(vert, 1.0);
			}
		`)},
		glsl.StageFragment: func() []glsl.Source {
			ss := []glsl.Source{}
			ss = append(ss, glsl.SourceBuf(`
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
			`))
			ss = append(ss, glsl.SourceBuf(strings.Join(mappedUniforms, "\n")))
			for _, s := range st.ShaderSources {
				ss = append(ss, s)
			}
			ss = append(ss, glsl.SourceBuf(`
				void main(void) {
					mainImage(gl_FragColor, gl_FragCoord.xy);
				}
			`))
			return ss
		}(),
	}, nil
}

func (st *ShaderToy) Setup() error {
	ss := make([]glsl.Source, 0, len(st.ShaderSources))
	for _, s := range st.ShaderSources {
		ss = append(ss, s)
	}
	mappings, err := extractMappings(ss)
	if err != nil {
		return err
	}

	for _, mapping := range mappings {
		res, err := mapping.resource(st.ResolveDir)
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

// A Mapping is a parsed representation of a "map <name>=<namespace>:<value>"
// directive.
type Mapping struct {
	Name      string
	Namespace string
	Value     string
}

func ParseMapping(str string) (Mapping, error) {
	match := inputMappingRe.FindStringSubmatch(str)
	if match != nil {
		return Mapping{
			Name:      match[1],
			Namespace: match[2],
			Value:     match[3],
		}, nil
	}
	return Mapping{}, fmt.Errorf("unable to parse mapping from %q", str)
}

func extractMappings(shaderSources []glsl.Source) ([]Mapping, error) {
	mappings := []Mapping{}
	for _, s := range shaderSources {
		src, err := s.Contents()
		if err != nil {
			return nil, err
		}
		matches := inputMappingSourceRe.FindAllSubmatch(src, -1)
		for _, match := range matches {
			mappings = append(mappings, Mapping{
				Name:      string(match[1]),
				Namespace: string(match[2]),
				Value:     string(match[3]),
			})
		}
	}
	return deduplicateMappings(mappings...), nil
}

// deduplicateMappings filters out mappings which appear multiple times in the
// specified lists by their name.
//
// Lists specified first have precedence.
func deduplicateMappings(inMappings ...Mapping) []Mapping {
	var outMappings []Mapping
	set := map[string]bool{}
	for _, m := range inMappings {
		// Deduplicate by using map keys.
		if !set[m.Name] {
			set[m.Name] = true
			outMappings = append(outMappings, m)
		}
	}
	return outMappings
}

func (m Mapping) samplerType() (string, bool) {
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
	case "video":
		return "sampler2D", true
	case "perip_mat4":
		return "mat4", true
	default:
		return "", false
	}
}

func (m Mapping) resource(pwd string) (resource, error) {
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
		source, err := parseMappingValue(pwd, m.Value)
		if err != nil {
			return nil, err
		}
		return newAudioTexture(m.Name, source)

	case "image":
		fd, err := os.Open(resolvePath(pwd, m.Value))
		if err != nil {
			return nil, err
		}
		defer fd.Close()
		img, _, err := image.Decode(fd)
		if err != nil {
			return nil, err
		}
		return newImageTexture(img, m.Name)

	case "video":
		return newVideoTexture(m.Name, resolvePath(pwd, m.Value))

	case "perip_mat4":
		return newMat4Peripheral(m.Name, pwd, m.Value)

	default:
		return nil, fmt.Errorf("don't know how to map %s", m.Namespace)
	}
}
