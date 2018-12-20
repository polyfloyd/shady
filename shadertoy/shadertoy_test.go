package shadertoy

import (
	"testing"

	"github.com/polyfloyd/shady/renderer"
)

func TestDetectEnvironment(t *testing.T) {
	sources := []string{
		`void mainImage( out vec4 fragColor, in vec2 fragCoord ) { }`,
		`void mainImage( out vec4 fragColor, vec2 fragCoord )`,
		`void mainImage(out vec4 foo,in vec2 bar){}`,
		`void   mainImage  (  out  vec4  o  ,  in   vec2  i  )  {  }`,
	}
	for _, s := range sources {
		env := renderer.DetectEnvironment(s)
		if env == "" {
			t.Fatalf("unable to detect environment from source: %q", s)
		}
		if env != "shadertoy" {
			t.Fatalf("detect environment is not ShaderToy for source: %q", s)
		}
	}
}
