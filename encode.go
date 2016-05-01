package glsl

import (
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"path"
	"time"
)

func DetectFormat(filename string) string {
	ext := path.Ext(filename)
	if len(ext) == 0 {
		return ""
	}
	switch ext[1:] {
	case "png":
		return "png"
	case "jpg", "jpeg":
		return "jpg"
	case "gif":
		return "gif"
	default:
		return ""
	}
}

func Encode(writer io.Writer, img image.Image, format string) error {
	switch format {
	case "png":
		return png.Encode(writer, img)
	case "jpg":
		return jpeg.Encode(writer, img, nil)
	}
	return fmt.Errorf("Unknown output format: %q", format)
}

func EncodeAnim(writer io.Writer, imgStream <-chan image.Image, format string, interval time.Duration) error {
	switch format {
	case "gif":
		return EncodeGIF(writer, imgStream, interval)
	default:
		for img := range imgStream {
			if err := Encode(writer, img, format); err != nil {
				return err
			}
		}
		return nil
	}
}

func EncodeGIF(writer io.Writer, imgStream <-chan image.Image, interval time.Duration) error {
	gifImg := &gif.GIF{
		Image:           []*image.Paletted{},
		Delay:           []int{},
		LoopCount:       0,
		Disposal:        []byte{},
		BackgroundIndex: 0,
	}
	for img := range imgStream {
		frame := image.NewPaletted(img.Bounds(), palette.Plan9)
		draw.Draw(frame, img.Bounds(), img, image.Point{X: 0, Y: 0}, draw.Over)
		gifImg.Image = append(gifImg.Image, frame)
		gifImg.Delay = append(gifImg.Delay, int(interval/(time.Second/100)))
		gifImg.Disposal = append(gifImg.Disposal, gif.DisposalBackground)
	}
	return gif.EncodeAll(writer, gifImg)
}
