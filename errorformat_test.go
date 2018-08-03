package glsl

import (
	"testing"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady/egl"
)

func initTestGL(t *testing.T) {
	display, err := egl.GetDisplay(egl.DefaultDisplay)
	if err != nil {
		t.Skip()
	}
	surface := display.CreateSurface(1, 1)
	display.BindAPI(egl.OpenGLAPI)
	context := display.CreateContext(surface)
	context.MakeCurrent()
	if err := gl.Init(); err != nil {
		t.Skip()
	}
}

func TestUnknownVar(t *testing.T) {
	initTestGL(t)

	sources := SourceBuf(`
void main() {
	a = 12;
}
	`)

	_, err := compileShader(StageVertex, sources)
	compileError, ok := err.(CompileError)
	if !ok {
		t.Fatalf("expected a CompileError, got %#v", err)
	}

	_ = compileError
}
