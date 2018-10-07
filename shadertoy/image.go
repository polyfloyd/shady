package shadertoy

import (
	"fmt"
	"image"
	"image/draw"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/polyfloyd/shady"
)

func init() {
	resourceBuilders["builtin"] = func(m Mapping, pwd string, texIndexEnum *uint32) (resource, error) {
		switch m.Value {
		case "RGBA Noise Small": // 64x64 4channels uint8
			r, err := newImageTexture(noise(image.Rect(0, 0, 64, 64)), m.Name, *texIndexEnum)
			*texIndexEnum++
			return r, err
		case "RGBA Noise Medium": // 256x256 4channels uint8
			r, err := newImageTexture(noise(image.Rect(0, 0, 256, 256)), m.Name, *texIndexEnum)
			*texIndexEnum++
			return r, err
		default:
			return nil, fmt.Errorf("unknown builtin mapping %q", m.Value)
		}
	}
	resourceBuilders["image"] = func(m Mapping, pwd string, texIndexEnum *uint32) (resource, error) {
		fd, err := os.Open(resolvePath(pwd, m.Value))
		if err != nil {
			return nil, err
		}
		defer fd.Close()
		img, _, err := image.Decode(fd)
		if err != nil {
			return nil, err
		}
		r, err := newImageTexture(img, m.Name, *texIndexEnum)
		*texIndexEnum++
		return r, err
	}
}

// imageTexture is a mapping of a static image texture.
type imageTexture struct {
	uniformName string
	id          uint32
	index       uint32
	rect        image.Rectangle
}

func newImageTexture(img image.Image, uniformName string, texIndex uint32) (*imageTexture, error) {
	tex := &imageTexture{
		uniformName: uniformName,
		index:       texIndex,
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

func (tex *imageTexture) UniformSource() string {
	return fmt.Sprintf(`
		uniform sampler2D %s;
		uniform vec3 %sSize;
	`, tex.uniformName, tex.uniformName)
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
	if loc, ok := uniforms[fmt.Sprintf("%sSize", tex.uniformName)]; ok {
		gl.Uniform3f(loc.Location, float32(tex.rect.Dx()), float32(tex.rect.Dy()), 1.0)
	}
}

func noise(rect image.Rectangle) image.Image {
	img := image.NewRGBA(rect)
	rng := rand.New(rand.NewSource(1337))
	rng.Read(img.Pix)
	return img
}

func resolvePath(pwd, path string) string {
	if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
		return path
	}
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
