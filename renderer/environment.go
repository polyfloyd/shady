package renderer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
)

type Stage string

const (
	StageVertex   Stage = "vert"
	StageFragment Stage = "frag"
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

// Source represents a single source file.
type Source interface {
	// Contents reads the contents of the source file.
	Contents() ([]byte, error)
	// Dir returns the parent directory the file is located in.
	Dir() string
}

// SourceBuf is an implementation of the Source interface that keeps its
// contents in memory.
type SourceBuf string

// Contents implemetns the Source interface.
func (s SourceBuf) Contents() ([]byte, error) {
	return []byte(s), nil
}

// Dir implemetns the Source interface.
//
// A Sourcebuf has no parent directory, so the current working directory is
// returned instead.
func (s SourceBuf) Dir() string {
	return "."
}

// SourceFile is an implementation of the Source interface for real files.
type SourceFile struct {
	Filename string
}

func SourceFiles(filenames ...string) []SourceFile {
	sources := make([]SourceFile, len(filenames))
	for i, f := range filenames {
		sources[i] = SourceFile{Filename: f}
	}
	return sources
}

// Contents implemetns the Source interface.
func (s SourceFile) Contents() ([]byte, error) {
	fd, err := os.Open(s.Filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	return ioutil.ReadAll(fd)
}

// Dir implemetns the Source interface.
func (s SourceFile) Dir() string {
	return filepath.Dir(s.Filename)
}

type Environment interface {
	// Sources should return the shader sources mapped by their pipeline stage.
	// Multiple shader sources are combined per stage.
	Sources() (map[Stage][]Source, error)

	// Setup may be used to initialize any OpenGL state before the first frame
	// is rendered.
	Setup(state RenderState) error

	// SubEnvironments returns a set of environments which render output is
	// required in this environment.
	//
	// The implementing environment is not required to retain any state of the
	// environments returned.
	SubEnvironments() (map[string]SubEnvironment, error)

	// PreRender updates the program's uniform values for each next frame.
	//
	// sinceStart is the animation time elapsed since the first frame was
	// rendered.
	PreRender(state RenderState)

	// Close should shut down the environment by freeing all associated
	// (OpenGL) resources.
	Close() error
}

type SubEnvironment struct {
	Environment
	Width, Height uint
}

type RenderState struct {
	Time            time.Duration
	Interval        time.Duration
	FramesProcessed uint64

	CanvasWidth  uint
	CanvasHeight uint

	Uniforms           map[string]Uniform
	PreviousFrameTexID func() uint32

	// SubBuffers contains the render output for each environment returned by
	// SubEnvironments as a textureID.
	SubBuffers map[string]uint32
}
