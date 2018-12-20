package renderer

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
)

func compileShader(stage Stage, sources ...Source) (uint32, error) {
	glStage, err := stage.glEnum()
	if err != nil {
		return 0, err
	}

	originalSources := make([]string, len(sources))
	src := ""
	for i, s := range sources {
		c, err := s.Contents()
		if err != nil {
			return 0, err
		}
		originalSources[i] = string(c)
		if i != 0 {
			src += fmt.Sprintf("#line 1 %d\n", i)
		}
		src += string(c)
		src += "\n\n"
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
			sources: originalSources,
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
	sources []string

	stage Stage
	log   string
}

func (err CompileError) Error() string {
	var buf bytes.Buffer
	if err.stage == StageVertex {
		fmt.Fprintf(&buf, "Error compiling vertex shader:\n")
	} else if err.stage == StageFragment {
		fmt.Fprintf(&buf, "Error compiling fragment shader:\n")
	}
	err.PrettyPrint(&buf)
	return buf.String()
}

func (err CompileError) PrettyPrint(out io.Writer) {
	markers := err.markers()
	if len(markers) == 0 {
		fmt.Fprintf(out, "%s\n", err.log)
	}

	for _, marker := range markers {
		lines := strings.Split(err.sources[marker.fileno], "\n")
		for i := marker.lineno - 2; i < marker.lineno+2; i++ {
			if 0 <= i && i < len(lines) {
				fmt.Fprintf(out, "%04d: %s\n", i+1, lines[i])
			}
			if i+1 == marker.lineno {
				fmt.Fprintf(out, "      ^ %s\n", marker.message)
			}
		}
	}
}

func (err CompileError) markers() []errorMarker {
	errLineRe := regexp.MustCompile(`(?m)^(\d+):(\d+)\((\d+)\): (.+)$`)

	var markers []errorMarker
	matches := errLineRe.FindAllStringSubmatch(err.log, -1)
	for _, m := range matches {
		fileno, _ := strconv.Atoi(m[1])
		lineno, _ := strconv.Atoi(m[2])
		message := m[4]

		markers = append(markers, errorMarker{
			fileno:  fileno,
			lineno:  lineno,
			message: message,
		})
	}
	return markers
}

type LinkError struct {
	sources map[Stage][]Source

	log string
}

func (err LinkError) Error() (str string) {
	return err.log
}

type errorMarker struct {
	lineno  int
	fileno  int
	message string
}
