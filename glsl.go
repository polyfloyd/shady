package glsl

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.1/glfw"
)

var glfwInitOnce sync.Once

type Shader struct {
	w, h uint

	win     *glfw.Window
	vertLoc uint32
	canvas  uint32

	uniforms map[string]Uniform
	env      Environment
	renderer renderer
	program  uint32
}

func NewShader(width, height uint, env Environment) (*Shader, error) {
	var err error
	glfwInitOnce.Do(func() {
		err = glfw.Init()
	})
	if err != nil {
		return nil, err
	}

	glfw.WindowHint(glfw.Visible, glfw.False)
	glfw.WindowHint(glfw.RedBits, 8)
	glfw.WindowHint(glfw.GreenBits, 8)
	glfw.WindowHint(glfw.BlueBits, 8)
	glfw.WindowHint(glfw.AlphaBits, 8)
	glfw.WindowHint(glfw.DoubleBuffer, glfw.False)
	win, err := glfw.CreateWindow(1<<12, 1<<12, "glsl", nil, nil)
	if err != nil {
		return nil, err
	}
	sh := &Shader{
		win:      win,
		env:      env,
		renderer: &textureRenderer{w: width, h: height},
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
				fmt.Fprintf(os.Stderr, "OpenGL %s: %s\n", dm.SeverityString(), dm.Message)
			}
		}
	}()

	// Set up the render targets.
	if err := sh.renderer.Setup(); err != nil {
		return nil, err
	}

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

	if err := env.Setup(); err != nil {
		return nil, fmt.Errorf("error setting up environment: %v", err)
	}

	// Set up the shader.
	rawSources := env.Sources()

	sources := map[uint32]string{}
	for stage, source := range rawSources {
		e, err := stage.glEnum()
		if err != nil {
			return nil, err
		}
		sources[e] = strings.Join(source, "\n")
	}
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

func (sh *Shader) Image() image.Image {
	sh.env.PreRender(sh.uniforms, RenderState{
		Time:         0,
		CanvasWidth:  sh.w,
		CanvasHeight: sh.h,
	})
	handle := sh.renderer.Draw()
	return sh.renderer.Image(handle)
}

func (sh *Shader) Animate(ctx context.Context, interval time.Duration, stream chan<- image.Image) {
	var t time.Duration
	var prevImageHandle interface{}
	buffer := make(chan interface{}, sh.renderer.NumBuffers())
	for frame := uint64(0); ; frame++ {
		var prevTexID uint32
		if prevImageHandle != nil {
			prevTexID = sh.renderer.Texture(prevImageHandle)
		}
		sh.env.PreRender(sh.uniforms, RenderState{
			Time:               t,
			Interval:           interval,
			FramesProcessed:    frame,
			CanvasWidth:        sh.w,
			CanvasHeight:       sh.h,
			PreviousFrameTexID: prevTexID,
		})
		t += interval

		handle := sh.renderer.Draw()
		buffer <- handle
		prevImageHandle = handle

		if prevImageHandle != nil {
			sh.renderer.FreeTexture(prevTexID)
		}
		if len(buffer) != cap(buffer) {
			// Give the first renders time to complete.
			continue
		}

		img := sh.renderer.Image(<-buffer)
		select {
		case <-ctx.Done():
			return
		case stream <- &Flip{Image: img}:
		}
	}
}

func (sh *Shader) Close() error {
	gl.DeleteProgram(sh.program)
	gl.DeleteBuffers(1, &sh.canvas)
	defer sh.win.Destroy()
	if err := sh.renderer.Close(); err != nil {
		return err
	}
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

type renderer interface {
	io.Closer
	Setup() error
	NumBuffers() int

	Draw() (handle interface{})
	Image(handle interface{}) image.Image

	Texture(handle interface{}) uint32
	FreeTexture(id uint32)
}

type textureRenderer struct {
	w, h           uint
	curTargetIndex int
	targets        [2]struct {
		texture      uint32
		renderbuffer uint32
		framebuffer  uint32
	}
}

func (tr *textureRenderer) Setup() error {
	for i := range tr.targets {
		t := &tr.targets[i]

		gl.GenTextures(1, &t.texture)
		gl.BindTexture(gl.TEXTURE_2D, t.texture)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(tr.w), int32(tr.h), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)

		gl.GenFramebuffers(1, &t.framebuffer)
		gl.BindFramebuffer(gl.FRAMEBUFFER, t.framebuffer)
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, t.texture, 0)
		gl.GenRenderbuffers(1, &t.renderbuffer)
		gl.BindRenderbuffer(gl.RENDERBUFFER, t.renderbuffer)

		gl.BindTexture(gl.TEXTURE_2D, 0)
		gl.BindRenderbuffer(gl.RENDERBUFFER, 0)
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	}
	return nil
}

func (tr *textureRenderer) NumBuffers() int {
	return len(tr.targets)
}

func (tr *textureRenderer) Image(handle interface{}) image.Image {
	i := handle.(int)
	img := image.NewRGBA(image.Rect(0, 0, int(tr.w), int(tr.h)))
	gl.BindTexture(gl.TEXTURE_2D, tr.targets[i].texture)
	gl.GetTexImage(gl.TEXTURE_2D, 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(&img.Pix[0]))
	gl.BindTexture(gl.TEXTURE_2D, 0)
	return img
}

func (tr *textureRenderer) Draw() interface{} {
	tr.curTargetIndex = (tr.curTargetIndex + 1) % len(tr.targets)
	i := tr.curTargetIndex
	gl.BindFramebuffer(gl.FRAMEBUFFER, tr.targets[i].framebuffer)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	return i
}

func (tr *textureRenderer) Texture(handle interface{}) uint32 {
	return tr.targets[handle.(int)].texture
}

func (tr *textureRenderer) FreeTexture(id uint32) {}

func (tr *textureRenderer) Close() error {
	for _, t := range tr.targets {
		gl.DeleteBuffers(1, &t.framebuffer)
		gl.DeleteBuffers(1, &t.renderbuffer)
		gl.DeleteTextures(1, &t.texture)
	}
	return nil
}
