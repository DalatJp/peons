// Package terrain assemble l'île qui porte les bonhommes à partir de morceaux
// dessinés : une extrémité gauche, autant de sections centrales qu'il faut (chacune
// l'une des variantes, au hasard), une extrémité droite. Toutes les images ont la
// même hauteur ; les sections centrales ont la même largeur.
//
// terrain.json, à côté des images :
//
//	{
//	  "left": "island_left.png", "right": "island_right.png",
//	  "centers": ["island_center.png", "island_center_1.png", "island_center_2.png"],
//	  "feet": 9,
//	  "guard": 12
//	}
//
// feet est la ligne de l'image où reposent les pieds des bonhommes (0 : le haut) ;
// guard, la largeur de la zone de garde entre le bord de l'île et eux.
package terrain

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io/fs"
	"path"
)

type manifest struct {
	Left    string   `json:"left"`
	Right   string   `json:"right"`
	Centers []string `json:"centers"`
	Feet    int      `json:"feet"`
	Guard   int      `json:"guard"`
}

// Terrain est immuable une fois chargé.
type Terrain struct {
	Left, Right *image.NRGBA
	Centers     []*image.NRGBA
	// Feet : la ligne où reposent les pieds, comptée depuis le haut de l'île.
	Feet int
	// Guard : largeur de la zone de garde, en pixels, entre le bord de l'île et le
	// premier ou le dernier bonhomme : ils ne doivent pas se tenir sur la pente du bord.
	Guard int
}

// Load lit dir/terrain.json et les images qu'il nomme.
func Load(fsys fs.FS, dir string) (*Terrain, error) {
	raw, err := fs.ReadFile(fsys, path.Join(dir, "terrain.json"))
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("terrain.json : %w", err)
	}
	if len(m.Centers) == 0 {
		return nil, fmt.Errorf("terrain.json : aucune section centrale")
	}
	read := func(name string) (*image.NRGBA, error) {
		f, err := fsys.Open(path.Join(dir, name))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("%s : %w", name, err)
		}
		out := image.NewNRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
		draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
		return out, nil
	}
	t := &Terrain{Feet: m.Feet, Guard: m.Guard}
	if t.Left, err = read(m.Left); err != nil {
		return nil, err
	}
	if t.Right, err = read(m.Right); err != nil {
		return nil, err
	}
	for _, name := range m.Centers {
		c, err := read(name)
		if err != nil {
			return nil, err
		}
		t.Centers = append(t.Centers, c)
	}
	h, w := t.Left.Bounds().Dy(), t.Centers[0].Bounds().Dx()
	for i, img := range append([]*image.NRGBA{t.Right}, t.Centers...) {
		if img.Bounds().Dy() != h {
			return nil, fmt.Errorf("terrain : les images doivent avoir la même hauteur (%d) ; la %de fait %d", h, i+1, img.Bounds().Dy())
		}
	}
	for _, c := range t.Centers {
		if c.Bounds().Dx() != w {
			return nil, fmt.Errorf("terrain : les sections centrales doivent avoir la même largeur (%d), l'une fait %d", w, c.Bounds().Dx())
		}
	}
	if t.Guard < 0 {
		return nil, fmt.Errorf("terrain : guard=%d ne peut pas être négatif", t.Guard)
	}
	if t.Feet < 0 || t.Feet >= h {
		return nil, fmt.Errorf("terrain : feet=%d hors de l'image (hauteur %d)", t.Feet, h)
	}
	return t, nil
}

// Height est la hauteur des images, en pixels.
func (t *Terrain) Height() int { return t.Left.Bounds().Dy() }

// CenterWidth est la largeur d'une section centrale.
func (t *Terrain) CenterWidth() int { return t.Centers[0].Bounds().Dx() }

// EdgesWidth est la largeur des deux extrémités ensemble.
func (t *Terrain) EdgesWidth() int { return t.Left.Bounds().Dx() + t.Right.Bounds().Dx() }

// Width donne la largeur de l'île faite de n sections centrales.
func (t *Terrain) Width(n int) int { return t.EdgesWidth() + max(n, 0)*t.CenterWidth() }

// SectionsFor donne le plus petit nombre de sections centrales (au moins 1) pour
// que l'île couvre au moins width pixels.
func (t *Terrain) SectionsFor(width int) int {
	need := width - t.EdgesWidth()
	if need <= 0 {
		return 1
	}
	return (need + t.CenterWidth() - 1) / t.CenterWidth()
}

// Build assemble l'île : l'extrémité gauche, une section centrale par élément de
// picks (l'indice de la variante, pris modulo leur nombre), l'extrémité droite.
func (t *Terrain) Build(picks []int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, t.Width(len(picks)), t.Height()))
	x := 0
	put := func(img *image.NRGBA) {
		draw.Draw(out, img.Bounds().Add(image.Pt(x, 0)), img, image.Point{}, draw.Src)
		x += img.Bounds().Dx()
	}
	put(t.Left)
	for _, p := range picks {
		put(t.Centers[((p%len(t.Centers))+len(t.Centers))%len(t.Centers)])
	}
	put(t.Right)
	return out
}
