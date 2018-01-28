package glsl

import (
	"time"
)

type RenderState struct {
	Time time.Duration

	CanvasWidth  uint
	CanvasHeight uint
}

type Environment interface {
	// PreRender updates the program's uniform values for the next frame.
	//
	// sinceStart is the animation time elapsed since the first frame was
	// rendered.
	PreRender(uniforms map[string]Uniform, state RenderState)
}
