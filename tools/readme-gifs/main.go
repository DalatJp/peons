// readme-gifs régénère les GIF du README (docs/img/*.gif) à partir des vrais
// assets embarqués : le personnage (internal/avatar), l'île (internal/terrain), la
// désagrégation (internal/snap) et les sprites du portail et des sous-agents. Rien
// n'est dessiné à la main : chaque image est composée comme l'application le fait
// (cmd/peons : geometry.go, run.go), hors écran, en Go pur.
//
//	go run ./tools/readme-gifs            # écrit dans docs/img
//	go run ./tools/readme-gifs -out <dossier>
//
// cmd/peons est un package main : la composition de la fenêtre (disposition de
// l'île, placement des bonhommes, portail, particules de sous-agents) y est
// reproduite ici à l'identique plutôt qu'importée ; si elle change là-bas, la
// refléter ici.
//
// Les GIF sont agrandis au plus proche sur un fond uni (la transparence d'un GIF est
// à 1 bit : l'île et la poussière y seraient crénelées, et un fond sombre mangerait
// la poussière). Chaque image n'embarque que la zone modifiée depuis la précédente.
package main

import (
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"peons/assets"
	"peons/internal/avatar"
	"peons/internal/session"
	"peons/internal/snap"
	"peons/internal/terrain"
)

// Constantes de cmd/peons (run.go, geometry.go) reprises telles quelles.
const (
	spriteW     = 32
	islandPad   = 4
	maxSections = 60

	portalDur    = 900 * time.Millisecond
	portalOpen   = 280 * time.Millisecond
	portalShut   = 680 * time.Millisecond
	fallStart    = 200 * time.Millisecond
	fallEnd      = 650 * time.Millisecond
	agentBase    = 29
	agentStag    = 3
	agentStep    = 10
	agentCell    = 7
	step         = 50 * time.Millisecond // pas d'échantillonnage : le pgcd des périodes du manifest
	dustStep     = 40 * time.Millisecond // cadence de la désagrégation et du portail dans l'app
	topMargin    = 2                     // pixels d'art au-dessus de ce que dessine le personnage
	bottomMargin = 3                     // pixels d'art sous l'île
)

// background : fond uni, ni noir (la poussière brune disparaîtrait) ni blanc (les
// bulles et le panneau blancs aussi) : un ciel moyen, lisible sur GitHub clair et sombre.
var background = color.NRGBA{0x9f, 0xc3, 0xdf, 0xff}

// world regroupe tout ce qu'il faut pour composer une scène.
type world struct {
	char    *avatar.Character
	terr    *terrain.Terrain
	portal  *image.NRGBA
	agents  map[session.AgentKind]*image.NRGBA
	slot    int             // avatar.Spacing : écart d'un corps au suivant
	reach   image.Rectangle // avatar.Reach
	epoch   time.Time       // « maintenant » pour le flottement des sous-agents
	topArt  int             // pixels d'art au-dessus de la ligne des pieds
	bodyTop int             // hauteur du corps neutre
}

func load() *world {
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	w := &world{epoch: time.Unix(1_800_000_000, 0)}
	var err error
	w.char, err = avatar.Load(assets.FS, "characters/clawd")
	must(err)
	w.terr, err = terrain.Load(assets.FS, "terrain")
	must(err)
	w.portal, err = avatar.LoadImage(assets.FS, avatar.FXDir+"/portal.png")
	must(err)
	w.agents = map[session.AgentKind]*image.NRGBA{}
	for kind, name := range map[session.AgentKind]string{
		session.AgentIdle: "agent_idle", session.AgentWorking: "agent_working", session.AgentThrottle: "agent_throttle",
	} {
		w.agents[kind], err = avatar.LoadImage(assets.FS, avatar.FXDir+"/"+name+".png")
		must(err)
	}
	w.slot, w.reach = w.char.Spacing(), w.char.Reach()
	ls, err := w.char.Layers("normal", w.char.At("normal", 0), "neutral")
	must(err)
	w.bodyTop = ls[0].Image.Bounds().Dy()
	// de quoi contenir toutes les poses (bulle, piston, roche qui tombe…), le portail
	// (au-dessus de la tête) et les particules de sous-agents
	w.topArt = max(-w.reach.Min.Y, w.bodyTop+2+w.portal.Bounds().Dy(), agentBase+agentStag+1+w.agents[session.AgentIdle].Bounds().Dy()) + topMargin
	return w
}

// geometry : disposition horizontale (geometry.go : islandGeometry), en pixels d'art.
type geometry struct {
	sections, islandW, w, islandX, slotsX0 int
}

func (wd *world) geometry(n int) geometry {
	t := wd.terr
	span := func(n int) int { return (n-1)*wd.slot + spriteW }
	g := geometry{sections: min(maxSections, t.SectionsFor(span(max(n, 1))+2*t.Guard))}
	g.islandW = t.Width(g.sections)
	first := (g.islandW - span(max(n, 1))) / 2
	left := min(0, first+wd.reach.Min.X)
	right := max(g.islandW, first+(max(n, 1)-1)*wd.slot+wd.reach.Max.X)
	g.w = right - left + 2*islandPad
	g.islandX = islandPad - left
	g.slotsX0 = g.islandX + first
	return g
}

// canvas : la fenêtre en pixels d'art, île posée, fond uni.
type canvas struct {
	wd       *world
	g        geometry
	w, h     int
	ground   int // ligne des pieds
	island   *image.NRGBA
	numSlots int
}

func (wd *world) newCanvas(n int) *canvas {
	c := &canvas{wd: wd, g: wd.geometry(n), numSlots: n}
	c.ground = wd.topArt
	c.w = c.g.w
	c.h = c.ground + wd.terr.Height() - wd.terr.Feet + bottomMargin
	// l'île : sections centrales variées mais fixes d'un rendu à l'autre
	picks := make([]int, c.g.sections)
	for i := range picks {
		picks[i] = (i*7 + 3) % len(wd.terr.Centers)
	}
	c.island = image.NewNRGBA(image.Rect(0, 0, c.w, c.h))
	src := wd.terr.Build(picks)
	draw.Draw(c.island, src.Bounds().Add(image.Pt(c.g.islandX, c.ground-wd.terr.Feet)), src, image.Point{}, draw.Src)
	return c
}

// base : le fond et l'île.
func (c *canvas) base() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.w, c.h))
	draw.Draw(img, img.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	draw.Draw(img, img.Bounds(), c.island, image.Point{}, draw.Over)
	return img
}

func (c *canvas) slotX(i int) int { return c.g.slotsX0 + i*c.wd.slot }

// actor dessine le bonhomme de l'emplacement i à l'instant t de la scène.
type actor func(c *canvas, dst *image.NRGBA, i int, t time.Duration)

// layers pose les calques d'une pose dans dst : le corps pose ses pieds sur la ligne du sol (run.go : place).
func (c *canvas) layers(dst *image.NRGBA, i int, body, emotion string, elapsed time.Duration) []avatar.Sprite {
	ch := c.wd.char
	ls, err := ch.Layers(body, ch.At(body, elapsed), emotion)
	if err != nil {
		panic(err)
	}
	x0, y0 := c.slotX(i), c.ground-ls[0].Image.Bounds().Dy()
	for _, l := range ls {
		draw.Draw(dst, l.Image.Bounds().Add(image.Pt(x0+l.At.X, y0+l.At.Y)), l.Image, image.Point{}, draw.Over)
	}
	return ls
}

type agent struct {
	id   string
	kind session.AgentKind
}

// pose : un bonhomme dans un corps et une émotion, éventuellement suivi de sous-agents.
// clock convertit le temps de la scène en temps écoulé depuis le début de l'animation.
func pose(body, emotion string, clock func(time.Duration) time.Duration, agents ...agent) actor {
	return func(c *canvas, dst *image.NRGBA, i int, t time.Duration) {
		c.layers(dst, i, body, emotion, clock(t))
		c.drawAgents(dst, i, agents, t)
	}
}

func since(offset time.Duration) func(time.Duration) time.Duration {
	return func(t time.Duration) time.Duration { return t + offset }
}

// drawAgents : une particule par sous-agent vivant, en ligne centrée au-dessus de la tête (run.go : layoutAgents).
func (c *canvas) drawAgents(dst *image.NRGBA, i int, agents []agent, t time.Duration) {
	if len(agents) == 0 {
		return
	}
	now := c.wd.epoch.Add(t)
	step := agentStep
	if n := len(agents); n > 1 {
		if avail := c.w - 2 - agentCell; avail < (n-1)*step {
			step = max(1, avail/(n-1))
		}
	}
	center := c.slotX(i) + spriteW/2
	rowW := (len(agents)-1)*step + agentCell
	start := max(1, min(center-rowW/2, c.w-1-rowW))
	for k, a := range agents {
		img := c.wd.agents[a.kind]
		pw, ph := img.Bounds().Dx(), img.Bounds().Dy()
		period, phase, stagger := agentHover(a.id)
		x := start + k*step + (agentCell-pw)/2
		top := agentBase + stagger + hoverUp(now, period, phase) + ph
		draw.Draw(dst, img.Bounds().Add(image.Pt(x, c.ground-top)), img, image.Point{}, draw.Over)
	}
}

// agentHover, hoverUp : voir run.go.
func agentHover(id string) (period time.Duration, phase float64, stagger int) {
	h := fnv.New32a()
	h.Write([]byte(id))
	v := h.Sum32()
	return time.Duration(700+v%800) * time.Millisecond, float64((v>>9)%1000) / 1000, int((v >> 20) % (agentStag + 1))
}

func hoverUp(now time.Time, period time.Duration, phase float64) int {
	if math.Mod(float64(now.UnixNano())/float64(period)+phase, 1) < 0.5 {
		return 1
	}
	return 0
}

// portalActor : un nouveau venu tombe d'un portail (run.go : drawPortal), puis reste en pose neutre.
func portalActor(start time.Duration) actor {
	return func(c *canvas, dst *image.NRGBA, i int, t time.Duration) {
		if t < start {
			return
		}
		t -= start
		if t >= portalDur { // installé : vue normale
			c.layers(dst, i, "normal", "neutral", t-portalDur)
			return
		}
		buf := image.NewNRGBA(dst.Rect)
		wd := c.wd
		slotX := c.slotX(i)
		centerX := slotX + spriteW/2
		ls, _ := wd.char.Layers("normal", wd.char.At("normal", 0), "neutral")
		landTop := c.ground - ls[0].Image.Bounds().Dy()
		ph := wd.portal.Bounds().Dy()
		cy := landTop - 2 - ph/2
		if sc := portalScale(t); sc > 0 {
			pw := wd.portal.Bounds().Dx()
			sw, sh := max(1, int(math.Round(float64(pw)*sc))), max(1, int(math.Round(float64(ph)*sc)))
			scaleBlit(buf, wd.portal, centerX-sw/2, cy-sh/2, sw, sh)
		}
		if top, show := fallTop(t, cy-ph/2, landTop); show {
			for _, l := range ls {
				r := l.Image.Bounds().Add(image.Pt(slotX+l.At.X, top+l.At.Y))
				draw.Draw(buf, r, l.Image, l.Image.Bounds().Min, draw.Over)
			}
		}
		draw.Draw(dst, dst.Rect, buf, image.Point{}, draw.Over)
	}
}

func portalScale(t time.Duration) float64 {
	switch {
	case t < portalOpen:
		x := float64(t) / float64(portalOpen)
		return 1 - (1-x)*(1-x)
	case t < portalShut:
		return 1
	case t < portalDur:
		x := float64(t-portalShut) / float64(portalDur-portalShut)
		return 1 - x*x
	}
	return 0
}

func fallTop(t time.Duration, startTop, landTop int) (int, bool) {
	switch {
	case t < fallStart:
		return 0, false
	case t >= fallEnd:
		return landTop, true
	}
	x := float64(t-fallStart) / float64(fallEnd-fallStart)
	return startTop + int(math.Round(float64(landTop-startTop)*x)), true
}

func scaleBlit(dst, src *image.NRGBA, dx, dy, dw, dh int) {
	sb := src.Bounds()
	for y := 0; y < dh; y++ {
		ty := dy + y
		if ty < 0 || ty >= dst.Rect.Dy() {
			continue
		}
		sy := sb.Min.Y + y*sb.Dy()/dh
		for x := 0; x < dw; x++ {
			sc := src.NRGBAAt(sb.Min.X+x*sb.Dx()/dw, sy)
			tx := dx + x
			if sc.A == 0 || tx < 0 || tx >= dst.Rect.Dx() {
				continue
			}
			if sc.A == 255 {
				dst.SetNRGBA(tx, ty, sc)
				continue
			}
			d := dst.NRGBAAt(tx, ty)
			ia := 255 - uint32(sc.A)
			dst.SetNRGBA(tx, ty, color.NRGBA{
				uint8((uint32(sc.R)*uint32(sc.A) + uint32(d.R)*ia) / 255),
				uint8((uint32(sc.G)*uint32(sc.A) + uint32(d.G)*ia) / 255),
				uint8((uint32(sc.B)*uint32(sc.A) + uint32(d.B)*ia) / 255),
				uint8(uint32(sc.A) + uint32(d.A)*ia/255),
			})
		}
	}
}

// snapActor : la pose reste figée jusqu'à hold, puis le bonhomme se désagrège (run.go : startDust).
func snapActor(body, emotion string, hold time.Duration, seed int64) actor {
	var s *snap.Snap
	return func(c *canvas, dst *image.NRGBA, i int, t time.Duration) {
		if t < hold {
			c.layers(dst, i, body, emotion, t)
			return
		}
		if s == nil {
			ch := c.wd.char
			ls, err := ch.Layers(body, ch.At(body, hold), emotion)
			if err != nil {
				panic(err)
			}
			var r image.Rectangle
			for _, l := range ls {
				r = r.Union(l.Image.Bounds().Add(l.At))
			}
			img := image.NewNRGBA(r)
			for _, l := range ls {
				draw.Draw(img, l.Image.Bounds().Add(l.At), l.Image, image.Point{}, draw.Over)
			}
			s = snap.New(img, image.Pt(r.Min.X, r.Min.Y-ls[0].Image.Bounds().Dy()), rand.New(rand.NewSource(seed)))
		}
		buf := image.NewNRGBA(dst.Rect)
		s.Draw(buf, image.Pt(c.slotX(i), c.ground), t-hold)
		draw.Draw(dst, dst.Rect, buf, image.Point{}, draw.Over)
	}
}

// ---------------------------------------------------------------------------
// Encodage

type clip struct {
	name  string
	scale int
	c     *canvas
	stp   time.Duration
	total time.Duration
	acts  []actor
	// remap : temps de la scène pour l'instant de lecture (pour accélérer un cycle) ; nil : identité
	remap func(time.Duration) time.Duration
}

func (cl *clip) render(t time.Duration) *image.NRGBA {
	img := cl.c.base()
	if cl.remap != nil {
		t = cl.remap(t)
	}
	for i, a := range cl.acts {
		a(cl.c, img, i, t)
	}
	return img
}

// quantize ramène les images à 255 couleurs au plus (l'indice 0 est le « transparent »
// des images partielles) : exactes si possible ; sinon on retire des bits de poids
// faible jusqu'à ce que ça tienne (la poussière en vol fait varier les teintes).
func quantize(frames []*image.NRGBA) {
	seen := func() int {
		set := map[[3]uint8]bool{}
		for _, f := range frames {
			for i := 0; i < len(f.Pix); i += 4 {
				set[[3]uint8{f.Pix[i], f.Pix[i+1], f.Pix[i+2]}] = true
			}
		}
		return len(set)
	}
	for drop := 1; drop < 8 && seen() > 255; drop++ {
		for _, f := range frames {
			for i := 0; i < len(f.Pix); i += 4 {
				for k := 0; k < 3; k++ {
					f.Pix[i+k] &^= 1<<drop - 1
				}
			}
		}
	}
}
func (cl *clip) encode(path string) {
	// 1. échantillonner, en regroupant les images identiques consécutives
	var frames []*image.NRGBA
	var durs []time.Duration
	for t := time.Duration(0); t < cl.total; t += cl.stp {
		f := cl.render(t)
		if n := len(frames); n > 0 && imagesEqual(frames[n-1], f) {
			durs[n-1] += cl.stp
			continue
		}
		frames = append(frames, f)
		durs = append(durs, cl.stp)
	}
	quantize(frames)
	if *dump != "" { // vérification : une image PNG par image du GIF
		for n, f := range frames {
			big := image.NewNRGBA(image.Rect(0, 0, f.Rect.Dx()*cl.scale, f.Rect.Dy()*cl.scale))
			for y := 0; y < big.Rect.Dy(); y++ {
				for x := 0; x < big.Rect.Dx(); x++ {
					big.SetNRGBA(x, y, f.NRGBAAt(x/cl.scale, y/cl.scale))
				}
			}
			w, _ := os.Create(filepath.Join(*dump, fmt.Sprintf("%s_%03d.png", cl.name, n)))
			png.Encode(w, big)
			w.Close()
		}
	}
	// le regroupement peut être possible après quantification
	// 2. palette : indice 0 transparent, puis les couleurs
	pal := color.Palette{color.NRGBA{}}
	idx := map[color.NRGBA]uint8{}
	for _, f := range frames {
		for i := 0; i < len(f.Pix); i += 4 {
			c := color.NRGBA{f.Pix[i], f.Pix[i+1], f.Pix[i+2], 255}
			if _, ok := idx[c]; !ok {
				idx[c] = uint8(len(pal))
				pal = append(pal, c)
			}
		}
	}
	s := cl.scale
	b := frames[0].Rect
	out := &gif.GIF{LoopCount: 0, Config: image.Config{ColorModel: pal, Width: b.Dx() * s, Height: b.Dy() * s}, BackgroundIndex: 1}
	var prev *image.NRGBA
	for n, f := range frames {
		r := f.Rect // première image : tout
		if prev != nil {
			r = diffRect(prev, f)
			if r.Empty() {
				out.Delay[len(out.Delay)-1] += int(durs[n] / (10 * time.Millisecond))
				prev = f
				continue
			}
		}
		p := image.NewPaletted(image.Rect(r.Min.X*s, r.Min.Y*s, r.Max.X*s, r.Max.Y*s), pal)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				i := f.PixOffset(x, y)
				c := uint8(0) // inchangé : transparent
				if prev == nil || !samePx(prev, f, x, y) {
					c = idx[color.NRGBA{f.Pix[i], f.Pix[i+1], f.Pix[i+2], 255}]
				}
				for yy := 0; yy < s; yy++ {
					for xx := 0; xx < s; xx++ {
						p.SetColorIndex(x*s+xx, y*s+yy, c)
					}
				}
			}
		}
		out.Image = append(out.Image, p)
		out.Delay = append(out.Delay, int(durs[n]/(10*time.Millisecond)))
		out.Disposal = append(out.Disposal, gif.DisposalNone)
		prev = f
	}
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if err := gif.EncodeAll(file, out); err != nil {
		panic(err)
	}
	st, _ := file.Stat()
	fmt.Printf("%-24s %4dx%-4d %3d images %2d couleurs %6.1f Ko\n", filepath.Base(path), b.Dx()*s, b.Dy()*s, len(out.Image), len(pal)-1, float64(st.Size())/1024)
}

func samePx(a, b *image.NRGBA, x, y int) bool {
	i, j := a.PixOffset(x, y), b.PixOffset(x, y)
	return a.Pix[i] == b.Pix[j] && a.Pix[i+1] == b.Pix[j+1] && a.Pix[i+2] == b.Pix[j+2]
}

func imagesEqual(a, b *image.NRGBA) bool { return string(a.Pix) == string(b.Pix) }

func diffRect(a, b *image.NRGBA) image.Rectangle {
	var r image.Rectangle
	for y := 0; y < a.Rect.Dy(); y++ {
		for x := 0; x < a.Rect.Dx(); x++ {
			if !samePx(a, b, x, y) {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}

// ---------------------------------------------------------------------------

var dump = flag.String("dump", "", "dossier où écrire une image PNG par image de chaque GIF (vérification à l'œil)")

func main() {
	out := flag.String("out", filepath.Join("docs", "img"), "dossier de sortie")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		panic(err)
	}
	wd := load()
	one := wd.newCanvas(1)
	fmt.Printf("île : %d sections ; écart entre bonhommes : %d px d'art ; fenêtre d'un bonhomme : %dx%d px d'art\n",
		one.g.sections, wd.slot, one.w, one.h)

	// durée d'un tour complet de chaque corps (ppcm des périodes des corps et des effets), pas de 50 ms
	single := func(name string, scale int, total time.Duration, a actor) {
		(&clip{name: name, scale: scale, c: one, stp: step, total: total, acts: []actor{a}}).encode(filepath.Join(*out, name+".gif"))
	}
	t0 := since(0)
	single("normal", 6, 1200*time.Millisecond, pose("normal", "neutral", t0))
	single("content", 6, 1200*time.Millisecond, pose("normal", "happy", t0))
	single("triste", 6, 1200*time.Millisecond, pose("normal", "sad", t0))
	single("endormi", 6, 7200*time.Millisecond, pose("sleeping", "sleeping", t0))
	single("alert", 6, 700*time.Millisecond, pose("alert", "neutral", t0))
	single("compacting", 6, 1000*time.Millisecond, pose("compacting", "neutral", t0))
	single("bench", 6, 1000*time.Millisecond, pose("bench", "neutral", t0))
	single("portail", 6, 900*time.Millisecond+1600*time.Millisecond, portalActor(0))

	// mining : le minage, puis la roche qui se fend en temps accéléré, explose et retombe
	miningClip(wd, one).encode(filepath.Join(*out, "mining.gif"))

	// désagrégation : figé 1 s, 2,5 s de poussière, puis l'île vide
	(&clip{name: "snap", scale: 6, c: one, stp: dustStep, total: 1200*time.Millisecond + snap.Duration + 900*time.Millisecond,
		acts: []actor{snapActor("normal", "neutral", 1200*time.Millisecond, 7)}}).encode(filepath.Join(*out, "snap.gif"))

	// deux clawd, avec leurs sous-agents : l'un travaille (deux agents au travail, un muet), l'autre a fini (deux l'attendent)
	two := wd.newCanvas(2)
	(&clip{name: "sous-agents", scale: 6, c: two, stp: step, total: 7200 * time.Millisecond, acts: []actor{
		pose("mining", "neutral", t0, agent{"a1", session.AgentWorking}, agent{"a2", session.AgentWorking}, agent{"a3", session.AgentIdle}),
		pose("normal", "happy", since(300*time.Millisecond), agent{"a4", session.AgentThrottle}, agent{"a5", session.AgentThrottle}),
	}}).encode(filepath.Join(*out, "sous-agents.gif"))

	// la scène : cinq bonhommes, cinq états, une même île
	five := wd.newCanvas(5)
	(&clip{name: "scene", scale: 4, c: five, stp: step, total: 10800 * time.Millisecond, acts: []actor{
		pose("mining", "neutral", t0, agent{"s1", session.AgentWorking}, agent{"s2", session.AgentWorking}),
		pose("alert", "neutral", since(150*time.Millisecond)),
		pose("compacting", "neutral", since(250*time.Millisecond)),
		pose("normal", "happy", since(400*time.Millisecond), agent{"s3", session.AgentThrottle}),
		pose("sleeping", "sleeping", since(1200*time.Millisecond)),
	}}).encode(filepath.Join(*out, "scene.gif"))
}

// miningClip : un cycle de la roche (une minute de minage, comprimée : 0,6 s par palier de fissure), puis l'explosion.
func miningClip(wd *world, one *canvas) *clip {
	rock := wd.char.FX["mining_rock"]
	var stages []time.Duration // début de chaque palier de la roche (fissures), puis de l'explosion
	var at, burst time.Duration
	for i := range rock.Frames {
		if rock.Hold[i] {
			burst = at
			break
		}
		stages = append(stages, at)
		at += rock.Times[i]
	}
	var cycle time.Duration
	for _, d := range rock.Times {
		cycle += d
	}
	const stay = 900 * time.Millisecond // durée montrée de chaque palier de fissure
	intro := time.Duration(len(stages)) * stay
	total := intro + (cycle - burst) + 600*time.Millisecond
	remap := func(t time.Duration) time.Duration {
		if t < intro {
			return stages[int(t/stay)] + t%stay
		}
		return burst + (t - intro)
	}
	return &clip{name: "mining", scale: 6, c: one, stp: step, total: total, remap: remap, acts: []actor{pose("mining", "neutral", since(0))}}
}
