package claudecfg

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// Le plugin (plugin/hooks/hooks.json) doit écouter les mêmes événements, avec les
// mêmes matchers, que le bouton « Brancher » : sinon un utilisateur du plugin
// perdrait des événements sans le savoir.
func TestPluginHooksMatchEvents(t *testing.T) {
	data, err := os.ReadFile("../../plugin/hooks/hooks.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Hooks) != len(events) {
		t.Errorf("%d événements dans le plugin, %d dans claudecfg", len(file.Hooks), len(events))
	}
	for _, e := range events {
		entries := file.Hooks[e.Name]
		var matchers []string
		for _, en := range entries {
			if en.Matcher != "" {
				matchers = append(matchers, en.Matcher)
			}
			calls := false
			for _, h := range en.Hooks {
				calls = calls || slices.Equal(h.Args, []string{"hook"})
			}
			if !calls {
				t.Errorf("%s : aucun hook n'appelle « peons.exe hook »", e.Name)
			}
		}
		if !slices.Equal(matchers, e.Matchers) {
			t.Errorf("%s : matchers %v, attendu %v", e.Name, matchers, e.Matchers)
		}
	}
}
