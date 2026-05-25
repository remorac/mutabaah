// Package imageutil contains helpers for validating uploads and producing the
// thumbnail rendered in the navbar and profile card.
package imageutil

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

// ErrUnsupportedType is returned when an upload is neither JPEG nor PNG.
var ErrUnsupportedType = errors.New("unsupported image type")

// ThumbSize is the side length, in pixels, of the generated square thumbnail.
const ThumbSize = 256

// Validate inspects the first bytes of data and returns the mime type plus the
// canonical file extension (".jpg" or ".png"). Returns ErrUnsupportedType for
// anything else.
func Validate(data []byte) (mime, ext string, err error) {
	if len(data) == 0 {
		return "", "", ErrUnsupportedType
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	mime = http.DetectContentType(head)
	switch mime {
	case "image/jpeg":
		return mime, ".jpg", nil
	case "image/png":
		return mime, ".png", nil
	default:
		return "", "", ErrUnsupportedType
	}
}

// SaveOriginalAndThumb writes the original bytes verbatim and a center-cropped
// 256x256 thumbnail next to it. Files land at:
//
//	<dir>/<basename><ext>
//	<dir>/thumb_<basename>.jpg
//
// The original keeps its uploaded type; thumbnails are always JPEG.
func SaveOriginalAndThumb(dir, basename, ext string, data []byte) error {
	origPath := filepath.Join(dir, basename+ext)
	thumbPath := filepath.Join(dir, "thumb_"+basename+".jpg")

	src, err := decode(data, ext)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	thumb := resizeSquare(src, ThumbSize)

	if err := os.WriteFile(origPath, data, 0o644); err != nil {
		return fmt.Errorf("write original: %w", err)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 85}); err != nil {
		return fmt.Errorf("encode jpeg thumb: %w", err)
	}
	if err := os.WriteFile(thumbPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write thumb: %w", err)
	}
	return nil
}

func decode(data []byte, ext string) (image.Image, error) {
	r := bytes.NewReader(data)
	switch ext {
	case ".jpg":
		return jpeg.Decode(r)
	case ".png":
		return png.Decode(r)
	default:
		return nil, ErrUnsupportedType
	}
}

// resizeSquare returns a size x size image cropped to the largest centered
// square of src and rescaled with a high-quality filter.
func resizeSquare(src image.Image, size int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	side := w
	if h < side {
		side = h
	}
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y + (h-side)/2
	cropRect := image.Rect(x0, y0, x0+side, y0+side)

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, cropRect, draw.Over, nil)
	return dst
}
