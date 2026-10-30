package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"

	// Register GIF/PNG decode.
	_ "image/gif"
	_ "image/png"
)

const (
	adminPosterMaxBytes = 12 << 20 // 12 MiB upload cap before crop
	adminPosterJPEGQual = 90
)

// AdminPosterKey returns a stable R2 object key for an admin-uploaded poster.
func AdminPosterKey(videoID uint64, body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("posters/admin/%d/%s.jpg", videoID, hex.EncodeToString(sum[:16]))
}

// CropCenterToAspect center-crops src to the given width/height aspect ratio.
// aspectW and aspectH must be > 0. Returns a new NRGBA image.
func CropCenterToAspect(src image.Image, aspectW, aspectH float64) image.Image {
	if src == nil || aspectW <= 0 || aspectH <= 0 {
		return src
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return src
	}
	target := aspectW / aspectH
	cur := float64(sw) / float64(sh)
	var cw, ch int
	if cur > target {
		// too wide — crop sides
		ch = sh
		cw = int(float64(sh) * target)
	} else {
		// too tall — crop top/bottom
		cw = sw
		ch = int(float64(sw) / target)
	}
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}
	if cw > sw {
		cw = sw
	}
	if ch > sh {
		ch = sh
	}
	ox := b.Min.X + (sw-cw)/2
	oy := b.Min.Y + (sh-ch)/2
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	cropRect := image.Rect(ox, oy, ox+cw, oy+ch)
	if si, ok := src.(subImager); ok {
		return si.SubImage(cropRect)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, cw, ch))
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			dst.Set(x, y, src.At(ox+x, oy+y))
		}
	}
	return dst
}

// DecodeImageBytes decodes JPEG/PNG/GIF image bytes.
func DecodeImageBytes(body []byte) (image.Image, string, error) {
	img, format, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	return img, format, nil
}

// EncodeJPEG encodes img as JPEG.
func EncodeJPEG(img image.Image, quality int) ([]byte, error) {
	if quality <= 0 || quality > 100 {
		quality = adminPosterJPEGQual
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PutAdminPoster crops the uploaded image to aspectW:aspectH, encodes JPEG,
// and stores it via ObjectStore. Returns the object key.
func PutAdminPoster(ctx context.Context, store ObjectStore, videoID uint64, raw []byte, aspectW, aspectH float64) (key string, err error) {
	if store == nil {
		return "", fmt.Errorf("object store unavailable")
	}
	if videoID == 0 {
		return "", fmt.Errorf("video id required")
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("empty image")
	}
	if len(raw) > adminPosterMaxBytes {
		return "", fmt.Errorf("image too large")
	}
	img, _, err := DecodeImageBytes(raw)
	if err != nil {
		return "", fmt.Errorf("unsupported image: %w", err)
	}
	if aspectW <= 0 || aspectH <= 0 {
		aspectW, aspectH = 2, 3
	}
	cropped := CropCenterToAspect(img, aspectW, aspectH)
	out, err := EncodeJPEG(cropped, adminPosterJPEGQual)
	if err != nil {
		return "", err
	}
	key = AdminPosterKey(videoID, out)
	if err := store.Put(ctx, key, out, "image/jpeg"); err != nil {
		return "", err
	}
	return key, nil
}
