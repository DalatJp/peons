package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"
)

// maxLog : au-delà, le journal repart de zéro au prochain lancement.
const maxLog = 64 << 10

// logPath : sans console (lancé par le hook), c'est le seul endroit où lire pourquoi l'app s'est arrêtée.
func logPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "peons", "peons.log")
}

// logf ajoute une ligne au journal ; les échecs d'écriture sont ignorés, il n'y a personne à prévenir.
func logf(format string, args ...any) {
	path := logPath()
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o700)
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if st, err := os.Stat(path); err == nil && st.Size() > maxLog {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// logMemory consigne l'état du tas Go : si HeapAlloc monte sans cesse après les
// GC, quelque chose retient de la mémoire.
func logMemory(sessions, views int) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	logf("mémoire : tas %.1f Mo (en usage %.1f, système %.1f), %d GC, %d goroutines, %d sessions, %d vues",
		float64(m.HeapAlloc)/(1<<20), float64(m.HeapInuse)/(1<<20), float64(m.Sys)/(1<<20),
		m.NumGC, runtime.NumGoroutine(), sessions, views)
}

// logPanic consigne une panique de la goroutine principale puis termine le processus.
func logPanic() {
	if r := recover(); r != nil {
		logf("panique : %v\n%s", r, debug.Stack())
		os.Exit(1)
	}
}
