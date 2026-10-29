package proc

import "testing"

func TestSameTitle(t *testing.T) {
	for _, c := range []struct {
		tab, session string
		want         bool
	}{
		{"✳ notchi", "notchi", true},
		{"◐ notchi", "Notchi", true},
		{"✳ rag confidentialit", "rag confidentialité", true}, // titre d'onglet tronqué
		{"✳ mcp next", "notchi", false},
		{"", "notchi", false},
		{"✳ notchi", "", false},
	} {
		if got := SameTitle(c.tab, c.session); got != c.want {
			t.Errorf("SameTitle(%q, %q) = %v, attendu %v", c.tab, c.session, got, c.want)
		}
	}
}
