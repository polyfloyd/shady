package shadertoy

import (
	"fmt"
	"image"
	"image/draw"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady"
)

// imageTexture is a mapping of a static image texture.
type imageTexture struct {
	uniformName string
	id          uint32
	index       uint32
	rect        image.Rectangle
}

func newImageTexture(img image.Image, uniformName string) (*imageTexture, error) {
	tex := &imageTexture{
		uniformName: uniformName,
		index:       texIndexEnum,
		rect:        img.Bounds(),
	}
	texIndexEnum++
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
		0,                   // border
		gl.RGBA,             // format
		gl.UNSIGNED_BYTE,    // type
		gl.Ptr(rgbaImg.Pix), // data
	)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.BindTexture(gl.TEXTURE_2D, 0)
	return tex, nil
}

func (tex *imageTexture) PreRender(uniforms map[string]glsl.Uniform, state glsl.RenderState) {
	if loc, ok := uniforms[tex.uniformName]; ok {
		gl.ActiveTexture(gl.TEXTURE0 + tex.index)
		gl.BindTexture(gl.TEXTURE_2D, tex.id)
		gl.Uniform1i(loc.Location, int32(tex.index))
	}
	if m := ichannelNumRe.FindStringSubmatch(tex.uniformName); m != nil {
		if loc, ok := uniforms[fmt.Sprintf("iChannelResolution[%s]", m[1])]; ok {
			gl.Uniform3f(loc.Location, float32(tex.rect.Dx()), float32(tex.rect.Dy()), 1.0)
		}
	}
}

func noise(rect image.Rectangle) image.Image {
	img := image.NewRGBA(rect)
	rng := rand.New(rand.NewSource(1337))
	rng.Read(img.Pix)
	return img
}

func resolvePath(pwd, path string) string {
	if len(path) > 0 && path[0] == '~' {
		home := os.Getenv("HOME")
		if home == "" {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	if !filepath.IsAbs(path) {
		return filepath.Join(pwd, path)
	}
	return path
}
