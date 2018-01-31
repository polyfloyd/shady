package glsl

import (
	"time"
)

type RenderState struct {
	Time time.Duration

	CanvasWidth  uint
	CanvasHeight uint

	PreviousFrameTexID uint32
}

type Environment interface {
	// Sources may inspect and modify the supplied shader sources to match the
	// simulated environment.
	Sources(sources map[uint32][]string) map[uint32][]string

	// PreRender updates the program's uniform values for the next frame.
	//
	// sinceStart is the animation time elapsed since the first frame was
	// rendered.
	PreRender(uniforms map[string]Uniform, state RenderState)
}
