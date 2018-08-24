package encode

import (
	"image"
	"io"
	"path"
	"time"
)

var Formats = map[string]Format{
	"ansi":   &AnsiDisplay{},
	"x11":    &X11Display{},
	"gif":    GIFFormat{},
	"jpg":    JPGFormat{},
	"png":    PNGFormat{},
	"rgb24":  RGB24Format{},
	"rgba32": RGBA32Format{},
}

func DetectFormat(filename string) (Format, bool) {
	ext := path.Ext(filename)
	if len(ext) == 0 {
		return nil, false
	}
	for _, f := range Formats {
		for _, e := range f.Extensions() {
			if e == ext[1:] {
				return f, true
			}
		}
	}
	return nil, false
}

type Format interface {
	// Extensions returns all file extensions excluding '.' that this format is
	// commonly encoded into.
	Extensions() []string

	// Encode encodes a single image to the specfied io.Writer.
	Encode(w io.Writer, img image.Image) error

	// EncodeAnimation encodes a series of successive images to the specified
	// io.Writer.
	//
	// The function should consume all images from the stream until it closes.
	// The interval parameter is the time between two images.
	EncodeAnimation(w io.Writer, stream <-chan image.Image, interval time.Duration) error
}
