Shady
=====
[![Build Status](https://github.com/polyfloyd/shady/workflows/CI/badge.svg)](https://github.com/polyfloyd/shady/actions)

Shady is a nifty CLI tool for rendering GLSL fragment shaders for easy
development and hacking.


## Usage
### Installation
```sh
go install github.com/polyfloyd/shady/cmd/shady@latest
```

### Shadertoy
* https://shadertoy.com/

The basic setup is a single fragment shader, like a regular fragments shader,
calculates the color for each pixel. But instead of receiving vertex and normal
and transformation information from the vertex shader, it defines it's own
algorithm for shapes.

The best supported format/environment for shaders is that of Shadertoy.com.
Example:
```
void mainImage(out vec4 fragColor, in vec2 fragCoord) {
  vec2 uv = fragCoord.xy / iResolution.xy;
  fragColor = vec4(uv.x, uv.y, 0, 1);
}
```

Currently, the `iTime`, `iTimeDelta`, `iFrame`, `iDate`, `iMouse`, and
`iResolution`, `iChannelResolution` uniforms are supported. Other uniforms are
defined but not initialized.

See also https://www.shadertoy.com/howto for info on how to write shaders for
Shadertoy.

### Including other source files
To include another GLSL file, you may use the directive below:
```glsl
#pragma use "path/to/file.glsl"
```
This allows you to use functions and such from other GLSL files so it becomes
possible to create libraries. There is no namespacing or generation of forward
function declarations, it just takes a source file and dumps it in the place of
this directive much like C does. However, it does prevent including the same
file more than once in recursive inclusion.

File paths are resolved relative to the source file that declared the include
directive.

### Mappings
It is possible use resources like images, videos and audio from shaders in
this environment by using the `iChannelX` samplers. On the website, one can
select this resource input mapping using dialogs. This implementation requires
these mappings to be specified in the shader source. The respective uniform is
declared automatically.

Mappings can refer to other files such as images, videos and audio. Relative
paths are resolved relative to the GLSL file that declared them.

Mappings are declared in a special directive that is parsed by shady. These are
typically inserted at the top of the file. Its format is:
```glsl
#pragma map <uniform name>=<loader>:<value>
```

Mappings can also be set on the command line when invoking shady by using the
`-map` flag. The format is the same as described earlier. Mappings set on the
command line take precedence over mappings declared in source files. There are
no hard requirements for the `namespace` being the same as in the shader, so it
is possible to e.g. override an image with a video. Example:
```
shady -i thing.glsl -ofmt x11 -g 1366x768 -f 60 -map myTexture=video:myVideo.mkv
```

`uniform name` is the name of the sampler uniform that is inserted into the
source of the fragment shader. Unlike shadertoy.com, which names all samples as
`iChannelX`, the name can be of any value as long as it is a valid GLSL
variable name.
`loader` specifies how `value` should be interpreted.
There are a couple of loaders that you can choose from:

#### The "builtin" loader
The `builtin` loader gives access to some of the presets that can be found on
Shadertoy. Accepted values for `builtin` are:
* `Back Buffer`: creates a `sampler2D` containing the previously rendered
  image.
* `RGBA Noise Small`: creates a `sampler2D` texture with pseudo-random noise.
  The randomness is deterministic.
* `RGBA Noise Medium`: the same as above, but bigger.

Example: Enable the sampler named `iChannel0` as a noise texture:
```glsl
#pragma map iChannel0=builtin:RGBA Noise Medium
```

#### The "image" loader
Setting the loader to `image` interprets the value as a path to an image file
and creates a `sampler2D` containing a static texture containing the RGBA data
of the image file.
Supported formats are JPEG, PNG and GIF, the latter using the first frame of
the animation for the texture.

For each mapped image, an additional `vec3` uniform is created with the
original size of the image named `${uniform name}Size`. The Z component of this
vector is reserved.

Example:
```glsl
#pragma map myTexture=image:yoloswag.png
```

#### The "audio" loader
Audio files can be loaded as a texture with a size of 512x2. Row 0 contains the
FFT of the current window and row 1 contains the actual sound wave. For regular
audio files, The playback rate is determined by the duration and framerate
flags. For realtime audio, the window is the most recently produced audio,
skipping information if rendering can not keep up.

If the value is just a file, this file is used as audio. FFmpeg is invoked to
decode the file, so any format supported by FFmpeg can be played.

For realtime audio, only raw PCM pipes are supported. The filename must be
followed by the PCM format settings as `;<rate>:<channels>:<encoding>`.
`encoding` is the sign as `s` or `u` followed by the number of bits per sample
and then the endianness as `le` or `be`, e.g. `s16le`.

Example: Map `audio` to the audio of an MP3 file:
```glsl
#pragma map music=audio:whatever.mp3
```

#### The "video" loader
Using videos as textures is very similar to images, there is a `sampler2D`
uniform containing the current video frame and a `${uniform name}Size` vector
for the resolution. There is an additional `{uniform name}CurTime` float which
is the current time in seconds in the video. This value is the same as `iTime`,
but wraps when the video is restarted from the beginning.

The sound of the video is not available, although this may be implemented in
the future.

Example:
```glsl
#pragma map video=video:party.mkv
```

#### The "buffer" loader
It is possible to map another shader as a texture by using the `buffer` loader.
This is equivalent of just calling the `mainImage` function of this other
shader and using the calculated color as texel. However, buffers have a
separate resolution and `Back Buffer`. Because the render output of a buffer in
raster format, the size of the texture should be specified in the mapping by
appending `;WxH` to the shader filename.

Like videos, the buffer is declared as a `sampler2D` along with a
`${uniform name}Size` vector.

Example:
```glsl
#pragma map thing=buffer:other-shader.glsl;512x512
```

**NOTE**: Buffer support is not very well tested, your mileage may vary.

#### The "kinect" loader
If Shady was compiled using the `kinect` build tag, it is possible to use a
Kinect's RGB and depth image in shaders. Just pass `-tags kinect` to `go build`
when building and use `#pragma map kinect=kinect:on` to create a `sampler2D` of
the Kinect's video stream. The alpha channel holds the depth image.

Internally, libfreenect is used which only supports the earlier Kinect versions
for the XBox 360.


## Combining with other tools
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

shady -i example.glsl -ofmt rgb24 -f 20 | ledcat -f 20 show
```

### FFmpeg
FFmpeg may be used to render to video files:
```
# Render at 1024x768 at 20 fps and show it, the same as using `-ofmt x11`:
shady -i example.glsl -ofmt rgb24 -g 1024x768 -f 20 \
  | ffplay -f rawvideo -pixel_format rgb24 -video_size 1024x768 -f 20 -

# The same, but render 12 seconds to an MP4 file
shady -i example.glsl -ofmt rgb24 -g 1024x768 -f 10 \
  | ffmpeg -f rawvideo -pixel_format rgb24 -video_size 1024x768 \
    -framerate 10 -t 12 -i - example.mp4
```

### MPD
Visualising the output of MPD is possible by adding the following to your MPD
config:
```
audio_output {
  type   "fifo"
  name   "FIFO"
  path   "~/.mpd/mpd.fifo"
  format "22000:16:1"
}
```
And then creating a PCM audio mapping like this:
```glsl
#pragma map music=audio:~/.mpd/mpd.fifo;22000:1:s16le
```

## Troubleshooting
### My performance is really bad
Some shaders can really ask a lot from a system, in these cases it may not be
possible to animate real time. If it is acceptable to have the animation be of
finite length and restart after a while, write a series of frame to a file, and
load them in a loop.

```sh
# Render a 20 second loop to a file:
shady -i example.glsl -g 64x64 -f 60 -n $((20*60)) -ofmt rgb24 -o ./my-animation.bin

# Play the animation repeatedly:
while true; do
    cat ./my-animation.bin | ledcat -g 64x64 -f 60 show
done
```
Optionally, you could use something like gzip to reduce the file size.

### EGL is not initialized, or could not be initialized
Headless rendering is possible. If `$DISPLAY` is unset because X11 is not
running, try running shady with the `EGL_PLATFORM` env var set to `surfaceless`
or `drm`.

If you still are not able to get shady to run headless, animate to a file and
play from that file in real time. [See
above](#user-content-my-performance-is-really-bad).

### unexpected NEW_IDENTIFIER
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
