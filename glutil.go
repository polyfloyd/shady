package glsl

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

type GLDebugMessage struct {
	ID       uint32
	Source   uint32
	Type     uint32
	Severity uint32
	Message  string
	Stack    string
}

func (dm GLDebugMessage) String() string {
	var sev string
	switch dm.Severity {
	case gl.DEBUG_SEVERITY_HIGH:
		sev = "high"
	case gl.DEBUG_SEVERITY_MEDIUM:
		sev = "medium"
	case gl.DEBUG_SEVERITY_LOW:
		sev = "low"
	case gl.DEBUG_SEVERITY_NOTIFICATION:
		sev = "note"
	}
	return fmt.Sprintf("[%s] %s", sev, dm.Message)
}

func GLDebugOutput(out io.Writer) <-chan GLDebugMessage {
	ch := make(chan GLDebugMessage, 32)
	gl.Enable(gl.DEBUG_OUTPUT)
	gl.DebugMessageControl(gl.DONT_CARE, gl.DONT_CARE, gl.DONT_CARE, 0, nil, true)
	gl.DebugMessageCallback(func(source uint32, typ uint32, id uint32, severity uint32, length int32, message string, userParam unsafe.Pointer) {
		dm := GLDebugMessage{
			ID:       id,
			Source:   source,
			Type:     typ,
			Severity: severity,
			Message:  message,
		}
		var stack [512]byte
		stackLen := runtime.Stack(stack[:], false)
		dm.Stack = string(stack[:stackLen])

		select {
		case ch <- dm:
		default:
		}
	}, nil)
	return ch
}

func compileShader(typ uint32, source string) (uint32, error) {
	shader := gl.CreateShader(typ)

	csources, free := gl.Strs(source + "\x00")
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
		return 0, ShaderError{
			stage:   typ,
			message: log,
		}
	}
	return shader, nil
}

func linkProgram(sources map[uint32]string) (uint32, error) {
	shaders := map[uint32]uint32{}
	freeShaders := func() {
		for _, sh := range shaders {
			gl.DeleteShader(sh)
		}
	}

	for typ, source := range sources {
		sh, err := compileShader(typ, source)
		if err != nil {
			freeShaders()
			return 0, err
		}
		shaders[typ] = sh
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
		linkErr = ShaderError{message: log}
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

type ShaderError struct {
	stage uint32

	message string
}

func (err ShaderError) Error() (str string) {
	if err.stage == gl.VERTEX_SHADER {
		str += "Error compiling vertex shader:\n"
	} else if err.stage == gl.FRAGMENT_SHADER {
		str += "Error compiling fragment shader:\n"
	}
	str += err.message
	return
}

type Uniform struct {
	Name     string
	Type     uint32
	Location int32
}

func ListUniforms(program uint32) map[string]Uniform {
	var numUniforms int32
	gl.GetProgramiv(program, gl.ACTIVE_UNIFORMS, &numUniforms)
	var bufSize int32
	gl.GetProgramiv(program, gl.ACTIVE_UNIFORM_MAX_LENGTH, &bufSize)

	uniforms := map[string]Uniform{}
	for i := uint32(0); i < uint32(numUniforms); i++ {
		var length, size int32
		var typ uint32
		nameBuf := strings.Repeat("\x00", int(bufSize))
		gl.GetActiveUniform(program, i, bufSize, &length, &size, &typ, gl.Str(nameBuf))
		name := strings.SplitN(nameBuf, "\x00", -1)[0]

		uniforms[name] = Uniform{
			Name:     name,
			Type:     typ,
			Location: int32(i),
		}
	}
	return uniforms
}

func (u Uniform) TypeLiteral() string {
	switch u.Type {
	case gl.FLOAT:
		return "float"
	case gl.FLOAT_VEC2:
		return "vec2"
	case gl.FLOAT_VEC3:
		return "vec3"
	case gl.FLOAT_VEC4:
		return "vec4"
	case gl.DOUBLE:
		return "double"
	case gl.DOUBLE_VEC2:
		return "dvec2"
	case gl.DOUBLE_VEC3:
		return "dvec3"
	case gl.DOUBLE_VEC4:
		return "dvec4"
	case gl.INT:
		return "int"
	case gl.INT_VEC2:
		return "ivec2"
	case gl.INT_VEC3:
		return "ivec3"
	case gl.INT_VEC4:
		return "ivec4"
	case gl.UNSIGNED_INT:
		return "unsigned int"
	case gl.UNSIGNED_INT_VEC2:
		return "uvec2"
	case gl.UNSIGNED_INT_VEC3:
		return "uvec3"
	case gl.UNSIGNED_INT_VEC4:
		return "uvec4"
	case gl.BOOL:
		return "bool"
	case gl.BOOL_VEC2:
		return "bvec2"
	case gl.BOOL_VEC3:
		return "bvec3"
	case gl.BOOL_VEC4:
		return "bvec4"
	case gl.FLOAT_MAT2:
		return "mat2"
	case gl.FLOAT_MAT3:
		return "mat3"
	case gl.FLOAT_MAT4:
		return "mat4"
	case gl.FLOAT_MAT2x3:
		return "mat2x3"
	case gl.FLOAT_MAT2x4:
		return "mat2x4"
	case gl.FLOAT_MAT3x2:
		return "mat3x2"
	case gl.FLOAT_MAT3x4:
		return "mat3x4"
	case gl.FLOAT_MAT4x2:
		return "mat4x2"
	case gl.FLOAT_MAT4x3:
		return "mat4x3"
	case gl.DOUBLE_MAT2:
		return "dmat2"
	case gl.DOUBLE_MAT3:
		return "dmat3"
	case gl.DOUBLE_MAT4:
		return "dmat4"
	case gl.DOUBLE_MAT2x3:
		return "dmat2x3"
	case gl.DOUBLE_MAT2x4:
		return "dmat2x4"
	case gl.DOUBLE_MAT3x2:
		return "dmat3x2"
	case gl.DOUBLE_MAT3x4:
		return "dmat3x4"
	case gl.DOUBLE_MAT4x2:
		return "dmat4x2"
	case gl.DOUBLE_MAT4x3:
		return "dmat4x3"
	case gl.SAMPLER_1D:
		return "sampler1D"
	case gl.SAMPLER_2D:
		return "sampler2D"
	case gl.SAMPLER_3D:
		return "sampler3D"
	case gl.SAMPLER_CUBE:
		return "samplerCube"
	case gl.SAMPLER_1D_SHADOW:
		return "sampler1DShadow"
	case gl.SAMPLER_2D_SHADOW:
		return "sampler2DShadow"
	case gl.SAMPLER_1D_ARRAY:
		return "sampler1DArray"
	case gl.SAMPLER_2D_ARRAY:
		return "sampler2DArray"
	case gl.SAMPLER_1D_ARRAY_SHADOW:
		return "sampler1DArrayShadow"
	case gl.SAMPLER_2D_ARRAY_SHADOW:
		return "sampler2DArrayShadow"
	case gl.SAMPLER_2D_MULTISAMPLE:
		return "sampler2DMS"
	case gl.SAMPLER_2D_MULTISAMPLE_ARRAY:
		return "sampler2DMSArray"
	case gl.SAMPLER_CUBE_SHADOW:
		return "samplerCubeShadow"
	case gl.SAMPLER_BUFFER:
		return "samplerBuffer"
	case gl.SAMPLER_2D_RECT:
		return "sampler2DRect"
	case gl.SAMPLER_2D_RECT_SHADOW:
		return "sampler2DRectShadow"
	case gl.INT_SAMPLER_1D:
		return "isampler1D"
	case gl.INT_SAMPLER_2D:
		return "isampler2D"
	case gl.INT_SAMPLER_3D:
		return "isampler3D"
	case gl.INT_SAMPLER_CUBE:
		return "isamplerCube"
	case gl.INT_SAMPLER_1D_ARRAY:
		return "isampler1DArray"
	case gl.INT_SAMPLER_2D_ARRAY:
		return "isampler2DArray"
	case gl.INT_SAMPLER_2D_MULTISAMPLE:
		return "isampler2DMS"
	case gl.INT_SAMPLER_2D_MULTISAMPLE_ARRAY:
		return "isampler2DMSArray"
	case gl.INT_SAMPLER_BUFFER:
		return "isamplerBuffer"
	case gl.INT_SAMPLER_2D_RECT:
		return "isampler2DRect"
	case gl.UNSIGNED_INT_SAMPLER_1D:
		return "usampler1D"
	case gl.UNSIGNED_INT_SAMPLER_2D:
		return "usampler2D"
	case gl.UNSIGNED_INT_SAMPLER_3D:
		return "usampler3D"
	case gl.UNSIGNED_INT_SAMPLER_CUBE:
		return "usamplerCube"
	case gl.UNSIGNED_INT_SAMPLER_1D_ARRAY:
		return "usampler2DArray"
	case gl.UNSIGNED_INT_SAMPLER_2D_ARRAY:
		return "usampler2DArray"
	case gl.UNSIGNED_INT_SAMPLER_2D_MULTISAMPLE:
		return "usampler2DMS"
	case gl.UNSIGNED_INT_SAMPLER_2D_MULTISAMPLE_ARRAY:
		return "usampler2DMSArray"
	case gl.UNSIGNED_INT_SAMPLER_BUFFER:
		return "usamplerBuffer"
	case gl.UNSIGNED_INT_SAMPLER_2D_RECT:
		return "usampler2DRect"
	case gl.IMAGE_1D:
		return "image1D"
	case gl.IMAGE_2D:
		return "image2D"
	case gl.IMAGE_3D:
		return "image3D"
	case gl.IMAGE_2D_RECT:
		return "image2DRect"
	case gl.IMAGE_CUBE:
		return "imageCube"
	case gl.IMAGE_BUFFER:
		return "imageBuffer"
	case gl.IMAGE_1D_ARRAY:
		return "image1DArray"
	case gl.IMAGE_2D_ARRAY:
		return "image2DArray"
	case gl.IMAGE_2D_MULTISAMPLE:
		return "image2DMS"
	case gl.IMAGE_2D_MULTISAMPLE_ARRAY:
		return "image2DMSArray"
	case gl.INT_IMAGE_1D:
		return "iimage1D"
	case gl.INT_IMAGE_2D:
		return "iimage2D"
	case gl.INT_IMAGE_3D:
		return "iimage3D"
	case gl.INT_IMAGE_2D_RECT:
		return "iimage2DRect"
	case gl.INT_IMAGE_CUBE:
		return "iimageCube"
	case gl.INT_IMAGE_BUFFER:
		return "iimageBuffer"
	case gl.INT_IMAGE_1D_ARRAY:
		return "iimage1DArray"
	case gl.INT_IMAGE_2D_ARRAY:
		return "iimage2DArray"
	case gl.INT_IMAGE_2D_MULTISAMPLE:
		return "iimage2DMS"
	case gl.INT_IMAGE_2D_MULTISAMPLE_ARRAY:
		return "iimage2DMSArray"
	case gl.UNSIGNED_INT_IMAGE_1D:
		return "uimage1D"
	case gl.UNSIGNED_INT_IMAGE_2D:
		return "uimage2D"
	case gl.UNSIGNED_INT_IMAGE_3D:
		return "uimage3D"
	case gl.UNSIGNED_INT_IMAGE_2D_RECT:
		return "uimage2DRect"
	case gl.UNSIGNED_INT_IMAGE_CUBE:
		return "uimageCube"
	case gl.UNSIGNED_INT_IMAGE_BUFFER:
		return "uimageBuffer"
	case gl.UNSIGNED_INT_IMAGE_1D_ARRAY:
		return "uimage1DArray"
	case gl.UNSIGNED_INT_IMAGE_2D_ARRAY:
		return "uimage2DArray"
	case gl.UNSIGNED_INT_IMAGE_2D_MULTISAMPLE:
		return "uimage2DMS"
	case gl.UNSIGNED_INT_IMAGE_2D_MULTISAMPLE_ARRAY:
		return "uimage2DMSArray"
	case gl.UNSIGNED_INT_ATOMIC_COUNTER:
		return "atomic_uint"
	}
	return "invalid"
}

func (u Uniform) String() string {
	return fmt.Sprintf("uniform %s %s (%x)", u.TypeLiteral(), u.Name, u.Location)
}
