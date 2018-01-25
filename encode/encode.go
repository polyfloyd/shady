package encode

import (
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"time"
)

type PNGFormat struct{}

func (f PNGFormat) Extensions() []string {
	return []string{"png"}
}

func (f PNGFormat) Encode(w io.Writer, img image.Image) error {
	return png.Encode(w, img)
}

func (f PNGFormat) EncodeAnimation(w io.Writer, stream <-chan image.Image, interval time.Duration) error {
	for img := range stream {
		if err := f.Encode(w, img); err != nil {
			return err
		}
	}
	return nil
}

type JPGFormat struct{}

func (f JPGFormat) Extensions() []string {
	return []string{"jpg", "jpeg"}
}

func (f JPGFormat) Encode(w io.Writer, img image.Image) error {
	return jpeg.Encode(w, img, nil)
}

func (f JPGFormat) EncodeAnimation(w io.Writer, stream <-chan image.Image, interval time.Duration) error {
	for img := range stream {
		if err := f.Encode(w, img); err != nil {
			return err
		}
	}
	return nil
}

type RGB24Format struct{}

func (f RGB24Format) Extensions() []string {
	return []string{}
}

func (f RGB24Format) Encode(w io.Writer, img image.Image) error {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if _, err := w.Write([]byte{byte(r / 256), byte(g / 256), byte(b / 256)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f RGB24Format) EncodeAnimation(w io.Writer, stream <-chan image.Image, interval time.Duration) error {
	for img := range stream {
		if err := f.Encode(w, img); err != nil {
			return err
		}
	}
	return nil
}

type RGBA32Format struct{}

func (f RGBA32Format) Extensions() []string {
	return []string{}
}

func (f RGBA32Format) Encode(w io.Writer, img image.Image) error {
	var rgbaImg *image.RGBA
	if i, ok := img.(*image.RGBA); ok {
		rgbaImg = i
	} else {
		rgbaImg = image.NewRGBA(img.Bounds())
		draw.Draw(rgbaImg, img.Bounds(), img, image.Point{X: 0, Y: 0}, draw.Over)
	}
	_, err := w.Write(rgbaImg.Pix)
	return err
}

func (f RGBA32Format) EncodeAnimation(w io.Writer, stream <-chan image.Image, interval time.Duration) error {
	for img := range stream {
		if err := f.Encode(w, img); err != nil {
			return err
		}
	}
	return nil
}

type GIFFormat struct{}

func (f GIFFormat) Extensions() []string {
	return []string{"gif"}
}

func (f GIFFormat) Encode(w io.Writer, img image.Image) error {
	// Forward to the code stream encoder for easy code reuse.
	stream := make(chan image.Image, 1)
	stream <- img
	close(stream)
	return f.EncodeAnimation(w, stream, 0)
}

func (f GIFFormat) EncodeAnimation(w io.Writer, stream <-chan image.Image, interval time.Duration) error {
	gifImg := &gif.GIF{
		Image:           []*image.Paletted{},
		Delay:           []int{},
		LoopCount:       0,
		Disposal:        []byte{},
		BackgroundIndex: 0,
	}
	for img := range stream {
		frame := image.NewPaletted(img.Bounds(), palette.Plan9)
		draw.Draw(frame, img.Bounds(), img, image.Point{X: 0, Y: 0}, draw.Over)
		gifImg.Image = append(gifImg.Image, frame)
		gifImg.Delay = append(gifImg.Delay, int(interval/(time.Second/100)))
		gifImg.Disposal = append(gifImg.Disposal, gif.DisposalBackground)
	}
	return gif.EncodeAll(w, gifImg)
}
