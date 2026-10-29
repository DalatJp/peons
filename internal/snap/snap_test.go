package snap

import (
	"image"
	"image/color"
	"math/rand"
	"testing"
	"time"
)

// clawdLike : un bloc opaque de 30×20 à deux couleurs, posé en (5,7).
func clawdLike() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	for y := 7; y < 27; y++ {
		for x := 5; x < 35; x++ {
			c := color.NRGBA{215, 119, 87, 255}
			if (x+y)%5 == 0 {
				c = color.NRGBA{20, 20, 20, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func draw(s *Snap, t time.Duration) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(-60, -60, 120, 120))
	s.Draw(dst, image.Point{}, t)
	return dst
}

func count(img *image.NRGBA) int {
	n := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			n++
		}
	}
	return n
}

// Au départ, l'image telle quelle ; à la fin, plus rien, en moins de 3 secondes.
func TestStartsWholeEndsEmpty(t *testing.T) {
	src := clawdLike()
	s := New(src, image.Pt(10, -4), rand.New(rand.NewSource(1)))
	got := draw(s, 0)
	for y := -60; y < 120; y++ {
		for x := -60; x < 120; x++ {
			var want color.NRGBA
			if q := image.Pt(x-10, y+4); q.In(src.Rect) {
				want = src.NRGBAAt(q.X, q.Y)
			}
			if p := got.NRGBAAt(x, y); p != want && (p.A != 0 || want.A != 0) {
				t.Fatalf("à t=0, (%d,%d) = %v, attendu %v", x, y, p, want)
			}
		}
	}
	if Duration >= 3*time.Second {
		t.Errorf("durée %v : l'animation doit durer moins de 3 s", Duration)
	}
	if n := count(draw(s, Duration)); n != 0 {
		t.Errorf("à la fin, %d pixels encore dessinés", n)
	}
}

// Les pixels ne font que disparaître (aucun ne revient), le dessin fond de plus de
// moitié en 1,5 s, et rien ne sort de Bounds. Le nombre de pixels dessinés, lui,
// peut remonter un instant : deux pixels qui se recouvrent à l'envol se séparent.
func TestShrinksWithinBounds(t *testing.T) {
	s := New(clawdLike(), image.Point{}, rand.New(rand.NewSource(2)))
	start, prev := count(draw(s, 0)), len(s.parts)
	for t0 := time.Duration(0); t0 <= Duration; t0 += 20 * time.Millisecond {
		alive := 0
		for _, p := range s.parts {
			if t0.Seconds() < p.fade {
				alive++
			}
		}
		if alive > prev {
			t.Errorf("à %v : %d pixels vivants, plus qu'avant (%d)", t0, alive, prev)
		}
		prev = alive
		img := draw(s, t0)
		if n := count(img); t0 >= 1500*time.Millisecond && n*2 > start {
			t.Errorf("à %v : encore %d pixels sur %d", t0, n, start)
		}
		for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
			for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
				if img.NRGBAAt(x, y).A != 0 && !image.Pt(x, y).In(s.Bounds()) {
					t.Fatalf("à %v : (%d,%d) hors de Bounds %v", t0, x, y, s.Bounds())
				}
			}
		}
	}
}

// Une partie des pixels s'efface vite, sur place ; les autres s'envolent, avec le
// vent (vers la droite et le haut), le côté du vent le premier.
func TestDissolvesThenBlowsAway(t *testing.T) {
	s := New(clawdLike(), image.Point{}, rand.New(rand.NewSource(3)))
	inPlaceN, left, right := 0, []float64{}, []float64{}
	for _, p := range s.parts {
		if p.launch < 0 {
			inPlaceN++
			if p.fade > 1 {
				t.Errorf("un pixel qui s'efface sur place doit partir dans la première seconde : %.2f s", p.fade)
			}
			continue
		}
		end := p.pos(p.fade - 0.001)
		if end.X <= p.at.X || end.Y > p.at.Y+1 {
			t.Errorf("pixel parti de %v, fini en %v : le vent pousse vers la droite et le haut", p.at, end)
		}
		switch {
		case p.at.X < 15:
			left = append(left, p.launch)
		case p.at.X >= 25:
			right = append(right, p.launch)
		}
	}
	if f := float64(inPlaceN) / float64(len(s.parts)); f < 0.35 || f > 0.55 {
		t.Errorf("%.0f %% des pixels s'effacent sur place, attendu autour de 45 %%", f*100)
	}
	mean := func(v []float64) float64 {
		sum := 0.0
		for _, x := range v {
			sum += x
		}
		return sum / float64(len(v))
	}
	if mean(right) >= mean(left) {
		t.Errorf("envol moyen à droite %.2f s, à gauche %.2f s : le côté du vent doit partir le premier", mean(right), mean(left))
	}
	// à mi-parcours, des pixels ont quitté leur place
	moved := 0
	for _, p := range s.parts {
		if p.launch >= 0 && p.launch < 1.2 && p.fade > 1.2 && p.pos(1.2) != p.at {
			moved++
		}
	}
	if moved == 0 {
		t.Error("à 1,2 s, aucun pixel en vol")
	}
}
