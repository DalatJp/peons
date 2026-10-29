// Package snap désagrège une image à la manière du claquement de doigts de Thanos :
// une partie des pixels s'efface sur place, vite et au hasard ; les autres se
// détachent peu à peu, du côté où souffle le vent d'abord, s'envolent en ondulant,
// prennent la teinte de la poussière et s'effacent en vol. Tout a disparu au bout
// de Duration.
//
// Comme les autres animations, c'est une fonction du temps : Draw dessine l'état à
// l'instant t, sans rien retenir d'un appel à l'autre.
package snap

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"time"
)

// Duration : au-delà, plus rien n'est dessiné.
const Duration = 2500 * time.Millisecond

const (
	inPlace = 0.45 // part des pixels qui s'effacent sur place
	windX   = 30.0 // le vent pousse vers la droite (accélération, pixels par s²)…
	windY   = -8.0 // …et soulève un peu
)

// dust : la teinte vers laquelle vire un pixel qui s'envole.
var dust = color.NRGBA{74, 66, 60, 255}

type particle struct {
	at     image.Point // place d'origine
	col    color.NRGBA
	launch float64 // instant de l'envol, en secondes ; < 0 : s'efface sur place
	fade   float64 // instant où il disparaît
	vx, vy float64 // vitesse au départ, en pixels par seconde
	phase  float64 // de l'ondulation
}

// Snap est la désagrégation d'une image ; immuable une fois créée.
type Snap struct {
	parts  []particle
	bounds image.Rectangle
}

// New prépare la désagrégation des pixels opaques de img, posée en at.
func New(img *image.NRGBA, at image.Point, rng *rand.Rand) *Snap {
	b := img.Bounds()
	var opaque image.Rectangle
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.NRGBAAt(x, y).A != 0 {
				opaque = opaque.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	s := &Snap{}
	end := Duration.Seconds() - 0.01
	for y := opaque.Min.Y; y < opaque.Max.Y; y++ {
		for x := opaque.Min.X; x < opaque.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			p := particle{at: image.Pt(x, y).Sub(b.Min).Add(at), col: c, launch: -1}
			if rng.Float64() < inPlace {
				p.fade = 0.05 + 0.85*rng.Float64()
			} else {
				// le côté du vent (la droite) part le premier
				downwind := float64(x-opaque.Min.X) / float64(max(1, opaque.Dx()-1))
				p.launch = 0.1 + 0.7*(1-downwind) + 0.3*rng.Float64()
				p.fade = min(end, p.launch+0.45+0.9*rng.Float64())
				p.vx, p.vy = 10+16*rng.Float64(), -(4 + 10*rng.Float64())
				p.phase = rng.Float64()
			}
			s.parts = append(s.parts, p)
		}
	}
	for _, p := range s.parts { // tout ce que Draw peut toucher
		s.bounds = s.bounds.Union(image.Rect(p.at.X, p.at.Y, p.at.X+1, p.at.Y+1))
		if p.launch < 0 {
			continue
		}
		for k := 0; k <= 20; k++ {
			q := p.pos(p.launch + (p.fade-p.launch)*float64(k)/20)
			s.bounds = s.bounds.Union(image.Rect(q.X, q.Y, q.X+1, q.Y+1))
		}
	}
	return s
}

// pos : la place du pixel à l'instant t (secondes).
func (p particle) pos(t float64) image.Point {
	if p.launch < 0 || t < p.launch {
		return p.at
	}
	tau := t - p.launch
	wave := 0.8 * (math.Sin(2*math.Pi*(1.5*tau+p.phase)) - math.Sin(2*math.Pi*p.phase)) // nulle à l'envol : pas de saut
	return image.Pt(
		p.at.X+int(math.Round(p.vx*tau+windX*tau*tau/2)),
		p.at.Y+int(math.Round(p.vy*tau+windY*tau*tau/2+wave)))
}

// color : sa couleur à l'instant t ; en vol, elle vire vers la poussière.
func (p particle) color(t float64) color.NRGBA {
	if p.launch < 0 || t < p.launch {
		return p.col
	}
	k := 0.55 * (t - p.launch) / (p.fade - p.launch)
	mix := func(a, b uint8) uint8 { return uint8(math.Round(float64(a) + (float64(b)-float64(a))*k)) }
	return color.NRGBA{mix(p.col.R, dust.R), mix(p.col.G, dust.G), mix(p.col.B, dust.B), p.col.A}
}

// Draw dessine la désagrégation à l'instant t dans dst, décalée de origin ; ce
// qui sort de dst est perdu.
func (s *Snap) Draw(dst *image.NRGBA, origin image.Point, t time.Duration) {
	sec := t.Seconds()
	for _, p := range s.parts {
		if sec >= p.fade {
			continue
		}
		if q := p.pos(sec).Add(origin); q.In(dst.Rect) {
			dst.SetNRGBA(q.X, q.Y, p.color(sec))
		}
	}
}

// Bounds : tout ce que Draw peut toucher, avec origin en (0,0).
func (s *Snap) Bounds() image.Rectangle { return s.bounds }
