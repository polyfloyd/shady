package main

import (
	"testing"
)

func TestDetectEnvironmentShaderToy(t *testing.T) {
	sources := []string{
		`void mainImage( out vec4 fragColor, in vec2 fragCoord ) { }`,
		`void mainImage( out vec4 fragColor, vec2 fragCoord )`,
		`void mainImage(out vec4 foo,in vec2 bar){}`,
		`void   mainImage  (  out  vec4  o  ,  in   vec2  i  )  {  }`,
	}
	for _, s := range sources {
		env, ok := DetectEnvironment(s)
		if !ok {
			t.Fatalf("unable to detect environment from source: %q", s)
		}
		if _, ok := env.(*ShaderToy); !ok {
			t.Fatalf("detect environment is not ShaderToy for source: %q", s)
		}
	}
}

func TestDetectEnvironmentGLSLSandbox(t *testing.T) {
	sources := []string{
		`uniform vec2 resolution;`,
		`uniform  vec2  resolution;`,
	}
	for _, s := range sources {
		env, ok := DetectEnvironment(s)
		if !ok {
			t.Fatalf("unable to detect environment from source: %q", s)
		}
		if _, ok := env.(GLSLSandbox); !ok {
			t.Fatalf("detect environment is not ShaderToy for source: %q", s)
		}
	}
}
