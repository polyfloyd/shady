package glsl

import (
	"fmt"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
)

type Stage string

const (
	StageVertex   = "vert"
	StageFragment = "frag"
)

func (stage Stage) glEnum() (uint32, error) {
	switch stage {
	case StageVertex:
		return gl.VERTEX_SHADER, nil
	case StageFragment:
		return gl.FRAGMENT_SHADER, nil
	default:
		return 0, fmt.Errorf("invalid pipeline stage: %q")
	}
}

type Environment interface {
	// Sources should return the shader sources mapped by their pipeline stage.
	// Multiple shader sources are combined per stage.
	Sources() map[Stage][]string

	// Setup may be used to initialize any OpenGL state before the first frame
	// is rendered.
	Setup() error

	// PreRender updates the program's uniform values for each next frame.
	//
	// sinceStart is the animation time elapsed since the first frame was
	// rendered.
	PreRender(uniforms map[string]Uniform, state RenderState)
}

type RenderState struct {
	Time time.Duration

	CanvasWidth  uint
	CanvasHeight uint

	PreviousFrameTexID uint32
}
