// Commande mining-gif régénère le GIF d'animation de minage : le clawd qui creuse
// (corps, yeux, rocher, pioche, bras), en boucle sur ses deux frames, fond
// transparent. Le rocher est figé — l'animation de casse ne démarre qu'après 48 s,
// donc rendre à t=0 et t=300 ms donne le rocher statique.
//
//	go run ./tools/mining-gif art/mining.gif
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"time"

	"peons/assets"
	"peons/internal/avatar"
)

const (
	scale  = 6  // pixels d'écran par pixel d'art (plus proche : pixels carrés)
	frames = 2  // les deux frames du minage
	delay  = 30 // centièmes de seconde par frame (300 ms, comme dans l'app)
	pad    = 1  // marge (pixels d'art) autour du contenu
	// toile de composition : coordonnées relatives au coin haut-gauche du corps, décalées.
	canvas = 64
	off    = 16
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mining-gif <sortie.gif>")
		os.Exit(2)
	}
	c, err := avatar.Load(assets.FS, "characters/clawd")
	if err != nil {
		panic(err)
	}

	// composer chaque frame dans une toile large
	raw := make([]*image.NRGBA, frames)
	for f := range raw {
		pose := c.At("mining", time.Duration(300*f)*time.Millisecond)
		if pose.FX[0] != 0 {
			panic("le rocher n'est plus sur sa frame statique") // garde-fou : pas d'effet de casse
		}
		ls, err := c.Layers("mining", pose, "neutral")
		if err != nil {
			panic(err)
		}
		img := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
		for _, l := range ls { // corps, yeux, rocher, pioche, bras (dans l'ordre)
			draw.Draw(img, l.Image.Bounds().Add(l.At).Add(image.Pt(off, off)), l.Image, image.Point{}, draw.Over)
		}
		raw[f] = img
	}

	// rectangle opaque, union des deux frames (même cadrage pour qu'elles s'alignent)
	var bbox image.Rectangle
	for _, img := range raw {
		for y := 0; y < canvas; y++ {
			for x := 0; x < canvas; x++ {
				if img.NRGBAAt(x, y).A != 0 {
					bbox = bbox.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
	}
	bbox = bbox.Inset(-pad).Intersect(image.Rect(0, 0, canvas, canvas))

	// palette : transparent en index 0, puis les couleurs distinctes (pixel art : peu nombreuses)
	pal := color.Palette{color.RGBA{}}
	seen := map[color.RGBA]bool{}
	for _, img := range raw {
		for y := bbox.Min.Y; y < bbox.Max.Y; y++ {
			for x := bbox.Min.X; x < bbox.Max.X; x++ {
				if px := img.NRGBAAt(x, y); px.A != 0 {
					rc := color.RGBA{px.R, px.G, px.B, 255}
					if !seen[rc] {
						seen[rc] = true
						pal = append(pal, rc)
					}
				}
			}
		}
	}
	if len(pal) > 256 {
		panic(fmt.Sprintf("%d couleurs, trop pour une palette GIF", len(pal)))
	}

	// encoder le GIF, agrandi au plus proche, fond transparent, boucle infinie
	out := &gif.GIF{LoopCount: 0, BackgroundIndex: 0}
	w, h := bbox.Dx()*scale, bbox.Dy()*scale
	for _, img := range raw {
		p := image.NewPaletted(image.Rect(0, 0, w, h), pal)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				src := img.NRGBAAt(bbox.Min.X+x/scale, bbox.Min.Y+y/scale)
				idx := uint8(0) // transparent
				if src.A != 0 {
					idx = uint8(pal.Index(color.RGBA{src.R, src.G, src.B, 255}))
				}
				p.SetColorIndex(x, y, idx)
			}
		}
		out.Image = append(out.Image, p)
		out.Delay = append(out.Delay, delay)
		out.Disposal = append(out.Disposal, gif.DisposalBackground)
	}
	f, err := os.Create(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, out); err != nil {
		panic(err)
	}
	fmt.Printf("écrit %s : %d frames, %dx%d, %d couleurs\n", os.Args[1], frames, w, h, len(pal)-1)
}
