package main

import (
	"image"
	"image/color"
	"testing"
)

// blit agrandit chaque pixel en un carré u×u, à la bonne place, sans déborder.
func TestBlitScalesAndClips(t *testing.T) {
	o := &layeredOverlay{w: 6, h: 6, dib: dibSection{bits: make([]byte, 6*6*4)}}
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(1, 1, color.NRGBA{0, 0, 255, 255}) // (1,0) et (0,1) restent transparents
	o.blit(img, image.Pt(1, 1), 2)

	px := func(x, y int) [4]byte { // B, G, R, A
		i := (y*o.w + x) * 4
		return [4]byte{o.dib.bits[i], o.dib.bits[i+1], o.dib.bits[i+2], o.dib.bits[i+3]}
	}
	for _, c := range []image.Point{{1, 1}, {2, 1}, {1, 2}, {2, 2}} {
		if got := px(c.X, c.Y); got != [4]byte{0, 0, 255, 255} {
			t.Errorf("pixel rouge agrandi en %v : %v", c, got)
		}
	}
	for _, c := range []image.Point{{3, 3}, {4, 4}} {
		if got := px(c.X, c.Y); got != [4]byte{255, 0, 0, 255} {
			t.Errorf("pixel bleu agrandi en %v : %v", c, got)
		}
	}
	for _, c := range []image.Point{{3, 1}, {1, 3}, {0, 0}, {5, 5}} {
		if got := px(c.X, c.Y); got != [4]byte{} {
			t.Errorf("pixel transparent en %v : %v", c, got)
		}
	}
	// hors de la fenêtre : rien ne plante, rien n'est écrit de travers
	o.blit(img, image.Pt(5, 5), 2)
	o.blit(img, image.Pt(-3, -3), 2)
	if got := px(5, 5); got != [4]byte{0, 0, 255, 255} {
		t.Errorf("le coin haut-gauche de l'image doit tomber en (5,5) : %v", got)
	}
}

// Un pixel à demi transparent se compose par-dessus l'existant (alpha prémultiplié).
func TestBlitBlendsPartialAlpha(t *testing.T) {
	o := &layeredOverlay{w: 1, h: 1, dib: dibSection{bits: []byte{0, 0, 200, 255}}} // fond rouge opaque
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{0, 0, 255, 128}) // bleu à 50 %
	o.blit(img, image.Point{}, 1)
	b, g, r, a := o.dib.bits[0], o.dib.bits[1], o.dib.bits[2], o.dib.bits[3]
	if a != 255 || b < 120 || b > 130 || r < 95 || r > 105 || g != 0 {
		t.Errorf("mélange B=%d G=%d R=%d A=%d, attendu ≈ (127, 0, 100, 255)", b, g, r, a)
	}
}

func TestPremultiply(t *testing.T) {
	var d [4]byte
	premultiply(d[:], 10, 20, 30, 255)
	if d != [4]byte{30, 20, 10, 255} {
		t.Errorf("opaque : %v, attendu BGRA = 30 20 10 255", d)
	}
	premultiply(d[:], 200, 100, 50, 128)
	if d != [4]byte{25, 50, 100, 128} {
		t.Errorf("à demi transparent : %v, attendu 25 50 100 128", d)
	}
}
