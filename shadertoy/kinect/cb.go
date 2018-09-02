package kinect

// #include "libfreenect.h"
import "C"

var instance *Kinect

//export rgbCallbackGo
func rgbCallbackGo(dev *C.freenect_device, rgbPtr uintptr, timestamp C.uint32_t) {
	instance.rgbCallback(rgbPtr)
}

//export depthCallbackGo
func depthCallbackGo(dev *C.freenect_device, depthPtr uintptr, timestamp C.uint32_t) {
	instance.depthCallback(depthPtr)
}
