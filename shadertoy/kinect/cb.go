package kinect

// #include "libfreenect.h"
import "C"

//export rgbCallbackGo
func rgbCallbackGo(dev *C.freenect_device, rgbPtr uintptr, timestamp C.uint32_t) {
	kin := (*Kinect)(C.freenect_get_user(dev))
	kin.rgbCallback(rgbPtr)
}

//export depthCallbackGo
func depthCallbackGo(dev *C.freenect_device, depthPtr uintptr, timestamp C.uint32_t) {
	kin := (*Kinect)(C.freenect_get_user(dev))
	kin.depthCallback(depthPtr)
}
