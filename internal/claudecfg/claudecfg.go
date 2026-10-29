// Package claudecfg branche et débranche peons dans les hooks de Claude Code
// (settings.json).
//
// Le fichier appartient à l'utilisateur : on ne le réécrit que s'il est lisible
// et que son contenu change, on garde l'ordre de ses clés et tout ce qu'on ne
// comprend pas, et on écrit par fichier temporaire + renommage pour qu'un
// plantage ne laisse jamais un fichier tronqué.
package claudecfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Les événements écoutés ; Matcher : "" = pas de clé matcher, sinon les valeurs.
var events = []struct {
	Name     string
	Matchers []string
}{
	{"UserPromptSubmit", nil},
	{"SessionStart", nil},
	{"PreToolUse", []string{"*"}},
	{"PostToolUse", []string{"*"}},
	{"PostToolUseFailure", []string{"*"}},
	{"PermissionDenied", []string{"*"}},
	{"StopFailure", nil},
	{"PermissionRequest", []string{"*"}},
	{"PreCompact", []string{"auto", "manual"}},
	{"Stop", nil},
	{"SubagentStop", nil},
	{"SessionEnd", nil},
}

// ourCommand reconnaît nos commandes : `"…\peons.exe" hook`.
var ourCommand = regexp.MustCompile(`peons(\.exe)?"?\s+hook\s*$`)

// Dir est le dossier de configuration de Claude Code.
func Dir() (string, error) {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// Command est la commande de hook pour l'exécutable donné.
func Command(exe string) string { return `"` + exe + `" hook` }

// Install fait appeler command à chaque événement et retire d'abord toute trace
// d'une installation précédente (dossier déplacé, ancien hook). Renvoie vrai si
// le fichier a changé.
func Install(dir, command string) (bool, error) {
	return rewrite(dir, func(hooks *object) {
		strip(hooks)
		for _, e := range events {
			var entries []json.RawMessage
			hooks.get(e.Name, &entries)
			hook := map[string]any{"type": "command", "command": command}
			if len(e.Matchers) == 0 {
				entries = append(entries, raw(map[string]any{"hooks": []any{hook}}))
			}
			for _, m := range e.Matchers {
				entries = append(entries, raw(map[string]any{"matcher": m, "hooks": []any{hook}}))
			}
			hooks.set(e.Name, raw(entries))
		}
	})
}

// Uninstall retire les hooks de peons sans toucher aux autres.
func Uninstall(dir string) (bool, error) {
	return rewrite(dir, strip)
}

// Installed dit si un hook de peons est en place.
func Installed(dir string) bool {
	root, err := read(filepath.Join(dir, "settings.json"))
	if err != nil {
		return false
	}
	var hooks object
	root.get("hooks", &hooks)
	for _, name := range hooks.keys {
		var entries []json.RawMessage
		hooks.get(name, &entries)
		for _, e := range entries {
			if isOurs(e) {
				return true
			}
		}
	}
	return false
}

// InstalledCommand donne la commande du hook de peons qui est en place, ou ""
// s'il n'y en a pas : elle dit sur quel exécutable le hook pointe (déplacé ?).
func InstalledCommand(dir string) string {
	root, err := read(filepath.Join(dir, "settings.json"))
	if err != nil {
		return ""
	}
	var hooks object
	root.get("hooks", &hooks)
	for _, name := range hooks.keys {
		var entries []json.RawMessage
		hooks.get(name, &entries)
		for _, e := range entries {
			var h struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			}
			if json.Unmarshal(e, &h) != nil {
				continue
			}
			for _, c := range h.Hooks {
				if cmd := strings.TrimSpace(c.Command); ourCommand.MatchString(cmd) {
					return cmd
				}
			}
		}
	}
	return ""
}

func rewrite(dir string, edit func(hooks *object)) (bool, error) {
	if _, err := os.Stat(dir); err != nil {
		return false, fmt.Errorf("dossier de configuration Claude Code introuvable : %s", dir)
	}
	path := filepath.Join(dir, "settings.json")
	root, err := read(path)
	if err != nil {
		// un fichier illisible n'est jamais réécrit : on effacerait les réglages
		return false, fmt.Errorf("%s illisible, rien n'est modifié : %w", path, err)
	}
	before := compact(root)
	var hooks object
	if v, ok := root.vals["hooks"]; ok {
		if err := json.Unmarshal(v, &hooks); err != nil {
			return false, fmt.Errorf("%s : « hooks » n'est pas un objet, rien n'est modifié", path)
		}
	}
	edit(&hooks)
	if len(hooks.keys) == 0 {
		root.delete("hooks")
	} else {
		root.set("hooks", raw(&hooks))
	}
	after := compact(root)
	if bytes.Equal(before, after) {
		return false, nil
	}
	var out bytes.Buffer
	if err := json.Indent(&out, after, "", "  "); err != nil {
		return false, err
	}
	out.WriteByte('\n')
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak-peons", old, 0o600); err != nil {
			return false, err
		}
	}
	return true, writeAtomic(path, out.Bytes())
}

// strip retire de chaque événement les entrées qui appellent peons.
func strip(hooks *object) {
	for _, name := range append([]string(nil), hooks.keys...) {
		var entries []json.RawMessage
		hooks.get(name, &entries)
		kept := entries[:0]
		for _, e := range entries {
			if !isOurs(e) {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			hooks.delete(name)
		} else if len(kept) != len(entries) {
			hooks.set(name, raw(kept))
		}
	}
}

func isOurs(entry json.RawMessage) bool {
	var e struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(entry, &e) != nil {
		return false
	}
	for _, h := range e.Hooks {
		if ourCommand.MatchString(strings.TrimSpace(h.Command)) {
			return true
		}
	}
	return false
}

func read(path string) (*object, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &object{}, nil
	}
	if err != nil {
		return nil, err
	}
	// un settings.json enregistré par PowerShell commence par un BOM
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(data)) == 0 {
		return &object{}, nil
	}
	var o object
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // sans effet une fois renommé
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// compact donne le JSON de o sans espaces : deux contenus identiques se
// comparent même si leur mise en forme diffère.
func compact(o *object) []byte {
	b, _ := o.MarshalJSON()
	var out bytes.Buffer
	if json.Compact(&out, b) != nil {
		return b
	}
	return out.Bytes()
}

func raw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // ne contient que des types JSON
	}
	return b
}

// object est un objet JSON qui garde l'ordre de ses clés et le contenu brut de
// ses valeurs.
type object struct {
	keys []string
	vals map[string]json.RawMessage
}

func (o *object) get(key string, into any) {
	if v, ok := o.vals[key]; ok {
		json.Unmarshal(v, into) // un type inattendu laisse into vide
	}
}

func (o *object) set(key string, v json.RawMessage) {
	if o.vals == nil {
		o.vals = map[string]json.RawMessage{}
	}
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = v
}

func (o *object) delete(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *object) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*o = object{}
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		if err == nil {
			err = errors.New("objet JSON attendu")
		}
		return err
	}
	*o = object{vals: map[string]json.RawMessage{}}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return err
		}
		o.set(t.(string), v)
	}
	return nil
}

func (o *object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		b.Write(o.vals[k])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
