// soak : test de charge pour traquer les fuites. Envoie beaucoup d'événements
// à l'application qui tourne (sessions qui naissent, travaillent, finissent et
// disparaissent) ; on compare ensuite sa mémoire avant et après.
//
//	go run ./tools/soak -n 20000 -sessions 40
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"peons/internal/ipc"
	"peons/internal/session"
)

func main() {
	n := flag.Int("n", 10000, "nombre d'événements")
	sessions := flag.Int("sessions", 20, "sessions simultanées")
	pause := flag.Duration("pause", 0, "pause entre deux événements")
	flag.Parse()

	// le cycle d'une session, répété avec des identifiants toujours neufs
	cycle := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PermissionRequest",
		"PreToolUse", "PostToolUseFailure", "SubagentStop", "Stop", "SessionEnd"}
	step := make([]int, *sessions)
	gen := make([]int, *sessions)
	start, failed := time.Now(), 0
	for i := 0; i < *n; i++ {
		s := i % *sessions
		ev := session.Event{
			Session: fmt.Sprintf("soak-%d-%d", s, gen[s]),
			Name:    cycle[step[s]],
			Cwd:     `E:\Dev\soak`,
			PID:     uint32(1000 + s), // un PID inexistant : l'app doit les retirer d'elle-même
			Started: 1,
		}
		if err := ipc.Send(ev); err != nil {
			failed++
		}
		if step[s]++; step[s] == len(cycle) {
			step[s], gen[s] = 0, gen[s]+1
		}
		time.Sleep(*pause)
	}
	fmt.Printf("%d événements en %v, %d échecs\n", *n, time.Since(start).Round(time.Millisecond), failed)
	if failed > 0 {
		os.Exit(1)
	}
}
