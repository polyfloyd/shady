package egl

// #cgo pkg-config: egl
// #include <EGL/egl.h>
import "C"
import (
	"fmt"
	"strings"
)

var DefaultDisplay = NativeDisplayType{v: C.EGL_DEFAULT_DISPLAY}

type NativeDisplayType struct {
	v C.EGLNativeDisplayType
}

type API struct {
	v C.EGLenum
}

var (
	OpenGLAPI   = API{v: C.EGL_OPENGL_API}
	OpenGLESAPI = API{v: C.EGL_OPENGL_ES_API}
)

type Surface struct {
	conf C.EGLConfig
	surf C.EGLSurface
}

type Display struct {
	dpy C.EGLDisplay
}

func GetDisplay(dtype NativeDisplayType) (Display, error) {
	dpy := C.eglGetDisplay(dtype.v)
	if C.eglInitialize(dpy, nil, nil) == C.EGL_FALSE {
		return Display{}, fmt.Errorf("error initializing display: %w", getError())
	}
	return Display{dpy: dpy}, nil
}

// ClientAPIs retrieves a list of supported client APIs.
func (d Display) ClientAPIs() []string {
	str := C.GoString(C.eglQueryString(d.dpy, C.EGL_CLIENT_APIS))
	return strings.Split(strings.Trim(str, " "), " ")
}

// Extensions retrieves a list of supported extensions.
func (d Display) Extensions() []string {
	str := C.GoString(C.eglQueryString(C.EGLDisplay(d.dpy), C.EGL_EXTENSIONS))
	return strings.Split(strings.Trim(str, " "), " ")
}

// Vendor retrieves the EGL vendor string.
func (d Display) Vendor() string {
	return C.GoString(C.eglQueryString(C.EGLDisplay(d.dpy), C.EGL_VENDOR))
}

// Version retrieves the EGL version string.
func (d Display) Version() string {
	return C.GoString(C.eglQueryString(C.EGLDisplay(d.dpy), C.EGL_VERSION))
}

func (d Display) Destroy() {
	C.eglTerminate(d.dpy)
}

func (d Display) CreateSurface(width, height uint) (*Surface, error) {
	configAttribs := []C.EGLint{
		C.EGL_SURFACE_TYPE, C.EGL_PBUFFER_BIT,
		C.EGL_BLUE_SIZE, 8,
		C.EGL_GREEN_SIZE, 8,
		C.EGL_RED_SIZE, 8,
		C.EGL_RENDERABLE_TYPE, C.EGL_OPENGL_BIT,
		C.EGL_NONE,
	}
	pbufferAttribs := []C.EGLint{
		C.EGL_WIDTH, C.EGLint(width),
		C.EGL_HEIGHT, C.EGLint(height),
		C.EGL_NONE,
	}
	var numConfigs C.EGLint
	var eglCfg C.EGLConfig
	if C.eglChooseConfig(d.dpy, &configAttribs[0], &eglCfg, 1, &numConfigs) == C.EGL_FALSE {
		return nil, fmt.Errorf("failed to call eglChooseConfig")
	}

	eglSurf := C.eglCreatePbufferSurface(d.dpy, eglCfg, &pbufferAttribs[0])
	if eglSurf == nil {
		return nil, fmt.Errorf("failed to call eglCreatePbufferSurface")
	}
	return &Surface{
		conf: eglCfg,
		surf: eglSurf,
	}, nil
}

func (d Display) BindAPI(api API) error {
	if C.eglBindAPI(api.v) == C.EGL_FALSE {
		return fmt.Errorf("failed to call eglBindAPI")
	}
	return nil
}

func (d Display) CreateContext(surface *Surface, openGLMajor, openGLMinor int) (*Context, error) {
	attribs := []C.EGLint{
		C.EGL_CONTEXT_MAJOR_VERSION, C.EGLint(openGLMajor),
		C.EGL_CONTEXT_MINOR_VERSION, C.EGLint(openGLMinor),
		C.EGL_NONE,
	}
	context := C.eglCreateContext(d.dpy, surface.conf, nil, &attribs[0])
	if context == nil {
		return nil, fmt.Errorf("failed to call eglCreateContext")
	}
	return &Context{
		Display: d,
		Surface: surface,
		context: context,
	}, nil
}

type Context struct {
	Display Display
	Surface *Surface

	context C.EGLContext
}

func (cx Context) MakeCurrent() {
	C.eglMakeCurrent(cx.Display.dpy, cx.Surface.surf, cx.Surface.surf, cx.context)
}

func getError() error {
	switch code := C.eglGetError(); code {
	case C.EGL_NOT_INITIALIZED:
		return fmt.Errorf("egl is not initialized, or could not be initialized, for the specified EGL display connection")
	case C.EGL_BAD_ACCESS:
		return fmt.Errorf("egl cannot access a requested resource (for example a context is bound in another thread)")
	case C.EGL_BAD_ALLOC:
		return fmt.Errorf("egl failed to allocate resources for the requested operation")
	case C.EGL_BAD_ATTRIBUTE:
		return fmt.Errorf("an unrecognized attribute or attribute value was passed in the attribute list")
	case C.EGL_BAD_CONTEXT:
		return fmt.Errorf("an EGLContext argument does not name a valid EGL rendering context")
	case C.EGL_BAD_CONFIG:
		return fmt.Errorf("an EGLConfig argument does not name a valid EGL frame buffer configuration")
	case C.EGL_BAD_CURRENT_SURFACE:
		return fmt.Errorf("the current surface of the calling thread is a window, pixel buffer or pixmap that is no longer valid")
	case C.EGL_BAD_DISPLAY:
		return fmt.Errorf("an EGLDisplay argument does not name a valid EGL display connection")
	case C.EGL_BAD_SURFACE:
		return fmt.Errorf("an EGLSurface argument does not name a valid surface (window, pixel buffer or pixmap) configured for GL rendering")
	case C.EGL_BAD_MATCH:
		return fmt.Errorf("arguments are inconsistent (for example, a valid context requires buffers not supplied by a valid surface)")
	case C.EGL_BAD_PARAMETER:
		return fmt.Errorf("one or more argument values are invalid")
	case C.EGL_BAD_NATIVE_PIXMAP:
		return fmt.Errorf("a NativePixmapType argument does not refer to a valid native pixmap")
	case C.EGL_BAD_NATIVE_WINDOW:
		return fmt.Errorf("a NativeWindowType argument does not refer to a valid native window")
	case C.EGL_CONTEXT_LOST:
		return fmt.Errorf("a power management event has occurred. The application must destroy all contexts and reinitialise OpenGL ES state and objects to continue rendering")
	case C.EGL_SUCCESS:
		return nil
	default:
		return fmt.Errorf("unknown EGL error: %v", code)
	}
}
