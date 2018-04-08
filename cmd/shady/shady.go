package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/polyfloyd/shady"
	"github.com/polyfloyd/shady/encode"
	"github.com/polyfloyd/shady/glslsandbox"
	"github.com/polyfloyd/shady/shadertoy"
)

func main() {
	log.SetOutput(os.Stderr)

	formatNames := make([]string, 0, len(encode.Formats))
	for name := range encode.Formats {
		formatNames = append(formatNames, name)
	}

	var inputFiles arrayFlags
	flag.Var(&inputFiles, "i", "The shader file(s) to use")
	outputFile := flag.String("o", "-", "The file to write the rendered image to")
	geometry := flag.String("g", "env", "The geometry of the rendered image in WIDTHxHEIGHT format. If \"env\", look for the LEDCAT_GEOMETRY variable")
	envName := flag.String("env", "", "The environment (aka website) to simulate. Valid values are \"glslsandbox\", \"shadertoy\" or \"\" to autodetect")
	outputFormat := flag.String("ofmt", "", "The encoding format to use to output the image. Valid values are: "+strings.Join(formatNames, ", "))
	framerate := flag.Float64("framerate", 0, "Whether to animate using the specified number of frames per second")
	numFrames := flag.Uint("numframes", 0, "Limit the number of frames in the animation. No limit is set by default")
	duration := flag.Float64("duration", 0.0, "Limit the animation to the specified number of seconds. No limit is set by default")
	realtime := flag.Bool("rt", false, "Render at the actual number of frames per second set by -framerate")
	verbose := flag.Bool("v", false, "Show verbose output about rendering")
	var shadertoyMappings arrayFlags
	flag.Var(&shadertoyMappings, "map", "Specify or override ShaderToy input mappings")
	flag.Parse()

	if *duration != 0.0 && *numFrames != 0 {
		log.Fatalf("-duration and -numframes are mutually exclusive")
	}
	var animateNumFrames uint
	if *numFrames != 0 {
		if *framerate == 0 {
			log.Fatalf("-numframes is set while -framerate is not set")
		}
		animateNumFrames = *numFrames
	}
	if *duration != 0.0 {
		if *framerate == 0 {
			log.Fatalf("-duration is set while -framerate is not set")
		}
		animateNumFrames = uint(*duration * *framerate)
	}
	if *realtime && *framerate == 0 {
		log.Fatalf("-rt is set while -framerate is not set")
	}

	// Figure out the dimensions of the display.
	width, height, err := parseGeometry(*geometry)
	if err != nil {
		log.Fatalf("%v", err)
	}

	var format encode.Format
	var ok bool
	if *outputFormat == "" {
		if format, ok = encode.DetectFormat(*outputFile); !ok {
			log.Fatalf("Unable to detect output format. Please set the -ofmt flag")
		}
	} else if format, ok = encode.Formats[*outputFormat]; !ok {
		log.Fatalf("Unknown output format: %q", *outputFile)
	}

	// Load the shader sources.
	sources, err := glsl.Includes([]string(inputFiles)...)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	// Lock this goroutine to the current thread. This is required because
	// OpenGL contexts are bounds to threads.
	runtime.LockOSThread()

	if *envName == "" {
		for i := 0; *envName == "" && i < len(sources); i++ {
			src, err := sources[i].Contents()
			if err != nil {
				log.Fatal(err)
			}
			*envName = glsl.DetectEnvironment(string(src))
		}
		if *envName == "" {
			log.Fatalf("Unable to detect the environment to use. Please set it using -env")
		}
	}

	var env glsl.Environment
	switch *envName {
	case "glslsandbox":
		ss := make([]glsl.Source, 0, len(sources))
		for _, s := range sources {
			ss = append(ss, s)
		}
		env = glslsandbox.GLSLSandbox{ShaderSources: ss}
	case "shadertoy":
		mappings := make([]shadertoy.Mapping, 0, len(shadertoyMappings))
		for _, str := range shadertoyMappings {
			m, err := shadertoy.ParseMapping(str)
			if err != nil {
				log.Fatalf("%v", err)
			}
			mappings = append(mappings, m)
		}
		env = &shadertoy.ShaderToy{
			ShaderSources: sources,
			ResolveDir:    filepath.Dir(inputFiles[len(inputFiles)-1]),
			Mappings:      mappings,
		}
	default:
		log.Fatalf("Unknown environment: %q", *envName)
	}

	// Compile the shader.
	sh, err := glsl.NewShader(width, height, env)
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer sh.Close()

	// Open the output.
	outWriter, err := openWriter(*outputFile)
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer outWriter.Close()

	if *framerate <= 0 {
		img := sh.Image()
		// We're not dealing with an animation, just export a single image.
		if err := format.Encode(outWriter, img); err != nil {
			log.Fatalf("%v", err)
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
		if err := format.EncodeAnimation(outWriter, counterStream, interval); err != nil {
			log.Printf("Error animating: %v", err)
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
		lastFrame := time.Now()
		var frame uint
		for img := range imgStream {
			renderTime := time.Since(lastFrame)
			fps := 1.0 / (float64(renderTime) / float64(time.Second))
			speed := float64(interval) / float64(renderTime)
			lastFrame = time.Now()
			frame++

			if *verbose {
				var frameTarget string
				if animateNumFrames == 0 {
					frameTarget = "∞"
				} else {
					frameTarget = fmt.Sprintf("%d", animateNumFrames)
				}
				fmt.Fprintf(os.Stderr, "\rfps=%.2f frames=%d/%s speed=%.2f", fps, frame, frameTarget, speed)
			}

			if *realtime {
				time.Sleep(interval - renderTime)
			}
			counterStream <- img
			if frame == animateNumFrames {
				cancel()
				break
			}
		}
		if *verbose {
			fmt.Fprintf(os.Stderr, "\n")
		}
		waitgroup.Done()
	}()
	sh.Animate(ctx, interval, imgStream)
	close(imgStream)
	waitgroup.Wait()
}

func parseGeometry(geom string) (uint, uint, error) {
	if geom == "env" {
		geom = os.Getenv("LEDCAT_GEOMETRY")
		if geom == "" {
			return 0, 0, fmt.Errorf("LEDCAT_GEOMETRY is empty while instructed to load the display geometry from the environment")
		}
	}

	re := regexp.MustCompile("^(\\d+)x(\\d+)$")
	matches := re.FindStringSubmatch(geom)
	if matches == nil {
		return 0, 0, fmt.Errorf("invalid geometry: %q", geom)
	}
	w, _ := strconv.ParseUint(matches[1], 10, 32)
	h, _ := strconv.ParseUint(matches[2], 10, 32)
	if w == 0 || h == 0 {
		return 0, 0, fmt.Errorf("no geometry dimension can be 0, got (%d, %d)", w, h)
	}
	return uint(w), uint(h), nil
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

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "more of the same"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var myFlags arrayFlags
