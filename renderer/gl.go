package renderer

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

type GLDebugMessage struct {
	ID       uint32
	Source   uint32
	Type     uint32
	Severity uint32
	Message  string
	Stack    string
}

func (dm GLDebugMessage) SeverityString() string {
	switch dm.Severity {
	case gl.DEBUG_SEVERITY_HIGH:
		return "high"
	case gl.DEBUG_SEVERITY_MEDIUM:
		return "medium"
	case gl.DEBUG_SEVERITY_LOW:
		return "low"
	case gl.DEBUG_SEVERITY_NOTIFICATION:
		return "note"
	default:
		return ""
	}
}

func (dm GLDebugMessage) String() string {
	return fmt.Sprintf("[%s] %s", dm.SeverityString(), dm.Message)
}

func GLDebugOutput() <-chan GLDebugMessage {
	ch := make(chan GLDebugMessage, 32)
	gl.Enable(gl.DEBUG_OUTPUT)
	gl.DebugMessageControl(gl.DONT_CARE, gl.DONT_CARE, gl.DONT_CARE, 0, nil, true)
	gl.DebugMessageCallback(func(source uint32, typ uint32, id uint32, severity uint32, length int32, message string, userParam unsafe.Pointer) {
		dm := GLDebugMessage{
			ID:       id,
			Source:   source,
			Type:     typ,
			Severity: severity,
			Message:  message,
		}
		var stack [8192]byte
		stackLen := runtime.Stack(stack[:], false)
		dm.Stack = string(stack[:stackLen])

		select {
		case ch <- dm:
		default:
		}
	}, nil)
	return ch
}
