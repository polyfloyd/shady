Shady
=====

Shady is a nifty CLI tool for rendering GLSL fragment shaders for easy
development and hacking.


## Usage
### Installation
```sh
go get -u github.com/polyfloyd/shady/cmd/shady
```

### Writing Shaders
Currently, only the shaders from [glslsandbox.com](http://glslsandbox.com/) are
supported.

The setup is a single fragment shader, like a regular fragments shader,
calculates the color for each pixel. But instead of receiving vertex and normal
and tranformation information from the vertex shader, it defines it's own
algorithm for shapes.

Besides `gl_FragCoord`, the following uniforms are available:
* `uniform float time`: the time since startup in seconds
* `uniform vec2 resolution`: the resolution of the display in pixels

glslsandbox.com also exposes these uniforms, but are not (yet) supported by
Shady:
* `uniform vec2 mouse`: the position of the mouse cursor relative to the bottom
  left corner. Shady is a CLI application so this will never be supported
* `varying vec2 surfacePosition`: [???](https://github.com/mrdoob/glsl-sandbox/issues/45)
* `uniform sampler2D backbuffer`: a texture storing previously rendered image

Check out [example.glsl](example.glsl) to see what a shader for this website
looks like.


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
