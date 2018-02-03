package main

import (
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady"
)

type ShaderToy struct {
	Source string
}

func (st ShaderToy) Sources() map[glsl.Stage][]string {
	return map[glsl.Stage][]string{
		glsl.StageVertex: {`
			attribute vec3 vert;
			void main(void) {
				gl_Position = vec4(vert, 1.0);
			}
		`},
		glsl.StageFragment: {`
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
		`,
			st.Source,
			`
			void main(void) {
				mainImage(gl_FragColor, gl_FragCoord.xy);
			}
		`},
	}
}

func (ShaderToy) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	if loc, ok := uniforms["iResolution"]; ok {
		gl.Uniform3f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasHeight), 0.0)
	}
	if loc, ok := uniforms["iTime"]; ok {
		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	}
}
