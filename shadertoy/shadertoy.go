package shadertoy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"

	"github.com/polyfloyd/shady/renderer"
)

var (
	inputMappingSourceRe = regexp.MustCompile(`(?m)^#pragma\s+map\s+(\w+)=([^:]+):(.+)$`)
	inputMappingRe       = regexp.MustCompile(`^(\w+)=([^:]+):(.+)$`)
	IchannelNumRe        = regexp.MustCompile(`^iChannel(\d+)$`)
)

var texIndexEnum uint32

// A resource builder function a resource from instantiates a mapping
// definition that can offer additional functionality to the renderer.
//
// The map key is the Namespace of the mapping.
//
// The functions are called with the mapping that should be instantiated, the
// current working directory and an enumerator for texture IDs.
var resourceBuilders = map[string]ResourceBuildFunc{}

type ResourceBuildFunc func(Mapping, *uint32, renderer.RenderState) (Resource, error)

func RegisterResourceType(name string, fn ResourceBuildFunc) {
	if _, ok := resourceBuilders[name]; ok {
		panic(name + " is already registered as resource type")
	}
	resourceBuilders[name] = fn
}

// ShaderToy implements a shader environment similar to the one on
// shadertoy.com.
type ShaderToy struct {
	shaderSources []renderer.SourceFile
	mappings      []Mapping
	glslVersion   string

	resources []Resource
}

func NewShaderToy(
	shaderSources []renderer.SourceFile,
	overrideMappings []Mapping,
	glslVersion string,
) (*ShaderToy, error) {
	sourceMappings, err := extractMappings(shaderSources)
	if err != nil {
		return nil, err
	}
	mappings := deduplicateMappings(append(overrideMappings, sourceMappings...)...)

	return &ShaderToy{
		shaderSources: shaderSources,
		mappings:      mappings,
		glslVersion:   glslVersion,
		// resources is populated by Setup().
	}, nil
}

func (st ShaderToy) Sources() (map[renderer.Stage][]renderer.Source, error) {
	return map[renderer.Stage][]renderer.Source{
		renderer.StageVertex: {renderer.SourceBuf(fmt.Sprintf(`
			#version %s
			attribute vec3 vert;
			void main(void) {
				gl_Position = vec4(vert, 1.0);
			}
		`, st.glslVersion))},
		renderer.StageFragment: func() []renderer.Source {
			ss := []renderer.Source{}
			ss = append(ss, renderer.SourceBuf(fmt.Sprintf(`
				#version %s
				uniform vec3 iResolution;
				uniform float iTime;
				uniform float iTimeDelta;
				uniform float iFrame;
				uniform float iChannelTime[4];
				uniform vec4 iMouse;
				uniform vec4 iDate;
				uniform float iSampleRate;
				uniform vec3 iChannelResolution[4];
			`, st.glslVersion)))
			for _, res := range st.resources {
				ss = append(ss, renderer.SourceBuf(res.UniformSource()))
			}
			for _, s := range st.shaderSources {
				ss = append(ss, s)
			}
			ss = append(ss, renderer.SourceBuf(`
				void main(void) {
					mainImage(gl_FragColor, gl_FragCoord.xy);
				}
			`))
			return ss
		}(),
	}, nil
}

func (st *ShaderToy) Setup(state renderer.RenderState) error {
	if st.resources != nil {
		return fmt.Errorf("double call to ShaderToy.Setup")
	}
	for _, mapping := range st.mappings {
		res, err := mapping.resource(state)
		if err != nil {
			return err
		}
		st.resources = append(st.resources, res)
	}
	// If no mappings are found, we're good to go. If iChannels are referenced
	// anyway we'll let OpenGL decide if we should abort.
	return nil
}

func (st ShaderToy) SubEnvironments() (map[string]renderer.SubEnvironment, error) {
	envs := map[string]renderer.SubEnvironment{}
	for _, res := range st.resources {
		if bi, ok := res.(*bufferImage); ok {
			env, err := NewShaderToy(bi.sources, nil, st.glslVersion)
			if err != nil {
				return nil, err
			}
			envs[bi.name] = renderer.SubEnvironment{
				Environment: env,
				Width:       bi.width,
				Height:      bi.height,
			}
		}
	}
	return envs, nil
}

func (st ShaderToy) PreRender(state renderer.RenderState) {
	// https://shadertoyunofficial.wordpress.com/2016/07/20/special-shadertoy-features/
	if loc, ok := state.Uniforms["iResolution"]; ok {
		gl.Uniform3f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasHeight), 0.0)
	}
	if loc, ok := state.Uniforms["iTime"]; ok {
		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	}
	if loc, ok := state.Uniforms["iTimeDelta"]; ok {
		gl.Uniform1f(loc.Location, float32(state.Interval)/float32(time.Second))
	}
	if loc, ok := state.Uniforms["iDate"]; ok {
		t := time.Now()
		sinceMidnight := t.Sub(t.Truncate(time.Hour * 24))
		gl.Uniform4f(loc.Location,
			float32(t.Year()-1),
			float32(t.Month()-1),
			float32(t.Day()),
			float32(sinceMidnight)/float32(time.Second),
		)
	}
	if loc, ok := state.Uniforms["iFrame"]; ok {
		gl.Uniform1f(loc.Location, float32(state.FramesProcessed))
	}
	for _, resource := range st.resources {
		resource.PreRender(state)
	}
}

func (st *ShaderToy) Close() error {
	var errors []string
	for _, res := range st.resources {
		if err := res.Close(); err != nil {
			errors = append(errors, err.Error())
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("error shutting down ShaderToy resource(s): {%s}", strings.Join(errors, ", "))
	}
	return nil
}

type Resource interface {
	UniformSource() string
	PreRender(state renderer.RenderState)
	Close() error
}

// A Mapping is a parsed representation of a "map <name>=<namespace>:<value>"
// directive.
type Mapping struct {
	Name      string
	Namespace string
	Value     string
	PWD       string
}

func ParseMapping(str, pwd string) (Mapping, error) {
	match := inputMappingRe.FindStringSubmatch(str)
	if match != nil {
		return Mapping{
			Name:      match[1],
			Namespace: match[2],
			Value:     match[3],
			PWD:       pwd,
		}, nil
	}
	return Mapping{}, fmt.Errorf("unable to parse mapping from %q", str)
}

func extractMappings(shaderSources []renderer.SourceFile) ([]Mapping, error) {
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
				PWD:       s.Dir(),
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
		if !set[m.Name] {
			set[m.Name] = true
			outMappings = append(outMappings, m)
		}
	}
	return outMappings
}

func (m Mapping) resource(state renderer.RenderState) (Resource, error) {
	fn, ok := resourceBuilders[m.Namespace]
	if !ok {
		return nil, fmt.Errorf("don't know how to map %s", m.Namespace)
	}
	return fn(m, &texIndexEnum, state)
}

func ResolvePath(pwd, path string) (string, error) {
	if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
		return "", fmt.Errorf("URLs are not supported")
	}
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[1:]), nil
	}
	if !filepath.IsAbs(path) {
		return filepath.Join(pwd, path), nil
	}
	return path, nil
}
