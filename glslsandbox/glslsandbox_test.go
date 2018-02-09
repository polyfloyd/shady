package glslsandbox

import (
	"testing"

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
