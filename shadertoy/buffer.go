package shadertoy

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/go-gl/gl/v3.3-core/gl"

	"github.com/polyfloyd/shady/renderer"
)

func init() {
	resourceBuilders["buffer"] = func(m Mapping, texIndexEnum *uint32, _ renderer.RenderState) (Resource, error) {
		match := bufferValueRe.FindStringSubmatch(m.Value)
		if match == nil {
			return nil, fmt.Errorf("could not parse buffer value: %q (format: %s)", m.Value, bufferValueRe)
		}

		filename, err := ResolvePath(m.PWD, match[1])
		if err != nil {
			return nil, err
		}
		width, err := strconv.ParseUint(match[2], 10, 32)
		if err != nil {
			return nil, err
		}
		height, err := strconv.ParseUint(match[3], 10, 32)
		if err != nil {
			return nil, err
		}

		sources, err := renderer.Includes(filename)
		if err != nil {
			return nil, err
		}

		index := *texIndexEnum
		*texIndexEnum++
		return &bufferImage{
			name:     m.Name,
			index:    index,
			filename: filename,
			width:    uint(width),
			height:   uint(height),
			sources:  renderer.SourceFiles(sources...),
		}, nil
	}
}

var bufferValueRe = regexp.MustCompile(`^([^;]+);(\d+)x(\d+)$`)

type bufferImage struct {
	name  string
	index uint32

	filename      string
	width, height uint
	sources       []renderer.SourceFile
}

func (tex *bufferImage) UniformSource() string {
	return fmt.Sprintf(`
		uniform sampler2D %s;
		uniform vec3 %sSize;
	`, tex.name, tex.name)
}

func (tex *bufferImage) PreRender(state renderer.RenderState) {
	if loc, ok := state.Uniforms[tex.name]; ok {
		gl.ActiveTexture(gl.TEXTURE0 + tex.index)
		gl.BindTexture(gl.TEXTURE_2D, state.SubBuffers[tex.name])
		gl.Uniform1i(loc.Location, int32(tex.index))
	}
	if m := IchannelNumRe.FindStringSubmatch(tex.name); m != nil {
		if loc, ok := state.Uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(tex.width), float32(tex.height), 1.0)
		}
	}
	if loc, ok := state.Uniforms[fmt.Sprintf("%sSize", tex.name)]; ok {
		gl.Uniform3f(loc.Location, float32(tex.width), float32(tex.height), 1.0)
	}
}

func (tex *bufferImage) Close() error { return nil }
