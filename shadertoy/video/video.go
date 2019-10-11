package video

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"

	"github.com/polyfloyd/shady/renderer"
	"github.com/polyfloyd/shady/shadertoy"
)

func init() {
	shadertoy.RegisterResourceType("video", func(m shadertoy.Mapping, genTexID shadertoy.GenTexFunc, state renderer.RenderState) (shadertoy.Resource, error) {
		path, err := shadertoy.ResolvePath(m.PWD, m.Value)
		if err != nil {
			return nil, err
		}
		r, err := newVideoTexture(m.Name, path, genTexID(), state.Time)
		return r, err
	})
}

type videoTexture struct {
	uniformName string
	id          uint32
	index       uint32

	resolution        image.Rectangle
	frameInterval     time.Duration
	stream            <-chan interface{}
	currentVideoFrame int

	cancel func()
}

func newVideoTexture(uniformName, filename string, texIndex uint32, currentTime time.Duration) (*videoTexture, error) {
	ctx, cancel := context.WithCancel(context.Background())

	resolution, interval, stream, err := decodeVideoFile(ctx, filename, currentTime)
	if err != nil {
		cancel()
		return nil, err
	}

	vt := &videoTexture{
		uniformName: uniformName,
		index:       texIndex,

		resolution:        resolution,
		frameInterval:     interval,
		stream:            stream,
		currentVideoFrame: int(currentTime/interval) - 1,

		cancel: cancel,
	}
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
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	return vt, nil
}

func (vt *videoTexture) UniformSource() string {
	return fmt.Sprintf(`
		uniform sampler2D %s;
		uniform vec3 %sSize;
		uniform float %sCurTime;
	`, vt.uniformName, vt.uniformName, vt.uniformName)
}

func (vt *videoTexture) PreRender(state renderer.RenderState) {
	nextFrameTime := time.Duration(vt.currentVideoFrame+1) * vt.frameInterval
	if state.Time < nextFrameTime {
		return
	}
	vt.currentVideoFrame++

	var frame []byte
	switch val := <-vt.stream; t := val.(type) {
	case error:
		return // TODO: Maybe do something with the error?
	case []byte:
		frame = t
	case nil:
		return // EOF
	default:
		panic(fmt.Sprintf("unreachable (%#v)", val))
	}

	if loc, ok := state.Uniforms[vt.uniformName]; ok {
		gl.ActiveTexture(gl.TEXTURE0 + vt.index)
		gl.BindTexture(gl.TEXTURE_2D, vt.id)
		gl.TexSubImage2D(
			gl.TEXTURE_2D,             // target,
			0,                         // level,
			0,                         // xoffset,
			0,                         // yoffset,
			int32(vt.resolution.Dx()), // width,
			int32(vt.resolution.Dy()), // height,
			gl.RGB,                    // format,
			gl.UNSIGNED_BYTE,          // type,
			gl.Ptr(frame),             // data
		)
		gl.Uniform1i(loc.Location, int32(vt.index))
	}
	if m := shadertoy.IchannelNumRe.FindStringSubmatch(vt.uniformName); m != nil {
		if loc, ok := state.Uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(vt.resolution.Dx()), float32(vt.resolution.Dy()), 1.0)
		}
	}
	if loc, ok := state.Uniforms[fmt.Sprintf("%sSize", vt.uniformName)]; ok {
		gl.Uniform3f(loc.Location, float32(vt.resolution.Dx()), float32(vt.resolution.Dy()), 1.0)
	}
	if m := shadertoy.IchannelNumRe.FindStringSubmatch(vt.uniformName); m != nil {
		if loc, ok := state.Uniforms[fmt.Sprintf("iChannelTime[%s]", m[1])]; ok {
			gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
		}
	}
	if loc, ok := state.Uniforms[fmt.Sprintf("%sCurTime", vt.uniformName)]; ok {
		gl.Uniform1f(loc.Location, float32(state.Time)/float32(time.Second))
	}
}

func (vt *videoTexture) Close() error {
	vt.cancel()
	gl.DeleteTextures(1, &vt.id)
	return nil
}

func decodeVideoFile(ctx context.Context, filename string, currentTime time.Duration) (image.Rectangle, time.Duration, <-chan interface{}, error) {
	info, err := ffprobe(ctx, filename)
	if err != nil {
		return image.Rectangle{}, 0, nil, err
	}
	resolution, err := info.VideoResolution()
	if err != nil {
		return image.Rectangle{}, 0, nil, err
	}
	interval := time.Second
	if iv, err := info.VideoFrameInterval(); err == nil {
		interval = iv
	}

	seekToOffset := time.Duration(0)
	if duration, err := info.Duration(); err == nil {
		seekToOffset = currentTime % duration
	}

	out := make(chan interface{}, 4)
	go func() {
		defer close(out)
		for ctx.Err() == nil {
			cmd := exec.CommandContext(
				ctx,
				"ffmpeg",
				"-i", filename,
				"-ss", fmt.Sprintf("%.2f", seekToOffset.Seconds()),
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

			for {
				imgBuf := make([]byte, resolution.Dx()*resolution.Dy()*3)
				if _, err := io.ReadFull(stdout, imgBuf); err != nil {
					if err != io.EOF {
						out <- err
					}
					break
				}
				out <- imgBuf
			}

			if err := cmd.Wait(); err != nil {
				out <- err
				return
			}

			// Restart the next playback at the beginning.
			seekToOffset = 0
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
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func ffprobe(ctx context.Context, filename string) (*mediaInfo, error) {
	cmd := exec.CommandContext(
		ctx,
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

func (info *mediaInfo) firstStreamByType(typ string) (int, error) {
	for i, stream := range info.Streams {
		if stream.CodecType == typ {
			return i, nil
		}
	}
	return -1, fmt.Errorf("no stream with type %q found", typ)
}

func (info *mediaInfo) Duration() (time.Duration, error) {
	f, err := strconv.ParseFloat(info.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("could not parse video duration: %v", err)
	}
	return time.Duration(f * float64(time.Second)), nil
}

func (info *mediaInfo) VideoFrameInterval() (time.Duration, error) {
	streamIndex, err := info.firstStreamByType("video")
	if err != nil {
		return -1, err
	}
	videoInfo := &info.Streams[streamIndex]

	s := strings.Split(videoInfo.AvgFrameRate, "/")
	nu, err := strconv.Atoi(s[0])
	if err != nil {
		return -1, fmt.Errorf("could not determine video frame interval: %v", err)
	}
	de, err := strconv.Atoi(s[1])
	if err != nil {
		return -1, fmt.Errorf("could not determine video frame interval: %v", err)
	}

	if nu == 0 || de == 0 {
		return -1, fmt.Errorf("could not determine video frame interval")
	}
	return time.Duration(float64(time.Second) / (float64(nu) / float64(de))), nil
}

func (info *mediaInfo) VideoResolution() (image.Rectangle, error) {
	streamIndex, err := info.firstStreamByType("video")
	if err != nil {
		return image.Rectangle{}, err
	}
	videoInfo := &info.Streams[streamIndex]
	return image.Rect(0, 0, videoInfo.Width, videoInfo.Heigth), nil
}
