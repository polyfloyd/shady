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
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/polyfloyd/shady"
	"github.com/polyfloyd/shady/encode"
	"github.com/polyfloyd/shady/glslsandbox"
	"github.com/polyfloyd/shady/shadertoy"
)

func main() {
	log.SetOutput(os.Stderr)
	// Lock this goroutine to the current thread. This is required because
	// OpenGL contexts are bounds to threads.
	runtime.LockOSThread()

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
	watch := flag.Bool("w", false, "Watch the shader source files for changes")
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
	if *framerate <= 0 {
		animateNumFrames = 1
	}
	if *realtime && *framerate == 0 {
		log.Fatalf("-rt is set while -framerate is not set")
	}
	interval := time.Duration(float64(time.Second) / *framerate)

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

	// Open the output.
	outWriter, err := openWriter(*outputFile)
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer outWriter.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		signal.Stop(sig)
		cancel()
	}()

	in := make(chan image.Image, 10)
	out := (<-chan image.Image)(in)
	if animateNumFrames > 0 {
		out = limitNumFramesl(out, animateNumFrames)
	}
	if *realtime {
		out = limitFramerate(out, interval)
	}
	if *verbose {
		out = printStats(out, interval, animateNumFrames)
	}
	go func() {
		if err := format.EncodeAnimation(outWriter, out, interval); err != nil {
			log.Printf("Error animating: %v", err)
		}
		cancel()
	}()

	for ctx.Err() == nil {
		loopCtx, loopCancel := context.WithCancel(ctx)

		sh, watcher, err := func() (*glsl.Shader, *fsnotify.Watcher, error) {
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				return nil, nil, err
			}

			// Load the shader sources.
			sources, err := glsl.Includes([]string(inputFiles)...)
			if err != nil {
				for _, src := range inputFiles {
					watcher.Add(src)
				}
				return nil, watcher, err
			}
			for _, src := range sources {
				watcher.Add(src.Filename)
			}

			if *envName == "" {
				for i := 0; *envName == "" && i < len(sources); i++ {
					src, err := sources[i].Contents()
					if err != nil {
						return nil, watcher, err
					}
					*envName = glsl.DetectEnvironment(string(src))
				}
				if *envName == "" {
					return nil, watcher, fmt.Errorf("Unable to detect the environment to use. Please set it using -env")
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
						return nil, watcher, err
					}
					mappings = append(mappings, m)
				}
				env = &shadertoy.ShaderToy{
					ShaderSources: sources,
					ResolveDir:    filepath.Dir(inputFiles[len(inputFiles)-1]),
					Mappings:      mappings,
				}
			default:
				return nil, watcher, fmt.Errorf("Unknown environment: %q", *envName)
			}

			// Compile the shader.
			sh, err := glsl.NewShader(width, height, env)
			if err != nil {
				return nil, watcher, err
			}
			return sh, watcher, nil
		}()
		if err != nil {
			log.Println(err)
			_ = watch
			if !*watch {
				watcher.Close()
				loopCancel()
				break
			}
			select {
			case <-watcher.Events:
			case err := <-watcher.Errors:
				log.Println(err)
			case <-loopCtx.Done():
				loopCancel()
				break
			}
			watcher.Close()
			loopCancel()
			continue
		}

		if *watch {
			go func() {
				select {
				case <-watcher.Events:
					t := time.NewTimer(time.Millisecond * 20)
				outer:
					for {
						select {
						case <-watcher.Events:
						case <-t.C:
							break outer
						}
					}
					loopCancel()
				case err := <-watcher.Errors:
					log.Println(err)
				case <-loopCtx.Done():
				}
			}()
		}

		sh.Animate(loopCtx, interval, in)
		watcher.Close()
		sh.Close()
		loopCancel()
	}
}

func limitNumFramesl(in <-chan image.Image, desiredTotalNumFrames uint) <-chan image.Image {
	out := make(chan image.Image)
	go func() {
		defer close(out)
		frame := uint(0)
		for img := range in {
			frame++
			out <- img
			if frame >= desiredTotalNumFrames {
				break
			}
		}
	}()
	return out
}

func limitFramerate(in <-chan image.Image, interval time.Duration) <-chan image.Image {
	if interval == 0 {
		return in
	}
	out := make(chan image.Image)
	go func() {
		defer close(out)
		lastFrame := time.Now()
		for img := range in {
			time.Sleep(interval - time.Since(lastFrame))
			lastFrame = time.Now()
			out <- img
		}
	}()
	return out
}

func printStats(in <-chan image.Image, desiredInterval time.Duration, desiredTotalNumFrames uint) <-chan image.Image {
	out := make(chan image.Image)
	go func() {
		defer close(out)
		frame := uint(0)
		lastFrame := time.Now()
		var frameTarget string
		if desiredTotalNumFrames == 0 {
			frameTarget = "∞"
		} else {
			frameTarget = fmt.Sprintf("%d", desiredTotalNumFrames)
		}
		for img := range in {
			renderTime := time.Since(lastFrame)
			fps := 1.0 / (float64(renderTime) / float64(time.Second))
			speed := float64(desiredInterval) / float64(renderTime)
			lastFrame = time.Now()
			frame++
			fmt.Fprintf(os.Stderr, "\rfps=%.2f frames=%d/%s speed=%.2f", fps, frame, frameTarget, speed)

			out <- img
		}
	}()
	fmt.Fprintf(os.Stderr, "\n")
	return out
}

func parseGeometry(geom string) (uint, uint, error) {
	if geom == "env" {
		geom = os.Getenv("LEDCAT_GEOMETRY")
		if geom == "" {
			return 0, 0, fmt.Errorf("LEDCAT_GEOMETRY is empty while instructed to load the display geometry from the environment")
		}
	}

	re := regexp.MustCompile(`^(\d+)x(\d+)$`)
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
