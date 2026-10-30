package media

import (
	"image"
	"image/color"
	"testing"
)

func TestCropCenterToAspect_wideToPortrait(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 300, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 300; x++ {
			src.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	out := CropCenterToAspect(src, 2, 3)
	b := out.Bounds()
	w, h := b.Dx(), b.Dy()
	ratio := float64(w) / float64(h)
	want := 2.0 / 3.0
	if ratio < want-0.02 || ratio > want+0.02 {
		t.Fatalf("ratio=%v want ~%v (size %dx%d)", ratio, want, w, h)
	}
}

func TestCropCenterToAspect_tallToLandscape(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 300))
	out := CropCenterToAspect(src, 16, 9)
	b := out.Bounds()
	ratio := float64(b.Dx()) / float64(b.Dy())
	want := 16.0 / 9.0
	if ratio < want-0.05 || ratio > want+0.05 {
		t.Fatalf("ratio=%v want ~%v", ratio, want)
	}
}

func TestAdminPosterKey(t *testing.T) {
	k := AdminPosterKey(42, []byte("abc"))
	if k[:14] != "posters/admin/" {
		t.Fatalf("key=%q", k)
	}
}
