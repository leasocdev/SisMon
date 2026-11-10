# NetWatch 1.0

Monitorizează prin ICMP disponibilitatea echipamentelor de rețea grupate pe locații. Afișează starea în timp real, păstrează istoricul și calculează statistici. Totul este într-un singur executabil pentru Windows și Linux.

## Cerințe

- Windows 10/11 / Server 2016+ (amd64) sau Linux cu systemd (amd64, arm64).
- Acces ICMP de la server către echipamentele monitorizate.

## Pornire rapidă

```powershell
netwatch.exe set-password
netwatch.exe run            # în prim-plan, http://localhost:8501/
# sau ca serviciu (administrator):
netwatch.exe install
netwatch.exe start
```

Ghidul complet (Linux, HTTPS, reverse proxy, backup, actualizare, API) se află în [docs/operare.md](docs/operare.md).

> Interfața web este dezvoltată în Planul 2. Până atunci, aplicația expune API-ul JSON și fluxul SSE.

## Dezvoltare

Necesită Go ≥ 1.25.

```bash
go test ./...          # toate testele (include ICMP real pe loopback și testul de performanță)
go test -short ./...   # fără ICMP real și fără testul de performanță
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/netwatch
```

Structura codului este descrisă în `docs/superpowers/specs/2026-11-09-netwatch-design.md` (secțiunea 2).
