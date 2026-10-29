package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assistant(model string, input, create, read, output int, sidechain bool) string {
	return fmt.Sprintf(`{"type":"assistant","isSidechain":%v,"message":{"model":%q,"usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":%d}}}`,
		sidechain, model, input, create, read, output)
}

func write(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLatestTakesTheLastMainMessage(t *testing.T) {
	path := write(t,
		`{"type":"user","message":{"content":"salut"}}`,
		assistant("claude-opus-4-5", 10, 1000, 5000, 200, false),
		`{"type":"user","message":{"content":"encore"}}`,
		assistant("claude-opus-4-5", 12, 500, 60000, 300, false), // le dernier de la conversation
		assistant("claude-haiku", 1, 1, 1, 1, true),              // un sous-agent : ignoré
		assistant("<synthetic>", 99, 99, 99, 99, false),          // message synthétique : ignoré
		`pas du json mais avec "usage" dedans`,
	)
	u, ok := Latest(path)
	if !ok {
		t.Fatal("aucun usage trouvé")
	}
	if u.Tokens != 12+500+60000+300 || u.Window != DefaultWindow || u.Model != "claude-opus-4-5" {
		t.Errorf("usage %+v", u)
	}
	if f := u.Fraction(); f < 0.30 || f > 0.31 {
		t.Errorf("fraction %.3f, attendu ≈ 0,303", f)
	}
	if u.Remaining() != DefaultWindow-u.Tokens {
		t.Errorf("restant %d", u.Remaining())
	}
}

// Le titre est celui de l'onglet du terminal : le dernier nom (/rename) ou titre généré.
func TestScanFindsTheLastTitle(t *testing.T) {
	path := write(t,
		`{"type":"ai-title","aiTitle":"Premier titre généré","sessionId":"s"}`,
		assistant("claude-opus-4-5", 1, 2, 3, 4, false),
		`{"type":"agent-name","agentName":"  Nom donné à la main  ","sessionId":"s"}`,
		`{"type":"user","message":{"content":"un message qui parle d'\"ai-title\" et d'\"agent-name\""}}`,
	)
	info, ok := Scan(path)
	if !ok || info.Title != "Nom donné à la main" || !info.HasUsage || info.Usage.Tokens != 10 {
		t.Errorf("info %+v ok=%v", info, ok)
	}
	// sans nom donné, le titre généré le plus récent
	path = write(t,
		`{"type":"ai-title","aiTitle":"Ancien","sessionId":"s"}`,
		`{"type":"ai-title","aiTitle":"Plus récent","sessionId":"s"}`,
	)
	if info, ok := Scan(path); !ok || info.Title != "Plus récent" || info.HasUsage {
		t.Errorf("titre seul : %+v ok=%v", info, ok)
	}
	// un journal sans titre : l'usage seul
	if info, ok := Scan(write(t, assistant("m", 1, 1, 1, 1, false))); !ok || info.Title != "" || !info.HasUsage {
		t.Errorf("usage seul : %+v ok=%v", info, ok)
	}
}

func TestWindowFor(t *testing.T) {
	for _, tt := range []struct {
		model  string
		tokens int
		want   int
	}{
		{"claude-opus-4-5", 1000, DefaultWindow},
		{"claude-opus-4-5[1m]", 1000, LargeWindow},
		{"claude-sonnet-4", 250_000, LargeWindow}, // déjà au-delà de 200 000 : c'est une fenêtre d'un million
		{"", 0, DefaultWindow},
	} {
		if got := WindowFor(tt.model, tt.tokens); got != tt.want {
			t.Errorf("WindowFor(%q, %d) = %d, attendu %d", tt.model, tt.tokens, got, tt.want)
		}
	}
}

// Une fin de fichier énorme (sorties d'outils) ne doit pas cacher le dernier message.
func TestLatestReadsPastHugeLines(t *testing.T) {
	huge := `{"type":"user","message":{"content":"` + strings.Repeat("x", 600<<10) + `"}}`
	path := write(t, assistant("claude-opus-4-5", 1, 2, 3, 4, false), huge, huge)
	u, ok := Latest(path)
	if !ok || u.Tokens != 10 {
		t.Errorf("usage %+v ok=%v : le message précède 1,2 Mo de lignes d'outils, il doit être retrouvé", u, ok)
	}
}

func TestLatestRefusesWhatIsNotAJournal(t *testing.T) {
	if _, ok := Latest(filepath.Join(t.TempDir(), "absent.jsonl")); ok {
		t.Error("fichier absent")
	}
	txt := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(txt, []byte(assistant("m", 1, 1, 1, 1, false)), 0o600)
	if _, ok := Latest(txt); ok {
		t.Error("seuls les .jsonl sont lus")
	}
	if _, ok := Latest(write(t, `{"type":"user"}`)); ok {
		t.Error("un journal sans message de Claude n'a pas d'usage")
	}
	if u := (Usage{}); u.Fraction() != 0 || u.Remaining() != 0 {
		t.Error("usage vide")
	}
}

// Scan repère une annulation : le journal se termine sur un « [Request interrupted by
// user] ». Point clé : après l'annulation, Claude Code finalise quand même la réponse
// partielle (message assistant avec usage) — ça NE doit PAS lever l'annulation, sinon
// le clawd resterait à miner. Seul un vrai prompt de l'utilisateur la lève. Une mention
// en prose (message assistant avec usage) ne compte pas comme annulation.
func TestScanDetectsInterrupt(t *testing.T) {
	asst := assistant("claude-opus-4-5", 10, 0, 0, 5, false)
	interrupt := `{"type":"user","message":{"content":[{"type":"text","text":"[Request interrupted by user]"}]}}`
	toolCancel := `{"type":"user","message":{"content":[{"type":"tool_result","content":"[Request interrupted by user for tool use]"}]}}`
	toolResult := `{"type":"user","message":{"content":[{"tool_use_id":"x","type":"tool_result","content":"ok"}]}}`
	prompt := `{"type":"user","message":{"role":"user","content":"et maintenant ?"}}`
	prose := `{"type":"assistant","message":{"model":"m","usage":{"input_tokens":5,"output_tokens":1},"content":[{"type":"text","text":"j'ai écrit [Request interrupted by user] dans ma réponse"}]}}`
	tests := []struct {
		name  string
		lines []string
		want  bool
	}{
		{"fini sur annulation", []string{asst, interrupt}, true},
		{"annulation d'outil", []string{asst, toolCancel}, true},
		{"réponse partielle finalisée après ne lève pas l'annulation", []string{asst, interrupt, asst}, true},
		{"un résultat d'outil après ne lève pas l'annulation", []string{asst, interrupt, toolResult}, true},
		{"un vrai prompt lève l'annulation", []string{asst, interrupt, asst, prompt}, false},
		{"journal normal", []string{asst, asst}, false},
		{"mention en prose (avec usage)", []string{asst, prose}, false},
	}
	for _, tt := range tests {
		if info, _ := Scan(write(t, tt.lines...)); info.Interrupted != tt.want {
			t.Errorf("%s : Interrupted=%v, attendu %v", tt.name, info.Interrupted, tt.want)
		}
	}
}
