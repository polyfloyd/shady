package glsl

import (
	"context"
	"image"
	"runtime"
	"testing"
	"time"
)

const shaderPlain = `
	void main(void) {
		gl_FragColor = vec4(1.0, 0.0, 0.0, 1.0);
	}
`
const shaderWave = `
	void main(void) {
		gl_FragColor = vec4(cos(gl_FragCoord.x), sin(gl_FragCoord.y), 0.0, 1.0);
	}
`

var sources = map[string]string{
	"plain": shaderPlain,
	"wave":  shaderWave,
}

func BenchmarkCompile(b *testing.B) {
	for name, source := range sources {
		b.Run(name, func(b *testing.B) {
			runtime.LockOSThread()

			for n := 0; n < b.N; n++ {
				shader, err := NewShader(512, 512, source)
				if err != nil {
					b.Log(err)
					b.SkipNow()
				}
				shader.Close()
			}
		})
	}
}

func BenchmarkRenderImage(b *testing.B) {
	for name, source := range sources {
		b.Run(name, func(b *testing.B) {
			runtime.LockOSThread()

			shader, err := NewShader(512, 512, source)
			if err != nil {
				b.Log(err)
				b.SkipNow()
			}
			defer shader.Close()

			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				shader.Image(nil)
			}
		})
	}
}

func BenchmarkRenderAnimation(b *testing.B) {
	for name, source := range sources {
		b.Run(name, func(b *testing.B) {
			runtime.LockOSThread()

			shader, err := NewShader(512, 512, source)
			if err != nil {
				b.Log(err)
				b.SkipNow()
			}
			defer shader.Close()

			ctx, cancel := context.WithCancel(context.Background())
			stream := make(chan image.Image)
			go func() {
				for n := 0; n < b.N; n++ {
					<-stream
				}
				cancel()
			}()
			b.ResetTimer()
			shader.Animate(ctx, time.Millisecond, stream, nil)
			close(stream)
		})
	}
}
