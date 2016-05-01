package main

import (
	"flag"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"sync"
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
		format = glsl.DetectFormat(*outputFile)
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

	// Open the output.
	outWriter, err := OpenWriter(*outputFile)
	if err != nil {
		PrintError(err)
		return
	}
	defer outWriter.Close()

	if *framerate <= 0 {
		img := sh.Image(nil)
		// We're not dealing with an animation, just export a single image.
		if err := glsl.Encode(outWriter, img, format); err != nil {
			PrintError(err)
			return
		}
		return
	}

	interval := time.Duration(float64(time.Second) / *framerate)
	imgStream := make(chan image.Image, int(*framerate)+1)
	counterStream := make(chan image.Image)
	cancelAnim := make(chan struct{}, 4)
	defer close(cancelAnim)
	var waitgroup sync.WaitGroup
	waitgroup.Add(2)
	go func() {
		sig := make(chan os.Signal, 16)
		defer close(sig)
		signal.Notify(sig)
		<-sig
		signal.Stop(sig)
		cancelAnim <- struct{}{}
	}()
	go func() {
		if err := glsl.EncodeAnim(outWriter, counterStream, format, interval); err != nil {
			PrintError(fmt.Errorf("Error animating: %v", err))
			cancelAnim <- struct{}{}
			go func() {
				// Prevent deadlocking the counter routine.
				for _ = range counterStream {
				}
			}()
		}
		waitgroup.Done()
	}()
	go func() {
		defer close(counterStream)
		var frame uint
		for img := range imgStream {
			frame++
			counterStream <- img
			if frame == *numFrames {
				cancelAnim <- struct{}{}
				break
			}
		}
		waitgroup.Done()
	}()
	sh.Animate(interval, imgStream, cancelAnim, nil)
	close(imgStream)
	waitgroup.Wait()
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

func PrintError(err error) {
	fmt.Fprintln(os.Stderr, err)
}
