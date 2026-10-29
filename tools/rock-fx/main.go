// rock-fx tire de assets/fx/rock.png les frames du cycle de la roche du mineur : des
// fissures qui gagnent pendant une minute, l'explosion en éclats (le mineur se
// fige), les éclats qui retombent, une roche neuve qui tombe du haut de la fenêtre
// puis un peu de poussière à l'atterrissage, et le mineur reprend.
//
// Il réécrit assets/fx/rock_*.png (ces fichiers lui appartiennent : il efface les
// anciens) et l'entrée "mining_rock" du manifest. À relancer après avoir retouché
// rock.png, ou pour tirer d'autres fissures (-seed) :
//
//	go run ./tools/rock-fx
//
// La roche reste où le manifest la pose (sa première frame). La chute part du haut
// de la fenêtre et les éclats restent près de la roche : ni la fenêtre ni l'écart
// entre les clawd ne changent, ce que le programme vérifie en relisant le résultat.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"peons/internal/avatar"
)

var (
	root = flag.String("assets", "assets", "dossier des assets (personnage dans characters/clawd, effets dans fx)")
	seed = flag.Int64("seed", 1, "graine du hasard (fissures, éclats, poussière)")
)

const (
	effect = "mining_rock"
	// stages : la roche intacte puis 4 états de fissures, stageMS chacun : une minute
	// en tout. Un multiple du coup de pioche (2 frames de 300 ms) : chaque fissure et
	// l'explosion tombent sur un coup.
	stages  = 5
	stageMS = 12000

	burstN   = 8 // frames d'explosion
	burstMS  = 50
	settleMS = 250 // les éclats retombés, avant la roche suivante
	fallN    = 9   // la chute, en fallN pas ; le dernier est l'atterrissage
	fallMS   = 40

	chunks  = 20  // éclats
	dustN   = 24  // grains de poussière de l'explosion
	gravity = 1.0 // des éclats, en pixels par frame²
)

// pixmap : des pixels posés dans le repère de la frame du corps qui creuse.
type pixmap map[image.Point]color.NRGBA

// image rend les pixels dans une image à leur taille, et sa position ; vide : un
// pixel transparent à at.
func (p pixmap) image(at image.Point) (*image.NRGBA, image.Point) {
	var r image.Rectangle
	for q := range p {
		r = r.Union(image.Rect(q.X, q.Y, q.X+1, q.Y+1))
	}
	if r.Empty() {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1)), at
	}
	img := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for q, c := range p {
		img.SetNRGBA(q.X-r.Min.X, q.Y-r.Min.Y, c)
	}
	return img, r.Min
}

// frame : une frame de l'effet, telle qu'elle s'écrit dans le manifest.
type frame struct {
	file string // nom de l'image dans le dossier des effets, tel qu'il s'écrit dans le manifest
	at   image.Point
	ms   int
	hold bool
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "rock-fx :", err)
		os.Exit(1)
	}
}

func run() error {
	rng := rand.New(rand.NewSource(*seed))
	charDir := filepath.Join(*root, "characters", "clawd")
	fxDir := filepath.Join(*root, avatar.FXDir)
	rawManifest, err := os.ReadFile(filepath.Join(charDir, "manifest.json"))
	if err != nil {
		return err
	}
	var mf struct {
		FX map[string]struct {
			Frames []struct {
				X int `json:"x"`
				Y int `json:"y"`
			} `json:"frames"`
		} `json:"fx"`
	}
	if err := json.Unmarshal(rawManifest, &mf); err != nil {
		return fmt.Errorf("manifest : %w", err)
	}
	if len(mf.FX[effect].Frames) == 0 {
		return fmt.Errorf("manifest : pas d'effet %q", effect)
	}
	rest := image.Pt(mf.FX[effect].Frames[0].X, mf.FX[effect].Frames[0].Y) // où la roche repose
	rock, err := readPNG(filepath.Join(fxDir, "rock.png"))
	if err != nil {
		return err
	}
	c, err := avatar.Load(os.DirFS(*root), "characters/clawd")
	if err != nil {
		return err
	}
	miner := c.Bodies["mining"]
	if miner == nil || c.FX["mining_pickaxe"] == nil {
		return fmt.Errorf("le personnage n'a pas de corps mining ou pas de pioche")
	}
	bodyH := miner.Frames[0].Image.Bounds().Dy()
	ground := bodyH - 1 // la ligne des pieds

	// les bornes à ne pas dépasser : celles du personnage avec la roche seule, posée
	base := *c
	base.FX = map[string]*avatar.Effect{}
	for k, v := range c.FX {
		base.FX[k] = v
	}
	base.FX[effect] = &avatar.Effect{Frames: []avatar.Sprite{{Image: rock, At: rest}}, Period: avatar.DefaultPeriod}
	height := base.Height()
	top := bodyH - height // le haut de la fenêtre, au-dessus du corps qui creuse
	w, h := rock.Bounds().Dx(), rock.Bounds().Dy()
	box := image.Rect(rest.X-8, top, rest.X+w+3, rest.Y+h) // où les éclats peuvent aller

	local := pixmap{} // la roche, dans son propre repère
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if p := rock.NRGBAAt(x, y); p.A != 0 {
				local[image.Pt(x, y)] = p
			}
		}
	}
	dark, light := tones(local)
	impact := impactPoint(c.FX["mining_pickaxe"].Frames[0], rest, local)

	var frames []frame
	add := func(name string, p pixmap, at image.Point, ms int, hold bool) error {
		img, pos := p.image(at)
		if err := writePNG(filepath.Join(fxDir, name), img); err != nil {
			return err
		}
		frames = append(frames, frame{name, pos, ms, hold})
		return nil
	}
	old, _ := filepath.Glob(filepath.Join(fxDir, "rock_*.png"))
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return err
		}
	}

	// 1. les fissures : la roche intacte, puis de plus en plus fendue
	cracked := crack(local, impact, dark, light, rng)
	frames = append(frames, frame{"rock.png", rest, stageMS, false})
	for s := 1; s < stages; s++ {
		if err := add(fmt.Sprintf("rock_crack_%d.png", s), shift(cracked[s-1], rest), rest, stageMS, false); err != nil {
			return err
		}
	}

	// 2. l'explosion : la dernière roche fendue part en éclats, qui retombent
	burst, settled := explode(shift(cracked[len(cracked)-1], rest), impact.Add(rest), ground, box, light, rng)
	for k, p := range burst {
		if err := add(fmt.Sprintf("rock_burst_%d.png", k+1), p, rest, burstMS, true); err != nil {
			return err
		}
	}
	if err := add("rock_burst_rest.png", settled, rest, settleMS, true); err != nil {
		return err
	}

	// 3. la chute : la roche part d'au-dessus de la fenêtre et entre par son haut
	from := top - h
	g := 2 * float64(rest.Y-from) / (fallN * fallN)
	for k := 1; k < fallN; k++ {
		y := from + int(math.Round(g*float64(k*k)/2))
		switch {
		case y+h <= top: // encore hors de la fenêtre
			continue
		case y < top: // à moitié entrée : le haut est coupé
			p := pixmap{}
			for q, col := range local {
				if q.Y+y >= top {
					p[q.Add(image.Pt(rest.X, y))] = col
				}
			}
			if err := add(fmt.Sprintf("rock_fall_%d.png", k), p, rest, fallMS, true); err != nil {
				return err
			}
		default:
			frames = append(frames, frame{"rock.png", image.Pt(rest.X, y), fallMS, true})
		}
	}

	// 4. l'atterrissage : deux bouffées de poussière au pied de la roche
	mid := midTone(local)
	for k, puff := range [][]image.Point{
		{{-1, 0}, {-2, 0}, {-1, -1}, {w, 0}, {w + 1, 0}, {w, -1}},
		{{-3, 0}, {-2, -1}, {-3, -2}, {w + 2, 0}, {w + 1, -1}, {w + 2, -2}},
	} {
		p := shift(local, rest)
		for i, d := range puff {
			col := light
			if i%3 == 2 {
				col = mid
			}
			p[image.Pt(rest.X+d.X, ground+d.Y)] = col
		}
		if err := add(fmt.Sprintf("rock_land_%d.png", k+1), p, rest, []int{70, 90}[k], true); err != nil {
			return err
		}
	}

	if err := writeManifest(filepath.Join(charDir, "manifest.json"), rawManifest, frames); err != nil {
		return err
	}

	// relecture : la fenêtre n'a pas grandi
	c2, err := avatar.Load(os.DirFS(*root), "characters/clawd")
	if err != nil {
		return fmt.Errorf("relecture : %w", err)
	}
	if got := c2.Height(); got != height {
		return fmt.Errorf("la chute agrandit la fenêtre : %d px au-dessus du sol au lieu de %d", got, height)
	}
	total := 0
	for _, f := range frames {
		total += f.ms
	}
	fmt.Printf("%d frames, cycle de %.2f s (dont %.2f s figé), écart des clawd %d px\n",
		len(frames), float64(total)/1000, float64(total-stages*stageMS)/1000, c2.Spacing())
	return nil
}

// shift déplace une roche locale à sa place.
func shift(p pixmap, at image.Point) pixmap {
	out := pixmap{}
	for q, c := range p {
		out[q.Add(at)] = c
	}
	return out
}

func luma(c color.NRGBA) int { return 299*int(c.R) + 587*int(c.G) + 114*int(c.B) }

// tones : la couleur la plus sombre de la roche (les fissures) et une claire (leur
// bord éclairé, la poussière), au 85e centile.
func tones(p pixmap) (dark, light color.NRGBA) {
	var cs []color.NRGBA
	for _, c := range p {
		cs = append(cs, c)
	}
	sort.Slice(cs, func(i, j int) bool { return luma(cs[i]) < luma(cs[j]) })
	return cs[0], cs[len(cs)*85/100]
}

// midTone : la couleur médiane de la roche.
func midTone(p pixmap) color.NRGBA {
	var cs []color.NRGBA
	for _, c := range p {
		cs = append(cs, c)
	}
	sort.Slice(cs, func(i, j int) bool { return luma(cs[i]) < luma(cs[j]) })
	return cs[len(cs)/2]
}

// impactPoint : là où la pointe de la pioche (frame du coup) touche la roche, dans
// le repère de la roche : le milieu de la ligne la plus basse qu'elle recouvre. À
// défaut, le milieu du haut de la roche.
func impactPoint(pick avatar.Sprite, rest image.Point, rock pixmap) image.Point {
	low, xs := -1, []int{}
	b := pick.Image.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if pick.Image.NRGBAAt(x, y).A == 0 {
				continue
			}
			q := pick.At.Add(image.Pt(x, y)).Sub(rest)
			if _, ok := rock[q]; !ok {
				continue
			}
			if q.Y > low {
				low, xs = q.Y, xs[:0]
			}
			if q.Y == low {
				xs = append(xs, q.X)
			}
		}
	}
	if low < 0 {
		r := image.Rectangle{}
		for q := range rock {
			r = r.Union(image.Rect(q.X, q.Y, q.X+1, q.Y+1))
		}
		return image.Pt((r.Min.X+r.Max.X)/2, r.Min.Y+1)
	}
	sort.Ints(xs)
	return image.Pt(xs[len(xs)/2], low)
}

// crack rend les stages-1 états fendus de la roche : des fissures qui partent du
// point d'impact et s'allongent d'un état à l'autre (chaque état prolonge le
// précédent), puis, au dernier, deux éclats du bord sautent près de l'impact.
func crack(rock pixmap, impact image.Point, dark, light color.NRGBA, rng *rand.Rand) []pixmap {
	inside := func(q image.Point) bool { // à l'intérieur : la silhouette reste entière
		for _, d := range []image.Point{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			if _, ok := rock[q.Add(d)]; !ok {
				return false
			}
		}
		return true
	}
	taken := map[image.Point]bool{}
	type dir struct {
		d image.Point
		w int
	}
	// walk trace une fissure de n pixels au plus, pas à pas, sans toucher les autres
	walk := func(from image.Point, n int, dirs []dir) []image.Point {
		var path []image.Point
		at := from
		for len(path) < n {
			var opts []dir
			for _, d := range dirs {
				q := at.Add(d.d)
				if !inside(q) || taken[q] {
					continue
				}
				near := false // ne pas recoller à un pixel de fissure autre que celui d'où l'on vient
				for dy := -1; dy <= 1 && !near; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if r := q.Add(image.Pt(dx, dy)); r != at && taken[r] && (len(path) == 0 || r != path[len(path)-1]) {
							near = true
							break
						}
					}
				}
				if !near {
					opts = append(opts, d)
				}
			}
			if len(opts) == 0 {
				break
			}
			total := 0
			for _, o := range opts {
				total += o.w
			}
			k := rng.Intn(total)
			for _, o := range opts {
				if k -= o.w; k < 0 {
					at = at.Add(o.d)
					break
				}
			}
			taken[at] = true
			path = append(path, at)
		}
		return path
	}
	start := impact
	if !inside(start) {
		start = start.Add(image.Pt(0, 1))
	}
	taken[start] = true
	main := append([]image.Point{start}, walk(start, 11, []dir{{image.Pt(0, 1), 4}, {image.Pt(1, 1), 3}, {image.Pt(-1, 1), 2}, {image.Pt(1, 0), 1}})...)
	right := walk(main[len(main)/3], 6, []dir{{image.Pt(1, 0), 3}, {image.Pt(1, 1), 2}, {image.Pt(1, -1), 1}, {image.Pt(0, 1), 1}})
	left := walk(start, 4, []dir{{image.Pt(-1, 1), 3}, {image.Pt(-1, 0), 2}, {image.Pt(0, 1), 1}})
	late := walk(main[2*len(main)/3], 4, []dir{{image.Pt(-1, 1), 2}, {image.Pt(-1, 0), 2}, {image.Pt(1, 1), 1}})
	lit := map[image.Point]bool{} // le bord éclairé de chaque fissure, tiré une fois
	for _, q := range append(append(append(append([]image.Point{}, main...), right...), left...), late...) {
		if rng.Float64() < 0.35 {
			lit[q.Add(image.Pt(1, 0))] = true
		}
	}
	// part de chaque fissure visible, état par état
	reveal := [][4]float64{{0.3, 0, 0, 0}, {0.6, 0.4, 0, 0}, {1, 0.8, 0.6, 0}, {1, 1, 1, 1}}
	var out []pixmap
	for s, r := range reveal {
		p := pixmap{}
		for q, c := range rock {
			p[q] = c
		}
		shown := map[image.Point]bool{}
		for i, path := range [][]image.Point{main, right, left, late} {
			n := int(math.Ceil(r[i] * float64(len(path))))
			for _, q := range path[:min(n, len(path))] {
				shown[q] = true
			}
		}
		for q := range shown {
			if l := q.Add(image.Pt(1, 0)); lit[l] && !shown[l] && inside(l) && s >= 2 {
				p[l] = light
			}
		}
		for q := range shown {
			p[q] = dark
		}
		if s == len(reveal)-1 { // deux éclats du bord sautent, au plus près de l'impact
			var edge []image.Point
			for q := range rock {
				if !inside(q) {
					edge = append(edge, q)
				}
			}
			dist := func(q image.Point) int { d := q.Sub(impact); return d.X*d.X + d.Y*d.Y }
			sort.Slice(edge, func(i, j int) bool {
				if dist(edge[i]) != dist(edge[j]) {
					return dist(edge[i]) < dist(edge[j])
				}
				return edge[i].X < edge[j].X || edge[i].X == edge[j].X && edge[i].Y < edge[j].Y
			})
			for _, q := range edge[:min(2, len(edge))] {
				delete(p, q)
			}
		}
		out = append(out, p)
	}
	return out
}

// explode fait voler la roche en éclats depuis l'impact : burstN frames, puis ce
// qui reste au sol. Un éclat qui sort de box disparaît ; celui qui retombe se pose
// sur la ligne des pieds ; celui qui part sous terre (le bas de la roche, enfoncé)
// sans jamais en sortir disparaît aussi.
func explode(rock pixmap, origin image.Point, ground int, box image.Rectangle, light color.NRGBA, rng *rand.Rand) (frames []pixmap, settled pixmap) {
	pts := make([]image.Point, 0, len(rock))
	for q := range rock {
		pts = append(pts, q)
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].Y < pts[j].Y || pts[i].Y == pts[j].Y && pts[i].X < pts[j].X })
	// des graines éloignées les unes des autres ; chaque pixel va à la plus proche
	var seeds []image.Point
	for _, i := range rng.Perm(len(pts)) {
		ok := true
		for _, s := range seeds {
			if d := pts[i].Sub(s); d.X*d.X+d.Y*d.Y < 6 {
				ok = false
				break
			}
		}
		if ok {
			seeds = append(seeds, pts[i])
		}
		if len(seeds) == chunks {
			break
		}
	}
	type piece struct {
		pix            []image.Point
		vx, vy         float64
		off            image.Point
		bottom         int // ligne la plus basse, à sa place d'origine
		airborne, down bool
		gone           bool
	}
	pieces := make([]*piece, len(seeds))
	for i := range pieces {
		pieces[i] = &piece{bottom: math.MinInt}
	}
	for _, q := range pts {
		best := 0
		for i, s := range seeds {
			if d, b := q.Sub(s), q.Sub(seeds[best]); d.X*d.X+d.Y*d.Y < b.X*b.X+b.Y*b.Y {
				best = i
			}
		}
		p := pieces[best]
		p.pix = append(p.pix, q)
		p.bottom = max(p.bottom, q.Y)
	}
	for _, p := range pieces {
		var cx, cy float64
		for _, q := range p.pix {
			cx += float64(q.X)
			cy += float64(q.Y)
		}
		cx, cy = cx/float64(len(p.pix)), cy/float64(len(p.pix))
		dx, dy := cx-float64(origin.X), cy-float64(origin.Y)
		n := math.Max(math.Hypot(dx, dy), 1)
		speed := 1.0 + 1.5*rng.Float64()
		p.vx = dx/n*speed + 0.3 // loin de la pioche, qui vient de la gauche
		p.vy = -(2.2 + 2.0*rng.Float64()) + math.Min(dy/n, 0)*0.8
	}
	type grain struct {
		at     image.Point
		vx, vy float64
		life   int
		col    color.NRGBA
	}
	var dust []grain
	for i := 0; i < dustN; i++ {
		a := -math.Pi * rng.Float64() // vers le haut
		v := 2.2 + 1.6*rng.Float64()
		col := light
		if i%2 == 1 {
			col = rock[pts[rng.Intn(len(pts))]]
		}
		dust = append(dust, grain{pts[rng.Intn(len(pts)/2+1)], math.Cos(a) * v, math.Sin(a) * v, 2 + rng.Intn(3), col})
	}
	draw := func(p pixmap) {
		for _, pc := range pieces {
			if pc.gone {
				continue
			}
			for _, q := range pc.pix {
				p[q.Add(pc.off)] = rock[q]
			}
		}
	}
	for k := 1; k <= burstN; k++ {
		t := float64(k)
		for _, pc := range pieces {
			if pc.gone || pc.down {
				continue
			}
			off := image.Pt(int(math.Round(pc.vx*t)), int(math.Round(pc.vy*t+gravity*t*t/2)))
			falling := pc.vy+gravity*t > 0
			switch {
			case pc.bottom+off.Y <= ground:
				pc.airborne = true
			case pc.airborne: // retombé : posé sur la ligne des pieds
				off.Y, pc.down = ground-pc.bottom, true
			case falling: // jamais sorti de terre
				pc.gone = true
				continue
			}
			for _, q := range pc.pix {
				if r := q.Add(off); r.X < box.Min.X || r.X >= box.Max.X || r.Y < box.Min.Y {
					pc.gone = true
					break
				}
			}
			pc.off = off
		}
		p := pixmap{}
		draw(p)
		for _, g := range dust {
			if k > g.life {
				continue
			}
			r := g.at.Add(image.Pt(int(math.Round(g.vx*t)), int(math.Round(g.vy*t+0.6*t*t/2))))
			if r.In(box) && r.Y <= ground {
				p[r] = g.col
			}
		}
		frames = append(frames, p)
	}
	settled = pixmap{}
	for _, pc := range pieces {
		if pc.down && !pc.gone {
			for _, q := range pc.pix {
				settled[q.Add(pc.off)] = rock[q]
			}
		}
	}
	return frames, settled
}

func readPNG(name string) (*image.NRGBA, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s : %w", name, err)
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out, nil
}

func writePNG(name string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(name, buf.Bytes(), 0o644)
}

// writeManifest remplace l'entrée de l'effet dans le manifest, à la main pour ne
// pas toucher au reste du fichier (sa mise en forme est faite à la main).
func writeManifest(file string, raw []byte, frames []frame) error {
	key := []byte(`"` + effect + `": {`)
	i := bytes.Index(raw, key)
	if i < 0 {
		return fmt.Errorf("manifest : %s introuvable", key)
	}
	lineStart := bytes.LastIndexByte(raw[:i], '\n') + 1
	indent := string(raw[lineStart:i])
	end, depth := -1, 0
	for k := i + len(key) - 1; k < len(raw) && end < 0; k++ {
		switch raw[k] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				end = k + 1
			}
		}
	}
	if end < 0 {
		return fmt.Errorf("manifest : accolade de %s non fermée", key)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\"%s\": {\n%s  \"frames\": [\n", indent, effect, indent)
	for k, f := range frames {
		fmt.Fprintf(&b, "%s    { \"image\": %q, \"x\": %d, \"y\": %d, \"ms\": %d", indent, path.Clean(f.file), f.at.X, f.at.Y, f.ms)
		if f.hold {
			b.WriteString(`, "hold": true`)
		}
		b.WriteString(" }")
		if k < len(frames)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "%s  ]\n%s}", indent, indent)
	out := append(append(append([]byte{}, raw[:lineStart]...), b.String()...), raw[end:]...)
	return os.WriteFile(file, out, 0o644)
}
