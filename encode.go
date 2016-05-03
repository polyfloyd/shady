package glsl

import (
	"bytes"
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"path"
	"runtime"
	"sync"
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
	case "rgba32":
		return "rgba32"
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
	case "rgba32":
		return EncodeRGBA32(writer, img)
	}
	return fmt.Errorf("Unknown output format: %q", format)
}

func EncodeRGBA32(writer io.Writer, img image.Image) error {
	var rgbaImg *image.RGBA
	if i, ok := img.(*image.RGBA); ok {
		rgbaImg = i
	} else {
		rgbaImg = image.NewRGBA(img.Bounds())
		draw.Draw(rgbaImg, img.Bounds(), img, image.Point{X: 0, Y: 0}, draw.Over)
	}
	_, err := writer.Write(rgbaImg.Pix)
	return err
}

func EncodeAnim(writer io.Writer, imgStream <-chan image.Image, format string, interval time.Duration) error {
	switch format {
	case "gif":
		return EncodeGIF(writer, imgStream, interval)
	default:
		return EncodeSequence(writer, imgStream, format)
	}
}

func EncodeSequence(writer io.Writer, imgStream <-chan image.Image, format string) error {
	errs := make(chan error)
	encoded := make(chan chan []byte, runtime.NumCPU()) // Channelception.
	defer close(errs)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for out := range encoded {
			if _, err := writer.Write(<-out); err != nil {
				errs <- err
				break
			}
		}
		wg.Done()
	}()

	for {
		select {
		case err := <-errs:
			close(encoded)
			return err
		case img, ok := <-imgStream:
			if !ok {
				close(encoded)
				wg.Wait()
				return nil
			}
			out := make(chan []byte)
			go func(img image.Image, out chan []byte) {
				var buf bytes.Buffer
				if err := Encode(&buf, img, format); err != nil {
					errs <- err
				} else {
					out <- buf.Bytes()
				}
				close(out)
			}(img, out)
			select {
			case err := <-errs:
				close(encoded)
				return err
			case encoded <- out:
			}
		}
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
