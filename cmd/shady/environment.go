package main

import (
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady"
)

// GLSLSandbox implements the Environment interface to simulate the canvas of
// glslsandbox.com.
type GLSLSandbox struct{}

func (GLSLSandbox) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	if loc, ok := uniforms["resolution"]; ok {
		gl.Uniform2f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasWidth))
	}
	if loc, ok := uniforms["time"]; ok {
		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	}
}
