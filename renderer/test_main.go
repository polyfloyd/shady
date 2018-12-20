package renderer

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
	surface, err := display.CreateSurface(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := display.BindAPI(egl.OpenGLAPI); err != nil {
		t.Fatal(err)
	}
	context, err := display.CreateContext(surface, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	context.MakeCurrent()
	if err := gl.Init(); err != nil {
		t.Skip()
	}
}
