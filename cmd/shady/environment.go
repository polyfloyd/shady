package main

import (
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady"
)

// GLSLSandbox implements the Environment interface to simulate the canvas of
// glslsandbox.com.
type GLSLSandbox struct{}

func (GLSLSandbox) Sources(sources map[uint32][]string) map[uint32][]string {
	return sources
}

func (GLSLSandbox) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	if loc, ok := uniforms["resolution"]; ok {
		gl.Uniform2f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasHeight))
	}
	if loc, ok := uniforms["time"]; ok {
		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	}
}

type ShaderToy struct{}

func (ShaderToy) Sources(sources map[uint32][]string) map[uint32][]string {
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
