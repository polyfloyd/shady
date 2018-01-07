package main

import (
	"context"
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
		printError(fmt.Errorf("The numframes is set while the framerate is not set"))
		return
	}

	runtime.LockOSThread()

	// Detect the output format.
	format := *outputFormat
	if format == "" {
		format = glsl.DetectFormat(*outputFile)
	}
	if format == "" {
		printError(fmt.Errorf("Unable to detect output format. Please set the -ofmt flag"))
		return
	}

	// Load the shader.
	shaderSourceFile, err := openReader(*inputFile)
	if err != nil {
		printError(err)
		return
	}
	defer shaderSourceFile.Close()
	shaderSource, err := ioutil.ReadAll(shaderSourceFile)
	if err != nil {
		printError(err)
		return
	}
	// Compile the shader.
	sh, err := glsl.NewShader(*width, *height, string(shaderSource))
	if err != nil {
		printError(err)
		return
	}
	defer sh.Close()

	// Open the output.
	outWriter, err := openWriter(*outputFile)
	if err != nil {
		printError(err)
		return
	}
	defer outWriter.Close()

	if *framerate <= 0 {
		img := sh.Image(nil)
		// We're not dealing with an animation, just export a single image.
		if err := glsl.Encode(outWriter, img, format); err != nil {
			printError(err)
			return
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	interval := time.Duration(float64(time.Second) / *framerate)
	imgStream := make(chan image.Image, int(*framerate)+1)
	counterStream := make(chan image.Image)
	var waitgroup sync.WaitGroup
	waitgroup.Add(2)
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		signal.Stop(sig)
		cancel()
	}()
	go func() {
		if err := glsl.EncodeAnim(outWriter, counterStream, format, interval); err != nil {
			printError(fmt.Errorf("Error animating: %v", err))
			cancel()
			go func() {
				// Prevent deadlocking the counter routine.
				for range counterStream {
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
				cancel()
				break
			}
		}
		waitgroup.Done()
	}()
	sh.Animate(ctx, interval, imgStream, nil)
	close(imgStream)
	waitgroup.Wait()
}

func openReader(filename string) (io.ReadCloser, error) {
	if filename == "-" {
		return ioutil.NopCloser(os.Stdin), nil
	}
	return os.Open(filename)
}

func openWriter(filename string) (io.WriteCloser, error) {
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

func printError(err error) {
	fmt.Fprintln(os.Stderr, err)
}
