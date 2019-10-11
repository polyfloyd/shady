package image

import (
	"fmt"
	"image"
	"image/draw"
	"math/rand"
	"os"

	"github.com/go-gl/gl/v3.3-core/gl"

	"github.com/polyfloyd/shady/renderer"
	"github.com/polyfloyd/shady/shadertoy"
)

func init() {
	shadertoy.RegisterResourceType("builtin", func(m shadertoy.Mapping, genTexID shadertoy.GenTexFunc, _ renderer.RenderState) (shadertoy.Resource, error) {
		switch m.Value {
		case "Back Buffer":
			r := &backBufferImage{
				uniformName: m.Name,
				index:       genTexID(),
			}
			return r, nil
		case "RGBA Noise Small": // 64x64 4channels uint8
			r := newImageTexture(noise(image.Rect(0, 0, 64, 64)), m.Name, genTexID())
			return r, nil
		case "RGBA Noise Medium": // 256x256 4channels uint8
			r := newImageTexture(noise(image.Rect(0, 0, 256, 256)), m.Name, genTexID())
			return r, nil
		default:
			return nil, fmt.Errorf("unknown builtin mapping %q", m.Value)
		}
	})
	shadertoy.RegisterResourceType("image", func(m shadertoy.Mapping, genTexID shadertoy.GenTexFunc, _ renderer.RenderState) (shadertoy.Resource, error) {
		path, err := shadertoy.ResolvePath(m.PWD, m.Value)
		if err != nil {
			return nil, err
		}
		fd, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer fd.Close()
		img, _, err := image.Decode(fd)
		if err != nil {
			return nil, err
		}
		r := newImageTexture(img, m.Name, genTexID())
		return r, nil
	})
}

// imageTexture is a mapping of a static image texture.
type imageTexture struct {
	uniformName string
	id          uint32
	index       uint32
	rect        image.Rectangle
}

func newImageTexture(img image.Image, uniformName string, texID uint32) *imageTexture {
	tex := &imageTexture{
		uniformName: uniformName,
		index:       texID,
		rect:        img.Bounds(),
	}
	gl.GenTextures(1, &tex.id)
	gl.BindTexture(gl.TEXTURE_2D, tex.id)

	var rgbaImg *image.RGBA
	if i, ok := img.(*image.RGBA); ok {
		rgbaImg = i
	} else {
		rgbaImg = image.NewRGBA(img.Bounds())
		draw.Draw(rgbaImg, img.Bounds(), img, image.Point{X: 0, Y: 0}, draw.Over)
	}

	gl.TexImage2D(
		gl.TEXTURE_2D,            // target
		0,                        // level
		gl.RGBA,                  // internalFormat
		int32(img.Bounds().Dx()), // width
		int32(img.Bounds().Dy()), // height
		0,                        // border
		gl.RGBA,                  // format
		gl.UNSIGNED_BYTE,         // type
		gl.Ptr(rgbaImg.Pix),      // data
	)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.BindTexture(gl.TEXTURE_2D, 0)
	return tex
}

func (tex *imageTexture) UniformSource() string {
	return fmt.Sprintf(`
		uniform sampler2D %s;
		uniform vec3 %sSize;
	`, tex.uniformName, tex.uniformName)
}

func (tex *imageTexture) PreRender(state renderer.RenderState) {
	if loc, ok := state.Uniforms[tex.uniformName]; ok {
		gl.ActiveTexture(gl.TEXTURE0 + tex.index)
		gl.BindTexture(gl.TEXTURE_2D, tex.id)
		gl.Uniform1i(loc.Location, int32(tex.index))
	}
	if m := shadertoy.IchannelNumRe.FindStringSubmatch(tex.uniformName); m != nil {
		if loc, ok := state.Uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(tex.rect.Dx()), float32(tex.rect.Dy()), 1.0)
		}
	}
	if loc, ok := state.Uniforms[fmt.Sprintf("%sSize", tex.uniformName)]; ok {
		gl.Uniform3f(loc.Location, float32(tex.rect.Dx()), float32(tex.rect.Dy()), 1.0)
	}
}

func (tex *imageTexture) Close() error {
	gl.DeleteTextures(1, &tex.id)
	return nil
}

func noise(rect image.Rectangle) image.Image {
	img := image.NewRGBA(rect)
	rng := rand.New(rand.NewSource(1337))
	rng.Read(img.Pix)
	return img
}

type backBufferImage struct {
	uniformName string
	index       uint32
}

func (tex *backBufferImage) UniformSource() string {
	return fmt.Sprintf(`
		uniform sampler2D %s;
		uniform vec3 %sSize;
	`, tex.uniformName, tex.uniformName)
}

func (tex *backBufferImage) PreRender(state renderer.RenderState) {
	if loc, ok := state.Uniforms[tex.uniformName]; ok {
		gl.ActiveTexture(gl.TEXTURE0 + tex.index)
		gl.BindTexture(gl.TEXTURE_2D, state.PreviousFrameTexID())
		gl.Uniform1i(loc.Location, int32(tex.index))
	}
	if m := shadertoy.IchannelNumRe.FindStringSubmatch(tex.uniformName); m != nil {
		if loc, ok := state.Uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasHeight), 1.0)
		}
	}
	if loc, ok := state.Uniforms[fmt.Sprintf("%sSize", tex.uniformName)]; ok {
		gl.Uniform3f(loc.Location, float32(state.CanvasWidth), float32(state.CanvasHeight), 1.0)
	}
}

func (tex *backBufferImage) Close() error { return nil }
