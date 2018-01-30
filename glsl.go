package glsl

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"sync"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.1/glfw"
)

var glfwInitOnce sync.Once

type Shader struct {
	w, h uint

	win              *glfw.Window
	fbo, rbo, canvas uint32
	vertLoc          uint32
	pbos             [3]uint32

	uniforms    map[string]Uniform
	env         Environment
	program     uint32
	curBufIndex int
}

func NewShader(width, height uint, sources map[uint32][]string, env Environment) (*Shader, error) {
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
	sh := &Shader{
		win: win,
		w:   width,
		h:   height,
		env: env,
	}
	sh.win.MakeContextCurrent()

	// Initialize OpenGL
	if err := gl.Init(); err != nil {
		return nil, err
	}

	debug := GLDebugOutput()
	go func() {
		for dm := range debug {
			if dm.Severity != gl.DEBUG_SEVERITY_NOTIFICATION {
				fmt.Fprintf(os.Stderr, "OpenGL %s: %s\n%s\n", dm.SeverityString(), dm.Message, dm.Stack)
			}
		}
	}()

	// Set up the render target.
	// Framebuffer.
	gl.GenFramebuffers(1, &sh.fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, sh.fbo)
	// Color renderbuffer.
	gl.GenRenderbuffers(1, &sh.rbo)
	gl.BindRenderbuffer(gl.RENDERBUFFER, sh.rbo)
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.RGBA8, int32(sh.w), int32(sh.h))

	gl.FramebufferRenderbuffer(gl.DRAW_FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, sh.rbo)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.ReadBuffer(gl.COLOR_ATTACHMENT0)

	gl.GenBuffers(int32(len(sh.pbos)), &sh.pbos[0])
	for _, bufID := range sh.pbos {
		gl.BindBuffer(gl.PIXEL_PACK_BUFFER, bufID)
		gl.BufferData(gl.PIXEL_PACK_BUFFER, int(sh.w*sh.h*4), nil, gl.DYNAMIC_READ)
	}
	gl.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)

	// Create the canvas.
	vertices := []float32{
		-1.0, -1.0, 0.0,
		1.0, -1.0, 0.0,
		-1.0, 1.0, 0.0,
		1.0, 1.0, 0.0,
	}
	gl.GenBuffers(1, &sh.canvas)
	gl.BindBuffer(gl.ARRAY_BUFFER, sh.canvas)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(&vertices[0]), gl.STATIC_DRAW)

	// Set up the shader.
	sources = env.Sources(sources)
	sh.program, err = linkProgram(sources)
	if err != nil {
		return nil, err
	}
	gl.UseProgram(sh.program)
	sh.vertLoc = uint32(gl.GetAttribLocation(sh.program, gl.Str("vert\x00")))
	gl.EnableVertexAttribArray(sh.vertLoc)
	gl.VertexAttribPointer(sh.vertLoc, 3, gl.FLOAT, false, 0, nil)
	sh.uniforms = ListUniforms(sh.program)
	return sh, nil
}

func (sh *Shader) downloadImage(pboIndex int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, int(sh.w), int(sh.h)))
	gl.BindBuffer(gl.PIXEL_PACK_BUFFER, sh.pbos[pboIndex])
	gl.ReadPixels(0, 0, int32(sh.w), int32(sh.h), gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.GetBufferSubData(gl.PIXEL_PACK_BUFFER, 0, int(sh.w*sh.h*4), gl.Ptr(&img.Pix[0]))
	return img
}

func (sh *Shader) drawImage(pboIndex int) {
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	// Start the transfer of the image to the PBO.
	gl.BindBuffer(gl.PIXEL_PACK_BUFFER, sh.pbos[pboIndex])
	gl.ReadPixels(0, 0, int32(sh.w), int32(sh.h), gl.RGBA, gl.UNSIGNED_BYTE, nil)
}

func (sh *Shader) Image() image.Image {
	sh.env.PreRender(sh.uniforms, RenderState{
		Time:         0,
		CanvasWidth:  sh.w,
		CanvasHeight: sh.h,
	})
	sh.drawImage(0)
	return sh.downloadImage(0)
}

func (sh *Shader) Animate(ctx context.Context, interval time.Duration, stream chan<- image.Image) {
	var t time.Duration
	for frame := uint64(0); ; frame++ {
		sh.env.PreRender(sh.uniforms, RenderState{
			Time:         t,
			CanvasWidth:  sh.w,
			CanvasHeight: sh.h,
		})
		t += interval

		sh.drawImage(int(frame % uint64(len(sh.pbos))))
		if frame < uint64(len(sh.pbos)) {
			continue
		}

		img := sh.downloadImage(int((frame - 1) % uint64(len(sh.pbos))))
		select {
		case <-ctx.Done():
			return
		case stream <- &Flip{Image: img}:
		}
	}
}

func (sh *Shader) Close() error {
	gl.DeleteProgram(sh.program)
	gl.DeleteFramebuffers(1, &sh.fbo)
	gl.DeleteRenderbuffers(1, &sh.rbo)
	gl.DeleteBuffers(1, &sh.canvas)
	gl.DeleteBuffers(int32(len(sh.pbos)), &sh.pbos[0])
	sh.win.Destroy()
	return nil
}

// Flip wraps an image and flips it upside down.
type Flip struct {
	image.Image
}

func (flip *Flip) At(x, y int) color.Color {
	h := flip.Bounds().Dy()
	return flip.Image.At(x, h-y-1)
}
