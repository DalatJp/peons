package terrain

import (
	"fmt"
	"image"
	"image/color"
	"testing"
	"testing/fstest"

	"peons/assets"
)

func load(t *testing.T) *Terrain {
	t.Helper()
	tr, err := Load(assets.FS, "terrain")
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestRealTerrainLoads(t *testing.T) {
	tr := load(t)
	if len(tr.Centers) != 5 {
		t.Errorf("%d sections centrales, attendu 5 variantes", len(tr.Centers))
	}
	if tr.Height() != 26 || tr.CenterWidth() != 16 || tr.EdgesWidth() != 52 {
		t.Errorf("hauteur %d, section %d, extrémités %d", tr.Height(), tr.CenterWidth(), tr.EdgesWidth())
	}
}

// Les sections se raccordent dans n'importe quel ordre : de part et d'autre de
// chaque jointure possible (gauche|centre, centre|centre, centre|droite), les
// colonnes ont les mêmes pixels opaques, sans demi-transparence.
func TestRealTerrainPiecesJoin(t *testing.T) {
	tr := load(t)
	opaque := func(img *image.NRGBA, x int) string {
		col := make([]byte, img.Bounds().Dy())
		for y := range col {
			switch a := img.NRGBAAt(x, y).A; a {
			case 0:
				col[y] = '.'
			case 255:
				col[y] = '#'
			default:
				t.Errorf("alpha %d en (%d,%d) : une jointure ne se raccorde pas sur un pixel à moitié transparent", a, x, y)
			}
		}
		return string(col)
	}
	edges := map[string]string{
		"bord droit de l'extrémité gauche":  opaque(tr.Left, tr.Left.Bounds().Dx()-1),
		"bord gauche de l'extrémité droite": opaque(tr.Right, 0),
	}
	for i, c := range tr.Centers {
		edges[fmt.Sprintf("bord gauche de la section %d", i)] = opaque(c, 0)
		edges[fmt.Sprintf("bord droit de la section %d", i)] = opaque(c, c.Bounds().Dx()-1)
	}
	want := edges["bord droit de l'extrémité gauche"]
	for name, got := range edges {
		if got != want {
			t.Errorf("%s : %s, attendu %s", name, got, want)
		}
	}
}

func TestWidthAndSectionsFor(t *testing.T) {
	tr := load(t)
	for n, want := range map[int]int{0: 52, 1: 68, 3: 100} {
		if got := tr.Width(n); got != want {
			t.Errorf("Width(%d) = %d, attendu %d", n, got, want)
		}
	}
	// au moins une section, puis juste assez pour couvrir la largeur demandée
	for width, want := range map[int]int{1: 1, 52: 1, 68: 1, 69: 2, 84: 2, 85: 3, 168: 8, 169: 8, 181: 9} {
		got := tr.SectionsFor(width)
		if got != want {
			t.Errorf("SectionsFor(%d) = %d, attendu %d", width, got, want)
		}
		if tr.Width(got) < width {
			t.Errorf("SectionsFor(%d) = %d : l'île de %d ne couvre pas", width, got, tr.Width(got))
		}
	}
}

// Les pièces se posent dans l'ordre : gauche, sections choisies, droite.
func TestBuildOrder(t *testing.T) {
	solid := func(w, h int, c color.NRGBA) *image.NRGBA {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
		return img
	}
	tr := &Terrain{
		Left:  solid(3, 2, color.NRGBA{1, 0, 0, 255}),
		Right: solid(4, 2, color.NRGBA{2, 0, 0, 255}),
		Centers: []*image.NRGBA{
			solid(2, 2, color.NRGBA{10, 0, 0, 255}),
			solid(2, 2, color.NRGBA{11, 0, 0, 255}),
			solid(2, 2, color.NRGBA{12, 0, 0, 255}),
		},
	}
	out := tr.Build([]int{2, 0, 1, 4, -1}) // 4 -> 1, -1 -> 2 (modulo)
	want := []uint8{1, 1, 1, 12, 12, 10, 10, 11, 11, 11, 11, 12, 12, 2, 2, 2, 2}
	if out.Bounds().Dx() != len(want) || out.Bounds().Dy() != 2 {
		t.Fatalf("taille %v, attendu %dx2", out.Bounds(), len(want))
	}
	for x, r := range want {
		if got := out.NRGBAAt(x, 1).R; got != r {
			t.Errorf("colonne %d : %d, attendu %d", x, got, r)
		}
	}
}

func TestLoadRejectsInconsistentPieces(t *testing.T) {
	bad := fstest.MapFS{"t/terrain.json": {Data: []byte(`{"left":"l.png","right":"r.png","centers":[],"feet":0}`)}}
	if _, err := Load(bad, "t"); err == nil {
		t.Error("une île sans section centrale doit être refusée")
	}
	if _, err := Load(bad, "absent"); err == nil {
		t.Error("un dossier sans terrain.json doit être refusé")
	}
}
