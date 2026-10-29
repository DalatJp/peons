// Package transcript estime le remplissage du contexte d'une session Claude Code
// à partir de son journal (un fichier JSONL, dont le chemin accompagne chaque
// événement de hook) : le dernier message de Claude y porte l'usage de tokens de
// l'appel qui l'a produit.
//
// Le journal n'indique pas la taille de la fenêtre de contexte : on la suppose de
// 200 000 tokens, ou d'un million pour un modèle « [1m] » ou dès que l'usage
// dépasse 200 000. C'est une estimation, pas la valeur de Claude Code.
package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

const (
	// DefaultWindow : la fenêtre de contexte supposée.
	DefaultWindow = 200_000
	// LargeWindow : celle des modèles à un million de tokens.
	LargeWindow = 1_000_000

	// On lit la fin du fichier, par paliers : les lignes d'outils peuvent être énormes.
	tailStart = 256 << 10
	tailMax   = 4 << 20
)

// Usage est le contexte après le dernier message de Claude.
type Usage struct {
	Tokens int    // tokens dans le contexte : entrée (avec cache) + sortie du dernier appel
	Window int    // taille supposée de la fenêtre
	Model  string // modèle du dernier message
}

// Fraction donne la part de la fenêtre occupée, entre 0 et 1.
func (u Usage) Fraction() float64 {
	if u.Window <= 0 {
		return 0
	}
	return min(1, float64(u.Tokens)/float64(u.Window))
}

// Remaining donne les tokens encore disponibles.
func (u Usage) Remaining() int { return max(0, u.Window-u.Tokens) }

// WindowFor suppose la fenêtre de contexte d'un modèle.
func WindowFor(model string, tokens int) int {
	if strings.Contains(strings.ToLower(model), "[1m]") || tokens > DefaultWindow {
		return LargeWindow
	}
	return DefaultWindow
}

type record struct {
	Type        string `json:"type"`
	AgentName   string `json:"agentName"` // type agent-name : le nom de la session (/rename)
	AITitle     string `json:"aiTitle"`   // type ai-title : le titre que Claude Code génère
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Model string `json:"model"`
		Usage *struct {
			Input         int `json:"input_tokens"`
			CacheCreation int `json:"cache_creation_input_tokens"`
			CacheRead     int `json:"cache_read_input_tokens"`
			Output        int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// Info est ce qu'on lit d'un journal : l'usage du dernier message de Claude, le
// titre de la session (celui que Claude Code montre dans l'onglet du terminal) et
// si le journal se termine sur une annulation.
type Info struct {
	Usage    Usage
	HasUsage bool
	Title    string // vide tant que la session n'en a pas
	// Interrupted : le dernier événement du journal est un « [Request interrupted by
	// user] ». Annuler un tour (Échap) n'envoie aucun hook ; c'est la seule trace.
	Interrupted bool
}

// Latest lit le journal et donne l'usage du dernier message de Claude de la
// conversation principale. ok est faux si le fichier est illisible ou n'en
// contient pas encore.
func Latest(path string) (u Usage, ok bool) {
	info, ok := Scan(path)
	return info.Usage, ok && info.HasUsage
}

// Scan lit la fin du journal : l'usage du dernier message de Claude de la
// conversation principale, et le dernier titre de session (nom donné par
// /rename, sinon titre généré). ok est faux si le fichier est illisible ou ne
// contient encore ni l'un ni l'autre.
func Scan(path string) (info Info, ok bool) {
	if !strings.HasSuffix(path, ".jsonl") {
		return Info{}, false // on ne lit que des journaux, quoi qu'on nous envoie
	}
	f, err := os.Open(path)
	if err != nil {
		return Info{}, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return Info{}, false
	}
	for size := int64(tailStart); ; size *= 4 {
		start := max(0, st.Size()-size)
		info = scan(f, start)
		// on élargit tant qu'aucun usage n'est trouvé (les titres, eux, reviennent souvent)
		if info.HasUsage || start == 0 || size >= tailMax {
			return info, info.HasUsage || info.Title != ""
		}
	}
}

// scan lit à partir de start : le dernier message de Claude portant un usage, et le dernier titre.
func scan(f *os.File, start int64) Info {
	var info Info
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return info
	}
	r := bufio.NewReaderSize(f, 64<<10)
	if start > 0 {
		r.ReadBytes('\n') // la première ligne est probablement coupée
	}
	for {
		line, err := r.ReadBytes('\n')
		switch {
		case bytes.Contains(line, []byte(`"usage"`)):
			var rec record
			if json.Unmarshal(line, &rec) == nil && rec.Type == "assistant" && !rec.IsSidechain &&
				rec.Message.Usage != nil && rec.Message.Model != "<synthetic>" {
				us := rec.Message.Usage
				if tokens := us.Input + us.CacheCreation + us.CacheRead + us.Output; tokens > 0 {
					info.Usage = Usage{Tokens: tokens, Window: WindowFor(rec.Message.Model, tokens), Model: rec.Message.Model}
					info.HasUsage = true
				}
				// On ne touche PAS à Interrupted ici : après une annulation, Claude Code
				// finalise quand même la réponse partielle (un message assistant avec usage).
				// Si ce message levait l'annulation, le clawd resterait à miner pour toujours.
			}
		case bytes.Contains(line, []byte("[Request interrupted by user")):
			// annulation (Échap). Le marqueur est dans un message « user » (texte ou
			// résultat d'outil) ; les mentions en prose de Claude portent un usage et
			// passent au-dessus. Seul un vrai prompt (ci-dessous) la lèvera.
			info.Interrupted = true
		case bytes.Contains(line, []byte(`"type":"user"`)) &&
			!bytes.Contains(line, []byte(`"tool_result"`)) && !bytes.Contains(line, []byte(`"tool_use_id"`)):
			// un vrai prompt de l'utilisateur ouvre un nouveau tour : l'annulation
			// précédente est finie (un résultat d'outil, lui, n'est pas un prompt).
			info.Interrupted = false
		case bytes.Contains(line, []byte(`"agent-name"`)) || bytes.Contains(line, []byte(`"ai-title"`)):
			var rec record
			if json.Unmarshal(line, &rec) == nil {
				if title := strings.TrimSpace(rec.AgentName + rec.AITitle); title != "" { // un seul des deux est renseigné
					info.Title = title
				}
			}
		}
		if err != nil {
			return info
		}
	}
}
