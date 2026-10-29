package main

import (
	"image"
	"image/color"
	"image/gif"
	"math/rand"
	"os"
	"testing"
	"time"

	"peons/assets"
	"peons/internal/avatar"
	"peons/internal/mood"
	"peons/internal/prefs"
	"peons/internal/session"
)

// L'apparition d'un clawd : un portail s'ouvre depuis son centre puis se referme, et
// le clawd en tombe tout droit. Seules les sessions arrivées APRÈS le peuplement
// initial passent par un portail (sinon tout l'écran clignoterait au démarrage).
func TestPortalAppearance(t *testing.T) {
	// portail : fermé aux deux extrêmes, grand ouvert au milieu
	if portalScale(0) != 0 || portalScale(portalDur) != 0 {
		t.Error("le portail doit être fermé à t=0 et à la fin")
	}
	if s := portalScale((portalOpen + portalShut) / 2); s != 1 {
		t.Errorf("le portail doit être grand ouvert pendant la chute : %v", s)
	}
	// chute : rien avant fallStart, posé à fallEnd, entre les deux ça descend tout droit
	if _, show := fallTop(fallStart-time.Millisecond, 0, 20); show {
		t.Error("le clawd ne tombe pas avant fallStart")
	}
	if top, show := fallTop(fallEnd, -5, 20); !show || top != 20 {
		t.Errorf("posé à fallEnd : top=%d show=%v, attendu 20/true", top, show)
	}
	if mid, _ := fallTop((fallStart+fallEnd)/2, 0, 20); mid <= 0 || mid >= 20 {
		t.Errorf("à mi-chute : top=%d, attendu strictement entre 0 et 20", mid)
	}

	a := testApp(t, prefs.Prefs{HideIsland: true})
	a.portalFX, _ = avatar.LoadImage(assets.FS, avatar.FXDir+"/portal.png")
	a.ov, a.rng, a.cfg = &fakeOverlay{}, rand.New(rand.NewSource(1)), mood.DefaultConfig
	t0 := time.Now()
	a.store.Apply(session.Event{Session: "a", Name: "SessionStart", Cwd: "x"}, t0)
	a.update(t0) // peuplement initial : pas de portail (a.appeared passe à vrai ensuite)
	if !a.views[0].born.IsZero() {
		t.Error("le peuplement initial ne doit pas déclencher de portail")
	}

	// une session arrivée après coup naît d'un portail
	a.store.Apply(session.Event{Session: "b", Name: "SessionStart", Cwd: "y"}, t0.Add(time.Second))
	a.update(t0.Add(time.Second))
	i, vb := -1, (*view)(nil)
	for k, v := range a.views {
		if v.sess.ID == "b" {
			i, vb = k, v
		}
	}
	if vb == nil || vb.born.IsZero() {
		t.Fatal("la session b, arrivée après le peuplement, doit naître d'un portail")
	}
	// elle se dessine via la couche portail (une seule image transiente, dans la fenêtre)
	if ls := a.layout(i, vb); len(ls) != 1 || !ls[0].transient || !ls[0].img.Rect.In(image.Rect(0, 0, a.W/a.ui, a.H/a.ui)) {
		t.Errorf("l'arrivant doit se dessiner via la couche portail : %+v", ls)
	}
	// on ne la pointe pas tant qu'elle tombe (on vise le centre de son emplacement)
	bx := float32((a.slotsX0 + i*a.crowd.slot + spriteW/2) * a.ui)
	if _, ok := a.viewAt(bx, float32(a.ground-1)); ok {
		t.Error("un clawd qui arrive ne doit pas être pointable")
	}
}

// TestPortalGIF (diagnostic, via PORTAL_GIF=<gif>) rend l'animation d'apparition en
// GIF à fond transparent, en boucle — de quoi en garder une image dans art/.
func TestPortalGIF(t *testing.T) {
	out := os.Getenv("PORTAL_GIF")
	if out == "" {
		t.Skip("PORTAL_GIF non défini")
	}
	a := testApp(t, prefs.Prefs{HideIsland: true})
	a.portalFX, _ = avatar.LoadImage(assets.FS, avatar.FXDir+"/portal.png")
	a.ov, a.rng, a.cfg = &fakeOverlay{}, rand.New(rand.NewSource(1)), mood.DefaultConfig
	t0 := time.Now()
	a.store.Apply(session.Event{Session: "a", Name: "SessionStart", Cwd: "x"}, t0)
	a.update(t0) // pose la géométrie

	u := a.ui
	w, h := a.W/u, a.H/u
	cx0, cx1 := max(0, a.slotsX0-4), min(w, a.slotsX0+spriteW+4)
	cw := cx1 - cx0
	const scale, step = 6, 40 // ms : cadence réelle de l'animation
	var frames []*image.NRGBA
	for ms := 0; ms <= 1160; ms += step { // + une courte tenue sur le clawd posé
		buf := image.NewNRGBA(image.Rect(0, 0, w, h))
		a.drawPortal(buf, 0, time.Duration(ms)*time.Millisecond)
		frames = append(frames, buf)
	}
	pal := color.Palette{color.NRGBA{}} // index 0 : transparent
	seen := map[color.NRGBA]bool{}
	for _, f := range frames {
		for y := 0; y < h; y++ {
			for x := cx0; x < cx1; x++ {
				if c := f.NRGBAAt(x, y); c.A != 0 {
					rc := color.NRGBA{c.R, c.G, c.B, 255}
					if !seen[rc] {
						seen[rc] = true
						pal = append(pal, rc)
					}
				}
			}
		}
	}
	if len(pal) > 256 {
		t.Fatalf("%d couleurs, trop pour un GIF", len(pal))
	}
	g := &gif.GIF{LoopCount: 0, BackgroundIndex: 0}
	for _, f := range frames {
		p := image.NewPaletted(image.Rect(0, 0, cw*scale, h*scale), pal)
		for y := 0; y < h*scale; y++ {
			for x := 0; x < cw*scale; x++ {
				idx := uint8(0)
				if c := f.NRGBAAt(cx0+x/scale, y/scale); c.A != 0 {
					idx = uint8(pal.Index(color.NRGBA{c.R, c.G, c.B, 255}))
				}
				p.SetColorIndex(x, y, idx)
			}
		}
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, step/10)
		g.Disposal = append(g.Disposal, gif.DisposalBackground)
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(f, g); err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Logf("écrit %s : %d frames, %dx%d, %d couleurs", out, len(frames), cw*scale, h*scale, len(pal)-1)
}
