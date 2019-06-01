package shadertoy

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/tarm/serial"

	"github.com/polyfloyd/shady/renderer"
)

func init() {
	resourceBuilders["perip_mat4"] = func(m Mapping, texIndexEnum *uint32, state renderer.RenderState) (resource, error) {
		return newMat4Peripheral(m.Name, m.PWD, m.Value)
	}
}

var (
	periphFile   = regexp.MustCompile(`^([^;]+)(\??)$`)
	periphSerial = regexp.MustCompile(`^([^;]+);(\d+)(\??)$`)
)

type periphMat4 struct {
	uniformName        string
	currentValue       [16]float32
	currentValueLock   sync.Mutex
	closed, loopClosed chan struct{}
}

func newMat4Peripheral(uniformName, pwd, value string) (resource, error) {
	var reader io.ReadCloser
	var err error
	var failSilent bool
	if match := periphFile.FindStringSubmatch(value); match != nil {
		failSilent = match[2] != ""
		var fd *os.File
		fd, err = os.Open(resolvePath(pwd, match[1]))
		if err == nil {
			reader = fd
		}

	} else if match := periphSerial.FindStringSubmatch(value); match != nil {
		failSilent = match[3] != ""
		baudrate, _ := strconv.Atoi(match[2])
		var ser *serial.Port
		ser, err = serial.OpenPort(&serial.Config{
			Name:        match[1],
			Baud:        baudrate,
			ReadTimeout: time.Second * 10,
		})
		if err == nil {
			reader = ser
		} else if ser == nil {
			err = fmt.Errorf("Serial device %q not found", match[1])
		}
	}
	if err != nil && !failSilent {
		return nil, err
	}

	pr := &periphMat4{
		uniformName: uniformName,
		currentValue: [16]float32{
			1, 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		},
	}
	if reader == nil {
		return pr, nil
	}

	pr.closed = make(chan struct{})
	pr.loopClosed = make(chan struct{})
	go func() {
		defer reader.Close()
		defer close(pr.loopClosed)
		br := bufio.NewReader(reader)
		for {
			select {
			case <-pr.closed:
				break
			default:
			}

			line, err := br.ReadBytes('\n')
			if err != nil {
				break
			}
			spl := strings.Split(string(line), " ")
			if len(spl) != 17 || spl[0] != "mat4" {
				continue
			}
			var out [16]float32
			for i, v := range spl[1:] {
				f, _ := strconv.ParseFloat(v, 32)
				out[i] = float32(f)
			}
			pr.currentValueLock.Lock()
			pr.currentValue = out
			pr.currentValueLock.Unlock()
		}
	}()

	return pr, nil
}

func (pr *periphMat4) UniformSource() string {
	return fmt.Sprintf("uniform mat4 %s;", pr.uniformName)
}

func (pr *periphMat4) PreRender(state renderer.RenderState) {
	if loc, ok := state.Uniforms[pr.uniformName]; ok {
		pr.currentValueLock.Lock()
		gl.UniformMatrix4fv(loc.Location, 1, false, &pr.currentValue[0])
		pr.currentValueLock.Unlock()
	}
}

func (pr *periphMat4) Close() error {
	if pr.closed == nil {
		return nil
	}
	close(pr.closed)
	<-pr.loopClosed
	return nil
}
