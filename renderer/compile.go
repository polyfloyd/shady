package renderer

import (
	"io"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
)

const sourceSeparator = "\n\n"

func compileShader(stage Stage, sources ...Source) (uint32, error) {
	glStage, err := stage.glEnum()
	if err != nil {
		return 0, err
	}

	var src string
	for _, s := range sources {
		c, err := s.Contents()
		if err != nil {
			return 0, err
		}
		src += string(c)
		src += sourceSeparator
	}

	shader := gl.CreateShader(glStage)
	csources, free := gl.Strs(src + "\x00")
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLen)
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetShaderInfoLog(shader, logLen, nil, gl.Str(log))
		gl.DeleteShader(shader)
		return 0, CompileError{
			sources: sources,
			stage:   stage,
			log:     log,
		}
	}
	return shader, nil
}

func linkProgram(sources map[Stage][]Source) (uint32, error) {
	shaders := map[uint32]uint32{}
	freeShaders := func() {
		for _, sh := range shaders {
			gl.DeleteShader(sh)
		}
	}

	for stage, source := range sources {
		sh, err := compileShader(stage, source...)
		if err != nil {
			freeShaders()
			return 0, err
		}
		glStage, err := stage.glEnum()
		if err != nil {
			return 0, err
		}
		shaders[glStage] = sh
	}

	program := gl.CreateProgram()
	for _, sh := range shaders {
		gl.AttachShader(program, sh)
	}
	gl.LinkProgram(program)

	var linkErr error
	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLen)
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetProgramInfoLog(program, logLen, nil, gl.Str(log))
		linkErr = LinkError{log: log}
	}

	for _, sh := range shaders {
		gl.DetachShader(program, sh)
	}
	freeShaders()
	if linkErr != nil {
		gl.DeleteProgram(program)
		return 0, linkErr
	}
	return program, nil
}

type CompileError struct {
	sources []Source

	stage Stage
	log   string
}

func (err CompileError) Error() (str string) {
	if err.stage == StageVertex {
		str += "Error compiling vertex shader:\n"
	} else if err.stage == StageFragment {
		str += "Error compiling fragment shader:\n"
	}
	str += err.log
	return
}

func (err CompileError) PrettyPrint(out io.Writer, color bool) {
}

type LinkError struct {
	sources map[Stage][]Source

	log string
}

func (err LinkError) Error() (str string) {
	return err.log
}
