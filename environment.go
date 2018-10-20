package glsl

import (
	"fmt"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
)

var detectors []func(string) string

func RegisterEnvironmentDetector(det func(string) string) {
	detectors = append(detectors, det)
}

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
	}
	return 0, fmt.Errorf("invalid pipeline stage: %q", stage)
}

type Environment interface {
	// Sources should return the shader sources mapped by their pipeline stage.
	// Multiple shader sources are combined per stage.
	Sources() (map[Stage][]Source, error)

	// Setup may be used to initialize any OpenGL state before the first frame
	// is rendered.
	Setup(state RenderState) error

	// PreRender updates the program's uniform values for each next frame.
	//
	// sinceStart is the animation time elapsed since the first frame was
	// rendered.
	PreRender(state RenderState)

	// Close should shut down the environment by freeing all associated
	// (OpenGL) resources.
	Close() error
}

type RenderState struct {
	Time            time.Duration
	Interval        time.Duration
	FramesProcessed uint64

	CanvasWidth  uint
	CanvasHeight uint

	Uniforms           map[string]Uniform
	PreviousFrameTexID func() uint32
}

func DetectEnvironment(shaderSource string) string {
	for _, det := range detectors {
		if name := det(shaderSource); name != "" {
			return name
		}
	}
	return ""
}
