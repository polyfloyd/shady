package main

import (
	"regexp"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady"
)

// GLSLSandbox implements the Environment interface to simulate the canvas of glslsandbox.com.
type GLSLSandbox struct{}

func (GLSLSandbox) Sources(sources map[uint32][]string) map[uint32][]string {
	sources[gl.VERTEX_SHADER] = append(sources[gl.VERTEX_SHADER], `
		attribute vec3 vert;
		varying vec2 surfacePosition;

		void main(void) {
			surfacePosition = vert.xy;
			gl_Position = vec4(vert, 1.0);
		}
	`)
	return sources
}

func (GLSLSandbox) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	if loc, ok := uniforms["resolution"]; ok {
		gl.Uniform2f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasHeight))
	}
	if loc, ok := uniforms["time"]; ok {
		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	}
	if loc, ok := uniforms["mouse"]; ok {
		gl.Uniform2f(loc.Location, float32(state.CanvasWidth)*0.5, float32(state.CanvasHeight)*0.5)
	}
	if loc, ok := uniforms["surfaceSize"]; ok {
		gl.Uniform2f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasHeight))
	}
	if loc, ok := uniforms["backbuffer"]; ok {
		gl.BindTexture(gl.TEXTURE_2D, state.PreviousFrameTexID)
		gl.ActiveTexture(gl.TEXTURE0)
		gl.Uniform1i(loc.Location, 0)
	}
}

type ShaderToy struct{}

func (ShaderToy) Sources(sources map[uint32][]string) map[uint32][]string {
	sources[gl.VERTEX_SHADER] = append(sources[gl.VERTEX_SHADER], `
		attribute vec3 vert;
		void main(void) {
			gl_Position = vec4(vert, 1.0);
		}
	`)

	glueHead := `
		uniform vec3 iResolution;
		uniform float iTime;
		uniform float iTimeDelta;
		uniform float iFrame;
		uniform float iChannelTime[4];
		uniform vec4 iMouse;
		uniform vec4 iDate;
		uniform float iSampleRate;
		uniform vec3 iChannelResolution[4];
		// TODO: uniform samplerXX iChanneli;
	`
	mainShader := sources[gl.FRAGMENT_SHADER][0]
	glueTail := `
		void main(void) {
			mainImage(gl_FragColor, gl_FragCoord.xy);
		}
	`

	sources[gl.FRAGMENT_SHADER] = []string{glueHead + mainShader + glueTail}
	return sources
}

func (ShaderToy) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	if loc, ok := uniforms["iResolution"]; ok {
		gl.Uniform3f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasHeight), 0.0)
	}
	if loc, ok := uniforms["iTime"]; ok {
		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	}
}

func DetectEnvironment(shaderSource string) (glsl.Environment, bool) {
	// Quick and dirty: run some regular expressions on the source to infer the
	// environment.
	reGLSLSandbox := regexp.MustCompile("uniform\\s+vec2\\s+resolution")
	if reGLSLSandbox.MatchString(shaderSource) {
		return GLSLSandbox{}, true
	}
	// The mainImage function should always be present in ShaderToy image
	// shaders.
	reShaderToy := regexp.MustCompile("void\\s+mainImage\\s*\\(\\s*out\\s+vec4\\s+\\w+\\s*,\\s*in\\s+vec2\\s+\\w+\\s*\\)")
	if reShaderToy.MatchString(shaderSource) {
		return ShaderToy{}, true
	}
	return nil, false
}
