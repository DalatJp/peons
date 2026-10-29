package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"os"
)

// portaltex recrée la texture du portail (ovale large, orienté vers le bas) à
// l'échelle des sprites, d'après la palette d'une image de référence (non versionnée).
func main() {
	const w, h = 32, 18
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(7))
	cx, cy := float64(w-1)/2, float64(h-1)/2
	rx, ry := float64(w)/2, float64(h)/2
	// du bord (cyan vif) vers le centre (bleu profond)
	bands := []color.NRGBA{
		{11, 227, 253, 255},
		{10, 155, 251, 255},
		{6, 94, 222, 255},
		{14, 73, 178, 255},
		{4, 55, 150, 255},
		{3, 40, 105, 255},
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			d := math.Sqrt(dx*dx + dy*dy) // 0 au centre, 1 au bord
			if d > 1 {
				continue
			}
			var c color.NRGBA
			switch {
			case d > 0.86:
				c = bands[0]
			case d > 0.68:
				c = bands[1]
			case d > 0.48:
				c = bands[2]
			case d > 0.30:
				c = bands[3]
			case d > 0.14:
				c = bands[4]
			default:
				c = bands[5]
			}
			if d > 0.9 { // anti-alias du bord
				c.A = uint8(math.Round(255 * (1 - (d-0.9)/0.1)))
			}
			if d < 0.82 && rng.Float64() < 0.18 { // mouchetures : aspect tourbillon
				if rng.Float64() < 0.5 {
					c.R, c.G, c.B = 10, 155, 251
				} else {
					c.R, c.G, c.B = 4, 55, 150
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	f, err := os.Create(os.Args[1])
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
	f.Close()
}
