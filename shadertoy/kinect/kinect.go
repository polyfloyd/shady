package kinect

// #cgo pkg-config: libfreenect
// #include "libfreenect.h"
//
// void rgbCallbackGo(freenect_device *dev, void *rgb, uint32_t timestamp);
// void depthCallbackGo(freenect_device *dev, void *depth, uint32_t timestamp);
//
// void rgb_cb_cgo(freenect_device *dev, void *rgb, uint32_t timestamp) {
//   rgbCallbackGo(dev, rgb, timestamp);
// }
//
// void depth_cb_cgo(freenect_device *dev, void *depth, uint32_t timestamp) {
//   depthCallbackGo(dev, depth, timestamp);
// }
//
// void init_callbacks_cgo(freenect_device *dev) {
//   freenect_set_depth_callback(dev, depth_cb_cgo);
//   freenect_set_video_callback(dev, rgb_cb_cgo);
// }
import "C"

import (
	"fmt"
	"image"
	"log"
	"math"
	"reflect"
	"regexp"
	"sync"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"

	"github.com/polyfloyd/shady"
)

const format = C.FREENECT_VIDEO_RGB

var (
	resolution = image.Rect(0, 0, 640, 480)
	gamma      [2048]uint8
)

// TODO: duplicate
var ichannelNumRe = regexp.MustCompile(`^iChannel(\d+)$`)

func init() {
	for i := range gamma {
		a := float64(i) / float64(len(gamma))
		b := math.Pow(a, 3) * 6
		gamma[i] = 255 - uint8(b*256)
	}
}

type Kinect struct {
	ctx *C.freenect_context
	dev *C.freenect_device

	closed, loopClosed chan struct{}

	currentImage     *image.RGBA
	currentImageLock sync.Mutex

	uniformName  string
	textureIndex uint32
	textureID    uint32
}

func Open(uniformName string, textureIndex uint32) (*Kinect, error) {
	kin := &Kinect{
		closed:       make(chan struct{}),
		loopClosed:   make(chan struct{}),
		currentImage: image.NewRGBA(resolution),
		uniformName:  uniformName,
		textureIndex: textureIndex,
	}

	if C.freenect_init(&kin.ctx, C.NULL) < 0 {
		return nil, fmt.Errorf("freenect_init() failed")
	}

	C.freenect_set_log_level(kin.ctx, C.FREENECT_LOG_DEBUG)
	C.freenect_select_subdevices(kin.ctx, (C.freenect_device_flags)(C.FREENECT_DEVICE_MOTOR|C.FREENECT_DEVICE_CAMERA))

	nr_devices := C.freenect_num_devices(kin.ctx)
	log.Printf("Number of devices found: %d\n", nr_devices)

	if nr_devices < 1 {
		C.freenect_shutdown(kin.ctx)
		return nil, fmt.Errorf("no Kinect devices found")
	}

	deviceNum := 0
	if C.freenect_open_device(kin.ctx, &kin.dev, C.int(deviceNum)) < 0 {
		C.freenect_shutdown(kin.ctx)
		return nil, fmt.Errorf("could not open Kinect device")
	}

	gl.GenTextures(1, &kin.textureID)
	gl.BindTexture(gl.TEXTURE_2D, kin.textureID)

	gl.TexImage2D(
		gl.TEXTURE_2D,          // target
		0,                      // level
		gl.RGBA,                // internalFormat
		int32(resolution.Dx()), // width
		int32(resolution.Dy()), // height
		0,                            // border
		gl.RGBA,                      // format
		gl.UNSIGNED_BYTE,             // type
		gl.Ptr(kin.currentImage.Pix), // data
	)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)

	if instance != nil {
		return nil, fmt.Errorf("only one Kinect can be enabled at a time")
	}
	instance = kin

	go kin.freenectLoop()

	return kin, nil
}

func (kin *Kinect) Close() error {
	close(kin.closed)
	<-kin.loopClosed
	return nil
}

func (kin *Kinect) UniformSource() string {
	return fmt.Sprintf(`
		uniform sampler2D %s;
		uniform vec3 %sSize;
		uniform float %sCurTime;
	`, kin.uniformName, kin.uniformName, kin.uniformName)
}

func (kin *Kinect) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	kin.currentImageLock.Lock()
	defer kin.currentImageLock.Unlock()

	if loc, ok := uniforms[kin.uniformName]; ok {
		gl.ActiveTexture(gl.TEXTURE0 + kin.textureIndex)
		gl.BindTexture(gl.TEXTURE_2D, kin.textureID)
		gl.TexSubImage2D(
			gl.TEXTURE_2D, // target,
			0,             // level,
			0,             // xoffset,
			0,             // yoffset,
			int32(resolution.Dx()),       // width,
			int32(resolution.Dy()),       // height,
			gl.RGBA,                      // format,
			gl.UNSIGNED_BYTE,             // type,
			gl.Ptr(kin.currentImage.Pix), // data
		)
		gl.Uniform1i(loc.Location, int32(kin.textureIndex))
	}
	if m := ichannelNumRe.FindStringSubmatch(kin.uniformName); m != nil {
		if loc, ok := uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(resolution.Dx()), float32(resolution.Dy()), 1.0)
		}
	}
	if loc, ok := uniforms[fmt.Sprintf("%sSize", kin.uniformName)]; ok {
		gl.Uniform3f(loc.Location, float32(resolution.Dx()), float32(resolution.Dy()), 1.0)
	}
	//	if m := ichannelNumRe.FindStringSubmatch(kin.uniformName); m != nil {
	//		if loc, ok := uniforms[fmt.Sprintf("iChannelTime[%s]", m[1])]; ok {
	//			gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	//		}
	//	}
	//	if loc, ok := uniforms[fmt.Sprintf("%sCurTime", kin.uniformName)]; ok {
	//		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	//	}
}

func (kin *Kinect) freenectLoop() {
	tiltAngle := 15.

	C.freenect_set_tilt_degs(kin.dev, C.double(tiltAngle))
	C.freenect_set_led(kin.dev, C.LED_GREEN)
	C.init_callbacks_cgo(kin.dev)
	C.freenect_set_video_mode(kin.dev, C.freenect_find_video_mode(C.FREENECT_RESOLUTION_MEDIUM, format))
	C.freenect_set_depth_mode(kin.dev, C.freenect_find_depth_mode(C.FREENECT_RESOLUTION_MEDIUM, C.FREENECT_DEPTH_11BIT))
	C.freenect_start_depth(kin.dev)
	C.freenect_start_video(kin.dev)

outer:
	for {
		select {
		case <-kin.closed:
			break outer
		default:
		}

		if C.freenect_process_events(kin.ctx) < 0 {
			log.Printf("error processing freenect events")
			break outer
		}
	}

	C.freenect_stop_depth(kin.dev)
	C.freenect_stop_video(kin.dev)
	C.freenect_close_device(kin.dev)
	C.freenect_shutdown(kin.ctx)

	close(kin.loopClosed)
}

func (kin *Kinect) rgbCallback(rgbPtr uintptr) {
	kin.currentImageLock.Lock()
	defer kin.currentImageLock.Unlock()

	length := resolution.Dx() * resolution.Dy()
	rgb := *(*[]uint8)((unsafe.Pointer)(&reflect.SliceHeader{
		Data: rgbPtr,
		Len:  length * 3,
		Cap:  length * 3,
	}))

	for i := 0; i < length; i++ {
		kin.currentImage.Pix[i*4] = rgb[i*3]
		kin.currentImage.Pix[i*4+1] = rgb[i*3+1]
		kin.currentImage.Pix[i*4+2] = rgb[i*3+2]
	}
}

func (kin *Kinect) depthCallback(depthPtr uintptr) {
	kin.currentImageLock.Lock()
	defer kin.currentImageLock.Unlock()

	length := resolution.Dx() * resolution.Dy()
	depth := *(*[]uint16)((unsafe.Pointer)(&reflect.SliceHeader{
		Data: depthPtr,
		Len:  length,
		Cap:  length,
	}))

	for i, value := range depth {
		kin.currentImage.Pix[i*4+3] = gamma[value]
	}
}
