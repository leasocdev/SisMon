// Command netwatch is the NetWatch network monitor.
package main

import (
	"fmt"
	"io"
	"os"
	_ "time/tzdata"

	"netwatch/internal/version"
)

const usage = `Utilizare: netwatch <comandă> [opțiuni]

Comenzi:
  run            Pornește monitorizarea în prim-plan
  install        Instalează serviciul de sistem
  uninstall      Elimină serviciul de sistem
  start | stop | status
  set-password   Setează parola administratorului
  backup         Creează un backup imediat
  restore FIȘIER Restaurează baza dintr-un backup
  version        Afișează versiunea
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "NetWatch %s\n", version.Version)
		return 0
	default:
		fmt.Fprintf(stdout, "Comandă necunoscută: %s\n\n%s", args[0], usage)
		return 2
	}
}
