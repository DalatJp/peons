// extract-eyes sépare les yeux du corps des sprites clawd.
//
// Entrée  : source/body/*.png (corps avec yeux dessinés).
// Sortie  : assets/characters/clawd/body/*.png (corps sans yeux),
//
//	assets/characters/clawd/eyes/{neutral,happy,sad,sleeping}.png,
//	assets/characters/clawd/manifest.json (ancre des yeux par frame).
//
// Les yeux sont les composantes de la couleur la plus sombre qui ne touchent
// ni le bord ni le vide (le contour du corps, lui, borde le vide), plus les
// reflets qui les jouxtent. Le programme recompose chaque frame et échoue si le
// résultat diffère d'un seul pixel de la source.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var (
	srcDir = flag.String("src", "tools/extract-eyes/source/body", "corps d'origine (avec yeux)")
	outDir = flag.String("out", "assets/characters/clawd", "dossier du personnage")

	force = flag.Bool("force", false, "réécrire body/*.png et eyes/*.png même s'ils existent (écrase les sprites retouchés à la main !)")
)

// body regroupe les frames d'un corps ; l'ordre est celui des fichiers.
type body struct {
	name   string
	frames []string
	manual bool // dessiné à la main dans assets/ : rien à extraire, l'entrée du manifest est conservée telle quelle
	period int  // période d'animation par défaut (ms), si le manifest n'en a pas
}

// bodies fixe les noms de corps et l'ordre des frames (l'animation les boucle).
var bodies = []body{
	{"normal", []string{"clawd_neutral", "clawd_neutral_1"}, false, 600},
	{"alert", []string{"clawd_alert", "clawd_alert_1"}, false, 350},
	{"sleeping", nil, true, 1200},  // sleep.png et sleep_1.png sont retouchés dans assets/
	{"compacting", nil, true, 500}, // neutral.png écrasé par le manifest (squash) sous le piston
	{"mining", nil, true, 300},     // mining.png, mining_1.png et ses fx (pioche, roche) sont dessinés à la main
}

// emotions : une image d'yeux par émotion. Pour l'instant toutes identiques à
// neutral ; elles existent pour pouvoir être redessinées une à une.
var emotions = []string{"neutral", "happy", "sad", "sleeping"}

type anchor struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type frameOut struct {
	Image string  `json:"image"`
	Eyes  *anchor `json:"eyes,omitempty"` // absent : pas d'yeux sur cette frame
}

type bodyOut struct {
	Period int        `json:"period"` // ms par frame ; réglable à la main, conservé
	Frames []frameOut `json:"frames"`
	FX     []string   `json:"fx,omitempty"` // écrit à la main : conservé tel quel
}

type manifestOut struct {
	Version int                        `json:"version"`
	Bodies  map[string]bodyOut         `json:"bodies"`
	Eyes    map[string]string          `json:"eyes"`
	FX      map[string]json.RawMessage `json:"fx,omitempty"` // écrit à la main : conservé tel quel
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "extract-eyes:", err)
		os.Exit(1)
	}
}

func run() error {
	// les calques fx se règlent à la main dans le manifest : on les reprend de l'existant
	var old manifestOut
	if raw, err := os.ReadFile(filepath.Join(*outDir, "manifest.json")); err == nil {
		if err := json.Unmarshal(raw, &old); err != nil {
			return fmt.Errorf("manifest existant: %w", err)
		}
	}
	mf := manifestOut{Version: 1, Bodies: map[string]bodyOut{}, Eyes: map[string]string{}, FX: old.FX}
	var eyes *image.NRGBA // l'image d'yeux, commune à toutes les frames
	for _, b := range bodies {
		if b.manual {
			mf.Bodies[b.name] = old.Bodies[b.name]
			fmt.Printf("%-16s (à la main, conservé)\n", b.name)
			continue
		}
		bo := bodyOut{FX: old.Bodies[b.name].FX, Period: cmpOr(old.Bodies[b.name].Period, b.period)}
		for _, name := range b.frames {
			src, err := load(filepath.Join(*srcDir, name+".png"))
			if err != nil {
				return err
			}
			clean, pair, at, err := split(src)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			rel := "body/" + strings.TrimPrefix(name, "clawd_") + ".png"
			// ces images sont retouchées à la main : on n'écrase jamais l'existant
			if _, err := os.Stat(filepath.Join(*outDir, rel)); err != nil || *force {
				if err := save(filepath.Join(*outDir, rel), clean); err != nil {
					return err
				}
				fmt.Println("écrit", rel)
			}
			fo := frameOut{Image: rel}
			if pair != nil {
				if eyes == nil {
					eyes = pair
				} else if !samePixels(eyes, pair) {
					return fmt.Errorf("%s: yeux différents des autres frames (à nommer séparément)", name)
				}
				fo.Eyes = &at
			}
			// recomposition : corps nettoyé + yeux doit redonner la source
			re := image.NewNRGBA(clean.Bounds())
			draw.Draw(re, re.Bounds(), clean, image.Point{}, draw.Src)
			if pair != nil {
				draw.Draw(re, pair.Bounds().Add(image.Pt(at.X-pair.Bounds().Dx()/2, at.Y)), pair, image.Point{}, draw.Over)
			}
			if !samePixels(re, src) {
				return fmt.Errorf("%s: la recomposition diffère de la source", name)
			}
			note := "sans yeux"
			if pair != nil {
				note = fmt.Sprintf("yeux %dx%d centrés en x=%d, haut y=%d", pair.Bounds().Dx(), pair.Bounds().Dy(), at.X, at.Y)
			}
			fmt.Printf("%-16s %2dx%-2d %s  recomposition identique\n", name, src.Bounds().Dx(), src.Bounds().Dy(), note)
			bo.Frames = append(bo.Frames, fo)
		}
		mf.Bodies[b.name] = bo
	}
	if eyes == nil {
		return fmt.Errorf("aucun œil trouvé")
	}
	for _, e := range emotions {
		rel := "eyes/" + e + ".png"
		// ces images sont redessinées à la main : on n'écrase jamais l'existant
		if _, err := os.Stat(filepath.Join(*outDir, rel)); err != nil || *force {
			if err := save(filepath.Join(*outDir, rel), eyes); err != nil {
				return err
			}
			fmt.Println("écrit", rel)
		}
		mf.Eyes[e] = rel
	}
	raw, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*outDir, "manifest.json"), append(raw, '\n'), 0o644)
}

// split renvoie le corps sans yeux, l'image d'yeux (nil s'il n'y en a pas) et
// sa position dans la frame.
func split(src *image.NRGBA) (clean, pair *image.NRGBA, at anchor, err error) {
	b := src.Bounds()
	fill, dark := palette(src)
	opaque := func(p image.Point) bool { return p.In(b) && src.NRGBAAt(p.X, p.Y).A != 0 }

	// composantes 4-connexes de la couleur sombre qui n'ont aucun voisin (8) vide
	seen := map[image.Point]bool{}
	var parts [][]image.Point
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := image.Pt(x, y)
			if seen[p] || src.NRGBAAt(x, y) != dark {
				continue
			}
			comp, enclosed := []image.Point{}, true
			stack := []image.Point{p}
			seen[p] = true
			for len(stack) > 0 {
				q := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				comp = append(comp, q)
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						n := q.Add(image.Pt(dx, dy))
						if !opaque(n) {
							enclosed = false
						} else if dx*dy == 0 && !seen[n] && src.NRGBAAt(n.X, n.Y) == dark {
							seen[n] = true
							stack = append(stack, n)
						}
					}
				}
			}
			if enclosed {
				parts = append(parts, comp)
			}
		}
	}
	clean = image.NewNRGBA(b)
	draw.Draw(clean, b, src, b.Min, draw.Src)
	if len(parts) == 0 {
		return clean, nil, anchor{}, nil
	}
	if len(parts) != 2 {
		return nil, nil, anchor{}, fmt.Errorf("%d composantes sombres isolées, 2 attendues", len(parts))
	}

	// pixels d'œil = pixels sombres + reflets (ni sombre ni fond) dans la boîte de chaque œil
	var eyePx []image.Point
	box := image.Rectangle{}
	for _, comp := range parts {
		r := image.Rectangle{Min: comp[0], Max: comp[0].Add(image.Pt(1, 1))}
		for _, p := range comp {
			r = r.Union(image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))})
		}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if c := src.NRGBAAt(x, y); c != fill {
					eyePx = append(eyePx, image.Pt(x, y))
				}
			}
		}
		box = box.Union(r)
	}
	pair = image.NewNRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	for _, p := range eyePx {
		pair.SetNRGBA(p.X-box.Min.X, p.Y-box.Min.Y, src.NRGBAAt(p.X, p.Y))
		clean.SetNRGBA(p.X, p.Y, fill)
	}
	// l'ancre est le centre horizontal de la paire (et son bord haut) : des yeux
	// plus larges ou plus étroits restent centrés au même endroit
	return clean, pair, anchor{box.Min.X + box.Dx()/2, box.Min.Y}, nil
}

// palette donne la couleur de remplissage du corps (la plus fréquente) et la
// plus sombre (contour et yeux).
func palette(img *image.NRGBA) (fill, dark color.NRGBA) {
	count := map[color.NRGBA]int{}
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if c := img.NRGBAAt(x, y); c.A != 0 {
				count[c]++
			}
		}
	}
	cols := make([]color.NRGBA, 0, len(count))
	for c := range count {
		cols = append(cols, c)
	}
	slices.SortFunc(cols, func(a, b color.NRGBA) int { return count[b] - count[a] })
	fill = cols[0]
	dark = cols[0]
	lum := func(c color.NRGBA) int { return int(c.R)*299 + int(c.G)*587 + int(c.B)*114 }
	for _, c := range cols {
		if lum(c) < lum(dark) {
			dark = c
		}
	}
	return
}

// cmpOr renvoie v, ou def si v est nul.
func cmpOr(v, def int) int {
	if v != 0 {
		return v
	}
	return def
}

func samePixels(a, b *image.NRGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ca, cb := a.NRGBAAt(x, y), b.NRGBAAt(x, y)
			if ca.A == 0 && cb.A == 0 {
				continue
			}
			if ca != cb {
				return false
			}
		}
	}
	return true
}

func load(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out, nil
}

func save(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
