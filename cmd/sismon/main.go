// Command sismon is the SisMon network monitor.
package main

import (
	"fmt"
	"io"
	"os"
	_ "time/tzdata"

	"sismon/internal/svc"
	"sismon/internal/version"
)

const usage = `Utilizare: sismon <comandă> [opțiuni]

Comenzi:
  run              Pornește monitorizarea în prim-plan
  install          Instalează serviciul de sistem (necesită administrator/root)
  uninstall        Elimină serviciul de sistem
  start | stop     Pornește / oprește serviciul
  status           Afișează starea serviciului
  set-password     Setează parola administratorului
  backup           Creează un backup imediat
  restore FIȘIER   Restaurează baza dintr-un backup
  demo             Pornește o demonstrație cu date simulate (director demo-data, port 8502)
  version          Afișează versiunea

Opțiuni comune: --data DIRECTOR, --listen ADRESĂ:PORT, --tls auto|files,
  --tls-cert FIȘIER, --tls-key FIȘIER, --trusted-proxy IP/CIDR (repetabil)
`

func main() {
	svc.SetupConsoleUTF8()
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "SisMon %s\n", version.Version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	case "run":
		return cmdRun(rest, stdout, stderr)
	case "install", "uninstall", "start", "stop", "status":
		return cmdService(cmd, rest, stdout, stderr)
	case "set-password":
		return cmdSetPassword(rest, stdin, stdout, stderr)
	case "backup":
		return cmdBackup(rest, stdout, stderr)
	case "restore":
		return cmdRestore(rest, stdout, stderr)
	case "demo":
		return cmdDemo(rest, stdout, stderr)
	}
	fmt.Fprintf(stderr, "Comandă necunoscută: %s\n\n%s", cmd, usage)
	return 2
}
