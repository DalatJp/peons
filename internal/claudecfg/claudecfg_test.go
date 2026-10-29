package claudecfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exe = `E:\Dev\peons\peons.exe`

func setup(t *testing.T, settings string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	if settings != "" {
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settings), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func settings(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallKeepsTheUsersSettings(t *testing.T) {
	dir := setup(t, `{
  "theme": "dark",
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "mon-script.sh", "timeout": 5}]}],
    "Custom": [{"x": 1}]
  },
  "model": "opus"
}`)
	changed, err := Install(dir, Command(exe))
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	got := settings(t, dir)
	// l'ordre des clés et les champs inconnus sont conservés
	if i, j, k := strings.Index(got, `"theme"`), strings.Index(got, `"hooks"`), strings.Index(got, `"model"`); !(i < j && j < k) {
		t.Errorf("ordre des clés modifié :\n%s", got)
	}
	for _, keep := range []string{"mon-script.sh", `"timeout": 5`, `"Custom"`, `"x": 1`} {
		if !strings.Contains(got, keep) {
			t.Errorf("%q a disparu :\n%s", keep, got)
		}
	}
	var parsed struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		want := max(1, len(e.Matchers))
		if e.Name == "Stop" {
			want++ // le script de l'utilisateur reste devant le nôtre
		}
		if n := len(parsed.Hooks[e.Name]); n != want {
			t.Errorf("%s : %d entrées, attendu %d", e.Name, n, want)
		}
	}
	if !Installed(dir) {
		t.Error("Installed doit être vrai")
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json.bak-peons")); err != nil {
		t.Error("la sauvegarde manque")
	}
}

func TestInstallIsIdempotentAndMovesTheCommand(t *testing.T) {
	dir := setup(t, "")
	if changed, err := Install(dir, Command(exe)); err != nil || !changed {
		t.Fatalf("première installation : changed=%v err=%v", changed, err)
	}
	before := settings(t, dir)
	if changed, err := Install(dir, Command(exe)); err != nil || changed {
		t.Errorf("seconde installation : changed=%v err=%v", changed, err)
	}
	moved := `E:\ailleurs\peons.exe`
	if _, err := Install(dir, Command(moved)); err != nil {
		t.Fatal(err)
	}
	after := settings(t, dir)
	if strings.Contains(after, `E:\\Dev\\peons`) || !strings.Contains(after, `E:\\ailleurs`) {
		t.Errorf("l'ancien chemin doit être remplacé :\n%s", after)
	}
	if strings.Count(after, "peons.exe") != strings.Count(before, "peons.exe") {
		t.Error("une réinstallation ne doit pas dupliquer les hooks")
	}
}

func TestInstalledCommandTellsWhichExecutable(t *testing.T) {
	dir := setup(t, `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"mon-script.sh"}]}]}}`)
	if got := InstalledCommand(dir); got != "" {
		t.Errorf("aucun hook de peons, mais %q", got)
	}
	Install(dir, Command(exe))
	if got := InstalledCommand(dir); got != Command(exe) {
		t.Errorf("commande %q, attendu %q", got, Command(exe))
	}
	moved := `E:\ailleurs\peons.exe`
	Install(dir, Command(moved))
	if got := InstalledCommand(dir); got != Command(moved) {
		t.Errorf("après déplacement : %q, attendu %q", got, Command(moved))
	}
	Uninstall(dir)
	if got := InstalledCommand(dir); got != "" {
		t.Errorf("après désinstallation : %q", got)
	}
}

func TestUninstallLeavesOthers(t *testing.T) {
	dir := setup(t, `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"mon-script.sh"}]}]},"theme":"dark"}`)
	Install(dir, Command(exe))
	if changed, err := Uninstall(dir); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	got := settings(t, dir)
	if strings.Contains(got, "peons") || !strings.Contains(got, "mon-script.sh") || !strings.Contains(got, `"theme"`) {
		t.Errorf("désinstallation incorrecte :\n%s", got)
	}
	if Installed(dir) {
		t.Error("Installed doit être faux")
	}
	// plus rien à retirer, et la clé hooks vide ne reste pas
	dir = setup(t, `{"theme":"dark"}`)
	Install(dir, Command(exe))
	Uninstall(dir)
	if strings.Contains(settings(t, dir), "hooks") {
		t.Error("une clé hooks vide doit disparaître")
	}
}

func TestUnreadableFileIsNeverRewritten(t *testing.T) {
	for name, content := range map[string]string{
		"json cassé":       `{"hooks": `,
		"hooks mal typé":   `{"hooks": []}`,
		"racine mal typée": `[1,2]`,
	} {
		dir := setup(t, content)
		if _, err := Install(dir, Command(exe)); err == nil {
			t.Errorf("%s : une erreur est attendue", name)
		}
		if got := settings(t, dir); got != content {
			t.Errorf("%s : le fichier a été modifié : %s", name, got)
		}
	}
}

func TestBOMIsAccepted(t *testing.T) {
	dir := setup(t, "\xEF\xBB\xBF{\"theme\":\"dark\"}")
	if _, err := Install(dir, Command(exe)); err != nil {
		t.Fatal(err)
	}
	if got := settings(t, dir); !strings.Contains(got, `"theme"`) || strings.HasPrefix(got, "\xEF") {
		t.Errorf("BOM mal géré :\n%s", got)
	}
}

func TestMissingConfigDir(t *testing.T) {
	if _, err := Install(filepath.Join(t.TempDir(), "absent"), Command(exe)); err == nil {
		t.Error("un dossier absent doit être signalé, pas créé")
	}
}
