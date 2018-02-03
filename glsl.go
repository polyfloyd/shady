package glsl

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.1/glfw"
)

var glfwInitOnce sync.Once

type target struct {
	texture      uint32
	renderbuffer uint32
	framebuffer  uint32
}

type Shader struct {
	w, h uint

	win     *glfw.Window
	vertLoc uint32
	canvas  uint32

	targets        [2]target
	curTargetIndex int

	uniforms map[string]Uniform
	env      Environment
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
				fmt.Fprintf(os.Stderr, "OpenGL %s: %s\n", dm.SeverityString(), dm.Message)
			}
		}
	}()

	// Set up the render targets.
	for i := range sh.targets {
		t := &sh.targets[i]

		gl.GenTextures(1, &t.texture)
		gl.BindTexture(gl.TEXTURE_2D, t.texture)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(width), int32(height), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
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

func (sh *Shader) downloadImage(targetIndex int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, int(sh.w), int(sh.h)))
	gl.BindTexture(gl.TEXTURE_2D, sh.targets[targetIndex].texture)
	gl.GetTexImage(gl.TEXTURE_2D, 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(&img.Pix[0]))
	gl.BindTexture(gl.TEXTURE_2D, 0)
	return img
}

func (sh *Shader) drawImage(targetIndex int) {
	gl.BindFramebuffer(gl.FRAMEBUFFER, sh.targets[targetIndex].framebuffer)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
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
		curTarget := int(frame & 1)
		prevTarget := int(frame&1) ^ 1
		sh.env.PreRender(sh.uniforms, RenderState{
			Time:               t,
			Interval:           interval,
			FramesProcessed:    frame,
			CanvasWidth:        sh.w,
			CanvasHeight:       sh.h,
			PreviousFrameTexID: sh.targets[prevTarget].texture,
		})
		t += interval

		sh.drawImage(curTarget)
		if frame == 0 {
			// Give the first render time to complete.
			continue
		}

		img := sh.downloadImage(prevTarget)
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
	for _, t := range sh.targets {
		gl.DeleteBuffers(1, &t.framebuffer)
		gl.DeleteBuffers(1, &t.renderbuffer)
		gl.DeleteTextures(1, &t.texture)
	}
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
