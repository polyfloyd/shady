ifdef VERBOSE
	Q :=
	E := @true 
else
	Q := @
	E := @echo 

	ifndef NOCOLOR
		COLOR_COMPILE := \x1b[32;1m
		COLOR_RGB     := \x1b[31;1m
		COLOR_VIDEO   := \x1b[35;1m
		COLOR_RESET   := \x1b[0m
		E := @/bin/echo -e 
	endif
endif

RGBFPS=60
RGBGEOM=150x16
RGBSECONDS=4
VIDEOFPS=60
VIDEOGEOM=1366x768
VIDEOSECONDS=20

SHADY=../shady
FFMPEG=ffmpeg

SRCDIR := .
RGBDIR := ./rgb
VIDEODIR := ./video

SHADERFILES := $(shell find $(SRCDIR) -name '*.glsl')
RGBFILES   := $(SHADERFILES:$(SRCDIR)/%.glsl=$(RGBDIR)/%.rgb.gz)
VIDEOFILES := $(SHADERFILES:$(SRCDIR)/%.glsl=$(VIDEODIR)/%.mp4)

-include Makefile.local

.PHONY: all rgb video

all: rgb video

rgb: $(RGBFILES)
video: $(VIDEOFILES)

$SHADY:
	$(Q)cd ./.. && go build ./cmd/shady/

$(RGBDIR)/%.rgb.gz: $(SRCDIR)/%.glsl
	$(E)" [$(COLOR_RGB)RGB$(COLOR_RESET)] $@"
	$(Q)mkdir -p `dirname $@`
	$(Q)$(SHADY) -v -i $< -g $(RGBGEOM) -framerate $(RGBFPS) -duration $(RGBSECONDS) -ofmt rgb24 | gzip > $@

$(VIDEODIR)/%.mp4: $(SRCDIR)/%.glsl
	$(E)" [$(COLOR_VIDEO)VIDEO$(COLOR_RESET)] $@"
	$(Q)mkdir -p `dirname $@`
	$(Q)$(SHADY) -i $< -g $(VIDEOGEOM) -framerate $(VIDEOFPS) -duration $(VIDEOSECONDS) -ofmt rgb24 | \
	$(FFMPEG) -f rawvideo -pixel_format rgb24 -video_size $(VIDEOGEOM) \
		-framerate $(VIDEOFPS) -t $(VIDEOSECONDS) -i - -quality good -cpu-used 0 \
		-qmin 10 -qmax 42 -threads 8 $@
