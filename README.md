Shady
=====
[![Build Status](https://travis-ci.org/polyfloyd/shady.svg)](https://travis-ci.org/polyfloyd/shady)
[![Go Report Card](https://goreportcard.com/badge/github.com/polyfloyd/shady)](https://goreportcard.com/report/github.com/polyfloyd/shady)

Shady is a nifty CLI tool for rendering GLSL fragment shaders for easy
development and hacking.


## Usage
### Installation
```sh
go get -u github.com/polyfloyd/shady/cmd/shady
```

### Writing Shaders
The basic setup is a single fragment shader, like a regular fragments shader,
calculates the color for each pixel. But instead of receiving vertex and normal
and transformation information from the vertex shader, it defines it's own
algorithm for shapes.

#### GLSL Sandbox
* http://glslsandbox.com/

Besides `gl_FragCoord`, the following inputs are available:
* `uniform float time`: the time since startup in seconds
* `uniform vec2 resolution`: the resolution of the display in pixels
* `uniform sampler2D backbuffer`: a texture storing the previously rendered image
* `varying vec2 surfacePosition`: the panning position. For backwards compatibility
* `varying vec2 surfaceSize`:  the resolution after zooming. For backwards compatibility
* `uniform vec2 mouse`: the position of the mouse cursor relative to the bottom
  left corner. Shady is a CLI application so this will never be fully
  supported. However, the value is set to the center of the display.

Check out [example.glsl](shaders/example.glsl) to see what a shader for this website
looks like.

#### ShaderToy
* https://shadertoy.com/

Currently, the `iTime`, `iTimeDelta`, `iFrame`, `iDate`, `iMouse`, and
`iResolution` uniforms are supported. Other uniforms are defined but not
initialized.

It is possible use resources like images, videos and audio from shaders in
this environment by using the `iChannelX` samplers. On the website, one can
select this resource input mapping using dialogs. This implementation requires
these mappings to be specified in the shader source. When this is done, the
uniform is declared automatically.

Mappings are declared in a special comment that is parsed by shady. These are
typically inserted at the top of the file. It's format is:
```glsl
// map <uniform name>=<namespace>:<value>
```

`uniform name` is the name of the sampler uniform that is inserted into the
source of the fragment shader. `namespace` specifies how `value` should be
interpreted. The `builtin` namespace gives access to the presets that can be
found on ShaderToy. Setting the namespace to `image` interprets the value as a
path relative to the shader source file to use as texture.

Accepted values for `builtin` are:
* `RGBA Noise Small`
* `RGBA Noise Medium`

Example: Enable the sampler named `iChannel0` as a texture with the builtin
"RGBA Noise Medium" preset:
```glsl
// map iChannel0=builtin:RGBA Noise Medium
```

Example: Map `iChannel1` to an image referenced by a path relative to the
shader source file. Image files become sampler2D type uniforms.
```glsl
// map iChannel1=image:image.png
```

See also https://www.shadertoy.com/howto for info on how to write shaders for
ShaderToy.


### Ledcat
[Ledcat](https://github.com/polyfloyd/ledcat) is a program that can be used to
control lots of LEDs over lots of protocols. Shady can be combined with Ledcat
to bring the fireworks to your LED-displays!

It can be installed like this when you have the [Rust
Language](https://www.rust-lang.org/):
```sh
cargo install ledcat
```

To aid development, Ledcat can be used to simulate a display in a terminal like
this:
```sh
# LEDCAT_GEOMETRY is a special env var that Ledcat and Shady use to set the
# display size. It is also possible to use the -g flag on both programs.
export LEDCAT_GEOMETRY=128x128

shady -i example.glsl -ofmt rgb24 -framerate 20 | ledcat --framerate 20 show
```

### FFmpeg
FFmpeg may be used to visualize the output:
```
# Render at 1024x768 at 20 fps and show it
shady -i example.glsl -ofmt rgb24 -g 1024x768 -framerate 20 \
  | ffplay -f rawvideo -pixel_format rgb24 -video_size 1024x768 -framerate 20 -

# The same, but render 12 seconds to an MP4 file
shady -i example.glsl -ofmt rgb24 -g 1024x768 -framerate 10 \
  | ffmpeg -f rawvideo -pixel_format rgb24 -video_size 1024x768 \
    -framerate 10 -t 12 -i - example.mp4
```

### Troubleshooting
#### My performance is really bad
Some shaders can really ask a lot from a system, in these cases it may not be
possible to animate real time. If it is acceptable to have the animation be of
finite length and restart after a while, write a series of frame to a file, and
load them in a loop.

```sh
# Render a 20 second loop to a file:
shady -i example.glsl -g 64x64 -framerate 60 -numframes $((20*60)) -ofmt rgb24 -o ./my-animation.bin

# Play the animation repeatedly:
while true; do
    cat ./my-animation.bin | ledcat -g 64x64 -f 60 show
done
```
Optionally, you could use something like gzip to reduce the file size.

#### PlatformError: X11: Failed to open display
Make sure X is running and `$DISPLAY` is set. Headless support is
[TODO](https://github.com/polyfloyd/shady/issues/1).

If this is not possible or undesirable, animate to a file and play from that
file in real time. [See above](#user-content-my-performance-is-really-bad).

#### unexpected NEW_IDENTIFIER
```
Error compiling fragment shader:
0:2(1): error: syntax error, unexpected NEW_IDENTIFIER
```
The above error could be caused by a `precision mediump float;` being present.
Because this is an OpenGL ES directive, it is not supported. Try removing it or
wrapping with a preprocessor macro:
```glsl
#ifdef GL_ES
precision mediump float;
#endif
```


## Media
![Galaxy](media/galaxy.gif)
![Space](media/space.gif)
![Thingy](media/thingy.gif)
![Tunnel](media/tunnel.gif)
![Wolfenstein](media/wolfenstein.gif)
