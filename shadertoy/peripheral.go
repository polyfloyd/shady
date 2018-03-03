package shadertoy

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady"
	"github.com/tarm/serial"
)

var (
	periphFile   = regexp.MustCompile("^([^;]+)$")
	periphSerial = regexp.MustCompile("^([^;]+);(\\d+)$")
)

type periphMat4 struct {
	uniformName      string
	currentValue     [16]float32
	currentValueLock sync.Mutex
}

func newMat4Peripheral(uniformName, pwd, value string) (resource, error) {
	var reader io.ReadCloser
	if match := periphFile.FindStringSubmatch(value); match != nil {
		fd, err := os.Open(resolvePath(pwd, match[1]))
		if err != nil {
			return nil, err
		}
		reader = fd

	} else if match := periphSerial.FindStringSubmatch(value); match != nil {
		baudrate, _ := strconv.Atoi(match[2])
		ser, err := serial.OpenPort(&serial.Config{
			Name: match[1],
			Baud: baudrate,
		})
		if err != nil {
			return nil, err
		}
		reader = ser
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

	go func() {
		defer reader.Close()
		br := bufio.NewReader(reader)
		for {
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

func (pr *periphMat4) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	if loc, ok := uniforms[pr.uniformName]; ok {
		pr.currentValueLock.Lock()
		gl.UniformMatrix4fv(loc.Location, 1, false, &pr.currentValue[0])
		pr.currentValueLock.Unlock()
	}
}
