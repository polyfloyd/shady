package main

import (
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"runtime"
	"time"

	"github.com/polyfloyd/glsl"
)

func main() {
	inputFile := flag.String("i", "-", "The shader file to use. Will read from stdin by default")
	outputFile := flag.String("o", "-", "The file to write the rendered image to")
	width := flag.Uint("w", 512, "The width of the rendered image")
	height := flag.Uint("h", 512, "The height of the rendered image")
	outputFormat := flag.String("ofmt", "", "The encoding format to use to output the image")
	framerate := flag.Float64("framerate", 0, "Whether to animate using the specified number of frames per second")
	numFrames := flag.Uint("numframes", 0, "Limit the number of frames in the animation. No limit is set by default")
	flag.Parse()

	if *numFrames != 0 && *framerate == 0 {
		PrintError(fmt.Errorf("The numframes is set while the framerate is not set"))
		return
	}

	runtime.LockOSThread()

	// Detect the output format.
	format := *outputFormat
	if format == "" {
		format = DetectFormat(*outputFile)
	}
	if format == "" {
		PrintError(fmt.Errorf("Unable to detect output format. Please set the -ofmt flag"))
		return
	}

	// Load the shader.
	shaderSourceFile, err := OpenReader(*inputFile)
	if err != nil {
		PrintError(err)
		return
	}
	defer shaderSourceFile.Close()
	shaderSource, err := ioutil.ReadAll(shaderSourceFile)
	if err != nil {
		PrintError(err)
		return
	}

	// Compile the shader.
	sh, err := glsl.NewShader(*width, *height, string(shaderSource))
	if err != nil {
		PrintError(err)
		return
	}
	defer sh.Close()
	img := sh.Image(nil)

	// Open the output.
	outWriter, err := OpenWriter(*outputFile)
	if err != nil {
		PrintError(err)
		return
	}
	defer outWriter.Close()

	if *framerate <= 0 {
		// We're not dealing with an animation, just export a single image.
		if err := Export(outWriter, img, format); err != nil {
			PrintError(err)
			return
		}
		return
	}

	animStream := make(chan image.Image, int(*framerate)+1)
	cancelAnim := make(chan struct{})
	defer close(animStream)
	defer close(cancelAnim)
	go func() {
		sig := make(chan os.Signal, 1)
		defer close(sig)
		signal.Notify(sig)
		<-sig
		cancelAnim <- struct{}{}
	}()
	go func() {
		for frame := uint(1); ; frame++ {
			img := <-animStream
			if img == nil {
				break
			}
			if err := Export(outWriter, img, format); err != nil {
				cancelAnim <- struct{}{}
				PrintError(err)
				break
			}
			if frame == *numFrames {
				cancelAnim <- struct{}{}
				break
			}
		}
	}()
	sh.Animate(time.Duration(float64(time.Second) / *framerate), animStream, cancelAnim, nil)
}

func OpenReader(filename string) (io.ReadCloser, error) {
	if filename == "-" {
		return ioutil.NopCloser(os.Stdin), nil
	}
	return os.Open(filename)
}

func OpenWriter(filename string) (io.WriteCloser, error) {
	if filename == "-" {
		return nopCloseWriter{Writer: os.Stdout}, nil
	}
	return os.Create(filename)
}

type nopCloseWriter struct {
	io.Writer
}

func (nopCloseWriter) Close() error {
	return nil
}

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
	default:
		return ""
	}
}

func Export(writer io.Writer, img image.Image, format string) error {
	switch format {
	case "png":
		return png.Encode(writer, img)
	case "jpg":
		return jpeg.Encode(writer, img, nil)
	}
	return fmt.Errorf("Unknown output format: %q", format)
}

func PrintError(err error) {
	fmt.Fprintln(os.Stderr, err)
}
