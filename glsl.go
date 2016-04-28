package glsl

import (
	"image"
	"sync"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.1/glfw"
)

const vertexShader = `
	attribute vec3 vert;
	void main(void) {
		gl_Position = vec4(vert, 1.0);
	}
`

var glfwInitOnce sync.Once

type Shader struct {
	win             *glfw.Window
	uniforms        map[string]Uniform
	program, canvas uint32
	fbo, rbo        uint32
	vertLoc         uint32
}

func NewShader(fragmentShader string) (*Shader, error) {
	var err error
	glfwInitOnce.Do(func() {
		err = glfw.Init()
	})
	if err != nil {
		return nil, err
	}

	glfw.WindowHint(glfw.Visible, 0)
	glfw.WindowHint(glfw.RedBits, 8)
	glfw.WindowHint(glfw.GreenBits, 8)
	glfw.WindowHint(glfw.BlueBits, 8)
	glfw.WindowHint(glfw.AlphaBits, 8)
	glfw.WindowHint(glfw.DoubleBuffer, 0)
	win, err := glfw.CreateWindow(1<<12, 1<<12, "glsl", nil, nil)
	if err != nil {
		return nil, err
	}
	sh := &Shader{win: win}
	sh.win.MakeContextCurrent()

	// Initialize OpenGL
	if err := gl.Init(); err != nil {
		return nil, err
	}

	// Set up the render target.
	gl.GenFramebuffers(1, &sh.fbo)
	gl.GenRenderbuffers(1, &sh.rbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, sh.fbo)
	gl.BindRenderbuffer(gl.RENDERBUFFER, sh.rbo)
	gl.FramebufferRenderbuffer(gl.DRAW_FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, sh.rbo)
	gl.ReadBuffer(gl.COLOR_ATTACHMENT0)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)

	// Create the canvas.
	vertices := []float32{
		-1.0, -1.0, 0.0,
		1.0, -1.0, 0.0,
		-1.0, 1.0, 0.0,
		1.0, 1.0, 0.0,
	}
	gl.CreateBuffers(1, &sh.canvas)
	gl.BindBuffer(gl.ARRAY_BUFFER, sh.canvas)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(&vertices[0]), gl.STATIC_DRAW)

	// Set up the shader.
	sh.program, err = linkProgram(map[uint32]string{
		gl.VERTEX_SHADER:   vertexShader,
		gl.FRAGMENT_SHADER: fragmentShader,
	})
	if err != nil {
		return nil, err
	}
	gl.UseProgram(sh.program)
	sh.vertLoc = uint32(gl.GetAttribLocation(sh.program, gl.Str("vert\x00")))
	gl.EnableVertexAttribArray(sh.vertLoc)
	gl.VertexAttribPointer(sh.vertLoc, 3, gl.FLOAT, false, 0, nil)
	sh.uniforms = ListUniforms(sh.program)
	return sh, glError()
}

func (sh *Shader) Image(w, h uint, uniformValues map[string]func(int32)) image.Image {
	if uniformValues == nil {
		uniformValues = map[string]func(int32){}
	}

	if _, ok := uniformValues["resolution"]; !ok {
		uniformValues["resolution"] = func(loc int32) {
			gl.Uniform2f(loc, float32(w), float32(h))
		}
	}

	for name, setValue := range uniformValues {
		if u, ok := sh.uniforms[name]; ok {
			setValue(u.Location)
		}
	}

	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.RGB565, int32(w), int32(h))
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	// Read the framebuffer into an image.
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(&img.Pix[0]))
	return img
}

func (sh *Shader) Animate(w, h uint, interval time.Duration, stream chan<- image.Image, cancel <-chan struct{}, uniformValues map[string]func(int32)) {
	if uniformValues == nil {
		uniformValues = map[string]func(int32){}
	}

	var t time.Duration
	for {
		uniformValues["time"] = func(loc int32) {
			gl.Uniform1f(loc, float32(t)/float32(time.Second))
		}
		select {
		case <-cancel:
			return
		case stream <- sh.Image(w, h, uniformValues):
		}
		t += interval
	}
}

func (sh *Shader) Close() error {
	gl.DeleteProgram(sh.program)
	gl.DeleteBuffers(1, &sh.canvas)
	gl.DeleteFramebuffers(1, &sh.fbo)
	gl.DeleteRenderbuffers(1, &sh.rbo)
	gl.ReadBuffer(gl.BACK)
	sh.win.Destroy()
	return nil
}
