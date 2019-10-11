package renderer

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady/egl"
)

const (
	OpenGL20 OpenGLVersion = 20
	OpenGL21 OpenGLVersion = 21
	OpenGL30 OpenGLVersion = 30
	OpenGL31 OpenGLVersion = 31
	OpenGL32 OpenGLVersion = 32
	OpenGL33 OpenGLVersion = 33
)

var initGLOnce sync.Once

type Shader struct {
	w, h      uint
	glVersion OpenGLVersion

	vertLoc uint32
	vao     uint32
	vbo     uint32

	uniforms map[string]Uniform
	renderer renderer
	program  uint32

	env     Environment
	newEnvs chan Environment

	subTargets map[string]*Shader

	time            time.Duration
	frame           uint64
	prevFrameHandle interface{}
}

func initGL(glVersion OpenGLVersion) error {
	// Set up a rendering context.
	display, err := egl.GetDisplay(egl.DefaultDisplay)
	if err != nil {
		return err
	}
	surface, err := display.CreateSurface(1<<12, 1<<12)
	if err != nil {
		return err
	}
	if err := display.BindAPI(egl.OpenGLAPI); err != nil {
		return err
	}
	glMajor, glMinor := glVersion.majorMinor()
	glContext, err := display.CreateContext(surface, glMajor, glMinor)
	if err != nil {
		return err
	}
	glContext.MakeCurrent()

	// Initialize OpenGL.
	if err := gl.Init(); err != nil {
		return err
	}

	debug := GLDebugOutput()
	go func() {
		for dm := range debug {
			if dm.Severity != gl.DEBUG_SEVERITY_NOTIFICATION {
				fmt.Fprintf(os.Stderr, "OpenGL %s: %s\n", dm.SeverityString(), dm.Message)
			}
		}
	}()
	return nil
}

func NewShader(width, height uint, glVersion OpenGLVersion) (*Shader, error) {
	// Hack: Unit tests require a different style of initialization. We'll
	// detect whether we are running as a test for now.
	var err error
	if strings.HasSuffix(os.Args[0], ".test") {
		err = initGL(glVersion)
	} else {
		initGLOnce.Do(func() { err = initGL(glVersion) })
	}
	if err != nil {
		return nil, err
	}

	sh := &Shader{
		w:         width,
		h:         height,
		glVersion: glVersion,
		renderer:  &pboRenderer{w: width, h: height},
		newEnvs:   make(chan Environment, 1),
	}

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
	gl.GenVertexArrays(1, &sh.vao)
	gl.BindVertexArray(sh.vao)
	gl.GenBuffers(1, &sh.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, sh.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(&vertices[0]), gl.STATIC_DRAW)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindVertexArray(0)

	return sh, nil
}

// reloadEnvironment ensures that an environment is set and set up for
// rendering.
func (sh *Shader) reloadEnvironment(ctx context.Context) error {
	var env Environment
	if sh.env == nil {
		// If no environment is set, block until it is set or the context is
		// canceled.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case env = <-sh.newEnvs:
		}
	} else {
		// If an environment is already set, check if a newer environment is
		// available or just exit.
		select {
		case env = <-sh.newEnvs:
		default:
			return nil
		}
	}

	// Close the old environment if there is one.
	if sh.env != nil {
		sh.env.Close()
		gl.DeleteProgram(sh.program)
		sh.env = nil
	}
	if env == nil {
		return nil
	}

	renderState := RenderState{
		Time:            sh.time,
		FramesProcessed: sh.frame,
		CanvasWidth:     sh.w,
		CanvasHeight:    sh.h,
		Uniforms:        sh.uniforms,
	}
	if err := env.Setup(renderState); err != nil {
		return fmt.Errorf("error setting up environment: %v", err)
	}

	subEnvs, err := env.SubEnvironments()
	if err != nil {
		return err
	}
	sh.subTargets = map[string]*Shader{}
	for name, env := range subEnvs {
		s, err := NewShader(env.Width, env.Height, sh.glVersion)
		if err != nil {
			return err
		}
		s.SetEnvironment(env.Environment)
		if err := s.reloadEnvironment(context.Background()); err != nil {
			return err
		}
		sh.subTargets[name] = s
	}

	sources, err := env.Sources()
	if err != nil {
		return err
	}
	sh.program, err = linkProgram(sources)
	if err != nil {
		return err
	}
	gl.UseProgram(sh.program)
	sh.uniforms = ListUniforms(sh.program)
	sh.vertLoc = uint32(gl.GetAttribLocation(sh.program, gl.Str("vert\x00")))

	sh.env = env
	return nil
}

func (sh *Shader) SetEnvironment(env Environment) {
	sh.newEnvs <- env
}

func (sh *Shader) nextHandle(interval time.Duration) interface{} {
	if err := sh.reloadEnvironment(context.Background()); err != nil {
		log.Printf("Error reloading environment: %v", err)
		return nil
	}

	prevTexID, freePrevTexID := uint32(0), func() {}
	getPrevTexID := func() uint32 {
		if sh.prevFrameHandle != nil && prevTexID == 0 {
			prevTexID, freePrevTexID = sh.renderer.Texture(sh.prevFrameHandle)
		}
		return prevTexID
	}
	defer freePrevTexID()

	subTextures := map[string]uint32{}
	freeSubTextures := []func(){}
	for name, s := range sh.subTargets {
		h := s.nextHandle(interval)
		textureID, free := s.renderer.Texture(h)
		subTextures[name] = textureID
		freeSubTextures = append(freeSubTextures, free)
	}
	defer func() {
		for _, free := range freeSubTextures {
			free()
		}
	}()

	// Ensure that the render state is up to date.
	gl.BindVertexArray(sh.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, sh.vbo)
	gl.UseProgram(sh.program)
	gl.EnableVertexAttribArray(sh.vertLoc)
	gl.VertexAttribPointer(sh.vertLoc, 3, gl.FLOAT, false, 0, nil)

	sh.env.PreRender(RenderState{
		Time:               sh.time,
		Interval:           interval,
		FramesProcessed:    sh.frame,
		CanvasWidth:        sh.w,
		CanvasHeight:       sh.h,
		Uniforms:           sh.uniforms,
		PreviousFrameTexID: getPrevTexID,
		SubBuffers:         subTextures,
	})
	sh.time += interval
	sh.frame++

	// Render the geometry.
	handle := sh.renderer.Draw(func() {
		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	})
	sh.prevFrameHandle = handle
	return handle
}

func (sh *Shader) Image() image.Image {
	handle := sh.nextHandle(0)
	return sh.renderer.Image(handle)
}

func (sh *Shader) Animate(ctx context.Context, interval time.Duration, stream chan<- image.Image) {
	buffer := make(chan interface{}, sh.renderer.NumBuffers())
	for {
		if err := sh.reloadEnvironment(ctx); err == context.Canceled {
			return
		} else if err != nil {
			log.Printf("Error reloading environment: %v", err)
			continue
		}

		handle := sh.nextHandle(interval)
		buffer <- handle

		if len(buffer) != cap(buffer) {
			// Give the first renders time to complete.
			continue
		}

		img := sh.renderer.Image(<-buffer)
		select {
		case <-ctx.Done():
			return
		case stream <- &flip{Image: img}:
		}
	}
}

func (sh *Shader) Close() error {
	var envErr error
	if sh.env != nil {
		envErr = sh.env.Close()
	}
	for _, s := range sh.subTargets {
		s.Close()
	}
	gl.DeleteProgram(sh.program)
	gl.DeleteVertexArrays(1, &sh.vao)
	gl.DeleteBuffers(1, &sh.vbo)
	if err := sh.renderer.Close(); err != nil {
		return err
	}
	return envErr
}

// flip wraps an image and flips it upside down.
type flip struct {
	image.Image
}

func (flip *flip) At(x, y int) color.Color {
	h := flip.Bounds().Dy()
	return flip.Image.At(x, h-y-1)
}

type renderer interface {
	io.Closer
	Setup() error
	NumBuffers() int

	Draw(func()) (handle interface{})
	Image(handle interface{}) image.Image
	Texture(handle interface{}) (uint32, func())
}

type pboRenderer struct {
	w, h           uint
	curTargetIndex int
	targets        [3]struct {
		pbo, rbo, fbo uint32
	}
}

func (pr *pboRenderer) Setup() error {
	for i := range pr.targets {
		t := &pr.targets[i]
		// Framebuffer.
		gl.GenFramebuffers(1, &t.fbo)
		gl.BindFramebuffer(gl.FRAMEBUFFER, t.fbo)
		// Color renderbuffer.
		gl.GenRenderbuffers(1, &t.rbo)
		gl.BindRenderbuffer(gl.RENDERBUFFER, t.rbo)
		gl.RenderbufferStorage(gl.RENDERBUFFER, gl.RGBA8, int32(pr.w), int32(pr.h))

		gl.FramebufferRenderbuffer(gl.DRAW_FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, t.rbo)
		gl.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
		gl.ReadBuffer(gl.COLOR_ATTACHMENT0)

		// Pixelbuffer
		gl.GenBuffers(1, &t.pbo)
		gl.BindBuffer(gl.PIXEL_PACK_BUFFER, t.pbo)
		gl.BufferData(gl.PIXEL_PACK_BUFFER, int(pr.w*pr.h*4), nil, gl.DYNAMIC_READ)
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	gl.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
	gl.BindRenderbuffer(gl.RENDERBUFFER, 0)
	return nil
}

func (pr *pboRenderer) NumBuffers() int {
	return len(pr.targets)
}

func (pr *pboRenderer) Image(handle interface{}) image.Image {
	i := handle.(int)
	img := image.NewRGBA(image.Rect(0, 0, int(pr.w), int(pr.h)))
	gl.BindBuffer(gl.PIXEL_PACK_BUFFER, pr.targets[i].pbo)
	gl.GetBufferSubData(gl.PIXEL_PACK_BUFFER, 0, int(pr.w*pr.h*4), gl.Ptr(&img.Pix[0]))
	gl.BindBuffer(gl.PIXEL_PACK_BUFFER, 0)
	return img
}

// Draw instructs OpenGL to render a single image with the scene drawn by
// function provided.
// A handle is returned which can be used to access the image data.
func (pr *pboRenderer) Draw(drawFunc func()) interface{} {
	pr.curTargetIndex = (pr.curTargetIndex + 1) % len(pr.targets)
	t := &pr.targets[pr.curTargetIndex]
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.fbo)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	drawFunc()
	// Start the transfer of the image to the PBO.
	gl.BindBuffer(gl.PIXEL_PACK_BUFFER, t.pbo)
	gl.ReadPixels(0, 0, int32(pr.w), int32(pr.h), gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	return pr.curTargetIndex
}

func (pr *pboRenderer) Texture(handle interface{}) (uint32, func()) {
	t := pr.targets[handle.(int)]
	var tex uint32
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(pr.w), int32(pr.h), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)

	gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, t.pbo)
	gl.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, int32(pr.w), int32(pr.h), gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.BindBuffer(gl.PIXEL_UNPACK_BUFFER, 0)
	gl.BindTexture(gl.TEXTURE_2D, 0)
	return tex, func() {
		gl.DeleteTextures(1, &tex)
	}
}

func (pr *pboRenderer) Close() error {
	for _, t := range pr.targets {
		gl.DeleteFramebuffers(1, &t.fbo)
		gl.DeleteRenderbuffers(1, &t.rbo)
		gl.DeleteBuffers(1, &t.pbo)
	}
	return nil
}

type OpenGLVersion int

func ParseOpenGLVersion(s string) (OpenGLVersion, error) {
	re := regexp.MustCompile(`^(\d)\.(\d)$`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid OpenGL version: %q", s)
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	return OpenGLVersion(maj*10 + min), nil
}

func OpenGLVersionFromGLSLVersion(s string) (OpenGLVersion, error) {
	// Parse to int first, this verifies the format of the string.
	glslVersion, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}

	switch s {
	case "110":
		return OpenGL20, nil
	case "120":
		return OpenGL21, nil
	case "130":
		return OpenGL30, nil
	case "140":
		return OpenGL31, nil
	case "150":
		return OpenGL32, nil
	}

	// For all versions of OpenGL 3.3 and above, the corresponding GLSL version
	// matches the OpenGL version. So GL 4.1 uses GLSL 4.10.
	return OpenGLVersion(glslVersion / 10), nil
}

func (v OpenGLVersion) String() string {
	maj, min := v.majorMinor()
	return fmt.Sprintf("%d.%d", maj, min)
}

func (v OpenGLVersion) majorMinor() (int, int) {
	return int(v / 10), int(v % 10)
}
