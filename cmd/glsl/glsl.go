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
	"path"
	"runtime"

	"github.com/polyfloyd/glsl"
)

func main() {
	inputFile := flag.String("i", "-", "The shader file to use. Will read from stdin by default")
	outputFile := flag.String("o", "-", "The file to write the rendered image to")
	width := flag.Uint("w", 512, "The width of the rendered image")
	height := flag.Uint("h", 512, "The height of the rendered image")
	outputFormat := flag.String("ofmt", "", "The encoding format to use to output the image")
	flag.Parse()

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
	sh, err := glsl.NewShader(string(shaderSource))
	if err != nil {
		PrintError(err)
		return
	}
	defer sh.Close()
	img := sh.Image(*width, *height, nil)

	// Open the output.
	outWriter, err := OpenWriter(*outputFile)
	if err != nil {
		PrintError(err)
		return
	}
	defer outWriter.Close()

	// Export the image.
	if err := Export(outWriter, img, format); err != nil {
		PrintError(err)
		return
	}
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
