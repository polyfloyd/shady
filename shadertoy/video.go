package shadertoy

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady"
)

type videoTexture struct {
	uniformName string
	id          uint32
	index       uint32

	resolution    image.Rectangle
	frameInterval time.Duration
	stream        <-chan interface{}
}

func newVideoTexture(uniformName, filename string) (*videoTexture, error) {
	resolution, interval, stream, err := decodeVideoFile(filename)
	if err != nil {
		return nil, err
	}

	vt := &videoTexture{
		uniformName: uniformName,
		index:       texIndexEnum,

		resolution:    resolution,
		frameInterval: interval,
		stream:        stream,
	}
	texIndexEnum++
	gl.GenTextures(1, &vt.id)
	gl.BindTexture(gl.TEXTURE_2D, vt.id)

	initialData := make([]byte, resolution.Dx()*resolution.Dy()*3)
	gl.TexImage2D(
		gl.TEXTURE_2D,          // target
		0,                      // level
		gl.RGBA,                // internalFormat
		int32(resolution.Dx()), // width
		int32(resolution.Dy()), // height
		0,                      // border
		gl.RGB,                 // format
		gl.UNSIGNED_BYTE,       // type
		gl.Ptr(initialData[:]), // data
	)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	return vt, nil
}

func (vt *videoTexture) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	var frame []byte
	select {
	case val := <-vt.stream:
		switch t := val.(type) {
		case error:
			return // TODO: Maybe do something with the error?
		case []byte:
			frame = t
		default:
			panic(fmt.Sprintf("unreachable (%#v)", val))
		}
	default:
		return
	}

	if loc, ok := uniforms[vt.uniformName]; ok {
		gl.BindTexture(gl.TEXTURE_2D, vt.id)
		gl.TexSubImage2D(
			gl.TEXTURE_2D, // target,
			0,             // level,
			0,             // xoffset,
			0,             // yoffset,
			int32(vt.resolution.Dx()), // width,
			int32(vt.resolution.Dy()), // height,
			gl.RGB,           // format,
			gl.UNSIGNED_BYTE, // type,
			gl.Ptr(frame),    // data
		)
		gl.ActiveTexture(gl.TEXTURE0 + vt.index)
		gl.Uniform1i(loc.Location, int32(vt.index))
	}
	if m := ichannelNumRe.FindStringSubmatch(vt.uniformName); m != nil {
		if loc, ok := uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(vt.resolution.Dx()), float32(vt.resolution.Dy()), 1.0)
		}
	}
	if m := ichannelNumRe.FindStringSubmatch(vt.uniformName); m != nil {
		if loc, ok := uniforms[fmt.Sprintf("iChannelTime[%s]", m[1])]; ok {
			gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
		}
	}
}

type videoSource interface {
	SampleRate() float32

	ReadSamples(period time.Duration) []float64
}

func decodeVideoFile(filename string) (image.Rectangle, time.Duration, <-chan interface{}, error) {
	info, err := ffprobe(filename)
	if err != nil {
		return image.Rectangle{}, 0, nil, err
	}
	streamIndex, err := info.FirstStreamByType("video")
	if err != nil {
		return image.Rectangle{}, 0, nil, err
	}
	videoInfo := &info.Streams[streamIndex]
	resolution := image.Rect(0, 0, videoInfo.Width, videoInfo.Heigth)

	s := strings.Split(videoInfo.AvgFrameRate, "/")
	nu, _ := strconv.Atoi(s[0])
	de, _ := strconv.Atoi(s[1])
	interval := time.Second / time.Duration(float64(nu)/float64(de))

	out := make(chan interface{}, 24)
	go func() {
		cmd := exec.Command(
			"ffmpeg",
			"-i", filename,
			"-f", "rawvideo",
			"-pix_fmt", "rgb24",
			"-",
		)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			out <- err
			return
		}
		if err := cmd.Start(); err != nil {
			out <- err
			return
		}

		timer := time.NewTimer(interval)
		for {
			start := time.Now()
			imgBuf := make([]byte, resolution.Dx()*resolution.Dy()*3)
			if _, err := io.ReadFull(stdout, imgBuf); err != nil {
				if err != io.EOF {
					out <- err
				}
				break
			}
			select {
			case out <- imgBuf:
				time.Sleep(interval - time.Since(start))
			case <-timer.C:
				timer.Reset(interval)
			}
		}

		if err := cmd.Wait(); err != nil {
			out <- err
			return
		}
	}()
	return resolution, interval, out, nil
}

type mediaInfo struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		AvgFrameRate string `json:"avg_frame_rate"`
		Width        int    `json:"width"`
		Heigth       int    `json:"height"`
	} `json:"streams"`
}

func ffprobe(filename string) (*mediaInfo, error) {
	cmd := exec.Command(
		"ffprobe", filename,
		"-print_format", "json",
		"-show_format", "-show_streams",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("unable to get media info: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("unable to get media info: %v", err)
	}

	var data mediaInfo
	if err := json.NewDecoder(stdout).Decode(&data); err != nil {
		return nil, fmt.Errorf("unable to get media info: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("unable to get media info: %v", err)
	}
	return &data, nil
}

func (inf *mediaInfo) FirstStreamByType(typ string) (int, error) {
	for i, stream := range inf.Streams {
		if stream.CodecType == typ {
			return i, nil
		}
	}
	return -1, fmt.Errorf("no stream with type %q found", typ)
}
