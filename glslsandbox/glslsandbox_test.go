package glslsandbox

import (
	"context"
	"image"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/polyfloyd/shady"
)

func TestDetectEnvironment(t *testing.T) {
	sources := []string{
		`uniform vec2 resolution;`,
		`uniform  vec2  resolution;`,
	}
	for _, s := range sources {
		env := glsl.DetectEnvironment(s)
		if env == "" {
			t.Fatalf("unable to detect environment from source: %q", s)
		}
		if env != "glslsandbox" {
			t.Fatalf("detect environment is not ShaderToy for source: %q", s)
		}
	}
}

func TestOutputSize(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.SkipNow()
	}

	const w, h = 512, 512
	source := `
		void main(void) {
			gl_FragColor = vec4(1.0, 0.0, 0.0, 1.0);
		}
	`
	env := GLSLSandbox{Source: source}

	runtime.LockOSThread()
	shader, err := glsl.NewShader(w, h, env)
	if err != nil {
		t.Fatal(err)
	}
	defer shader.Close()

	img := shader.Image()
	if iw := img.Bounds().Dx(); iw != w {
		t.Fatalf("unexpected image width: %v, expected %v", iw, w)
	}
	if ih := img.Bounds().Dy(); ih != h {
		t.Fatalf("unexpected image height: %v, expected %v", ih, h)
	}
}

func TestColorOrdering(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.SkipNow()
	}

	const w, h = 512, 512
	source := `
		void main(void) {
			if (gl_FragCoord.y <= 1.0) {
				gl_FragColor = vec4(1.0, 0.0, 0.0, 1.0);
			} else if (gl_FragCoord.y <= 2.0) {
				gl_FragColor = vec4(0.0, 1.0, 0.0, 1.0);
			} else if (gl_FragCoord.y <= 3.0) {
				gl_FragColor = vec4(0.0, 0.0, 1.0, 1.0);
			} else {
				gl_FragColor = vec4(0.0, 0.0, 0.0, 1.0);
			}
		}
	`
	env := GLSLSandbox{Source: source}

	runtime.LockOSThread()
	shader, err := glsl.NewShader(w, h, env)
	if err != nil {
		t.Fatal(err)
	}
	defer shader.Close()

	img := shader.Image()
	if iw := img.Bounds().Dx(); iw != w {
		t.Fatalf("unexpected image width: %v, expected %v", iw, w)
	}
	if ih := img.Bounds().Dy(); ih != h {
		t.Fatalf("unexpected image height: %v, expected %v", ih, h)
	}

	if r, g, b, a := img.At(0, 0).RGBA(); r != 0xffff || g != 0 || b != 0 || a != 0xffff {
		t.Fatalf("unexpected red color: (%v, %v, %v, %v)", r, g, b, a)
	}
	if r, g, b, a := img.At(0, 1).RGBA(); r != 0 || g != 0xffff || b != 0 || a != 0xffff {
		t.Fatalf("unexpected green color: (%v, %v, %v, %v)", r, g, b, a)
	}
	if r, g, b, a := img.At(0, 2).RGBA(); r != 0 || g != 0 || b != 0xffff || a != 0xffff {
		t.Fatalf("unexpected blue color: (%v, %v, %v, %v)", r, g, b, a)
	}
	if r, g, b, a := img.At(0, 3).RGBA(); r != 0 || g != 0 || b != 0 || a != 0xffff {
		t.Fatalf("unexpected black color: (%v, %v, %v, %v)", r, g, b, a)
	}
}

func TestAnimationTime(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.SkipNow()
	}

	const w, h = 512, 512
	source := `
		uniform float time;
		void main(void) {
			if (time <= 0.5) {
				gl_FragColor = vec4(1.0, 0.0, 0.0, 1.0);
			} else if (time <= 1.5) {
				gl_FragColor = vec4(0.0, 1.0, 0.0, 1.0);
			} else if (time <= 2.5) {
				gl_FragColor = vec4(0.0, 0.0, 1.0, 1.0);
			} else {
				gl_FragColor = vec4(0.0, 0.0, 0.0, 1.0);
			}
		}
	`
	env := GLSLSandbox{Source: source}

	runtime.LockOSThread()
	shader, err := glsl.NewShader(w, h, env)
	if err != nil {
		t.Fatal(err)
	}
	defer shader.Close()

	images := make([]image.Image, 0, 4)
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan image.Image)
	defer close(out)

	go func() {
		for i := 0; i < cap(images); i++ {
			images = append(images, <-out)
		}
		cancel()
	}()
	shader.Animate(ctx, time.Second, out)

	if r, g, b, a := images[0].At(0, 0).RGBA(); r != 0xffff || g != 0 || b != 0 || a != 0xffff {
		t.Fatalf("unexpected red color: (%v, %v, %v, %v)", r, g, b, a)
	}
	if r, g, b, a := images[1].At(0, 1).RGBA(); r != 0 || g != 0xffff || b != 0 || a != 0xffff {
		t.Fatalf("unexpected green color: (%v, %v, %v, %v)", r, g, b, a)
	}
	if r, g, b, a := images[2].At(0, 2).RGBA(); r != 0 || g != 0 || b != 0xffff || a != 0xffff {
		t.Fatalf("unexpected blue color: (%v, %v, %v, %v)", r, g, b, a)
	}
	if r, g, b, a := images[3].At(0, 3).RGBA(); r != 0 || g != 0 || b != 0 || a != 0xffff {
		t.Fatalf("unexpected black color: (%v, %v, %v, %v)", r, g, b, a)
	}
}

func TestAnimationBackbuffer(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.SkipNow()
	}

	const w, h = 512, 512
	source := `
		uniform sampler2D backbuffer;
		void main(void) {
			vec4 c = texture2D(backbuffer, gl_FragCoord.xy);
			if (c.x == 1.0) {
				gl_FragColor = vec4(0.0, 1.0, 0.0, 1.0);
			} else if (c.y == 1.0) {
				gl_FragColor = vec4(0.0, 0.0, 1.0, 1.0);
			} else if (c.z == 1.0) {
				gl_FragColor = vec4(0.0, 0.0, 0.0, 1.0);
			} else {
				gl_FragColor = vec4(1.0, 0.0, 0.0, 1.0);
			}
		}
	`
	env := GLSLSandbox{Source: source}

	runtime.LockOSThread()
	shader, err := glsl.NewShader(w, h, env)
	if err != nil {
		t.Fatal(err)
	}
	defer shader.Close()

	images := make([]image.Image, 0, 4)
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan image.Image)
	defer close(out)

	go func() {
		for i := 0; i < cap(images); i++ {
			images = append(images, <-out)
		}
		cancel()
	}()
	shader.Animate(ctx, time.Second, out)

	if r, g, b, a := images[0].At(0, 0).RGBA(); r != 0xffff || g != 0 || b != 0 || a != 0xffff {
		t.Fatalf("unexpected red color: (%v, %v, %v, %v)", r, g, b, a)
	}
	if r, g, b, a := images[1].At(0, 1).RGBA(); r != 0 || g != 0xffff || b != 0 || a != 0xffff {
		t.Fatalf("unexpected green color: (%v, %v, %v, %v)", r, g, b, a)
	}
	if r, g, b, a := images[2].At(0, 2).RGBA(); r != 0 || g != 0 || b != 0xffff || a != 0xffff {
		t.Fatalf("unexpected blue color: (%v, %v, %v, %v)", r, g, b, a)
	}
	if r, g, b, a := images[3].At(0, 3).RGBA(); r != 0 || g != 0 || b != 0 || a != 0xffff {
		t.Fatalf("unexpected black color: (%v, %v, %v, %v)", r, g, b, a)
	}
}
