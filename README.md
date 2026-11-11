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

## Interfața web

Deschideți `http://SERVER:8501/` în browser. Pagini:

| Pagină | Rol |
|---|---|
| `/` Monitorizare | carduri pe locații, filtre pe stări, căutare, panou de detalii (`/locatie/{id}`) |
| `/dispecerat` | ecran mare, fără meniu; antet cu logo, denumire, subtitlu, oră și dată; rotația grilă/hartă și schema de culori se setează în Administrare; `?rotatie=60` și `?tema=luminoasa` le suprascriu |
| `/harta` | hartă OpenStreetMap (prin proxy-ul serverului, cu cache); contururi locale când OSM nu răspunde |
| `/statistici` | disponibilitate (locația e indisponibilă doar când cad echipamentele primare), acoperire, incidente, MTTR (timp mediu de remediere), pe orice perioadă de până la 400 de zile |
| `/raport-24h` | situația curentă și întreruperile echipamentelor primare din ultimele 24 h, plus incidentele minore (echipamente secundare), pregătit pentru tipărire A4 |
| `/statistici` → CSV / Excel / PDF | exportul exact al datelor afișate |
| `/administrare` | locații și echipamente, import/export CSV și Excel, setări, identitate și logo, backup-uri, jurnal, diagnostic, parolă |

## Demonstrație

```powershell
netwatch.exe demo            # http://localhost:8502/, date simulate în .demo-data
```

12 locații din Moldova cu scenarii care evoluează (căderi, degradări, latență mare, erori de monitor). Parola de administrare este `netwatch-demo`. Directorul `demo-data` este separat de datele reale și poate fi șters oricând.

## Versiuni (release)

```bash
go run ./tools/release --version 1.5.0     # dist/netwatch-windows-amd64.exe, netwatch-linux-amd64, netwatch-linux-arm64, SHA256SUMS
go run ./tools/release --skip-web           # fără recompilarea interfeței
```

CI (`.github/workflows/ci.yml`) rulează testele cu `-race` pe Windows și Linux, ICMP real pe Linux (socket neprivilegiat și raw), verificarea tipurilor, testele unitare și end-to-end. La o etichetă `vX.Y.Z` construiește binarele și publică un GitHub Release.

## Dezvoltare

Necesită Go ≥ 1.25 și, pentru interfața web, Node.js ≥ 22 (doar la dezvoltare; executabilul nu are nevoie de Node).

```bash
cd web && npm ci && npm run build && cd ..   # compilează interfața în internal/webui/dist (încorporată în executabil)
go test ./...          # toate testele (include ICMP real pe loopback și testul de performanță)
go test -short ./...   # fără ICMP real și fără testul de performanță
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/netwatch
```

Front-end (în `web/`):

```bash
npm run dev      # Vite pe :5173, cu proxy către un server pornit pe :8501
npm run check    # verificare tipuri (svelte-check)
npm test         # teste unitare (Vitest)
npm run e2e      # teste end-to-end (Playwright + tools/e2eserver cu ping simulat); prima dată: npx playwright install chromium
```

Fără `npm run build`, executabilul pornește doar cu API-ul și afișează o pagină text la `/`.

Structura codului este descrisă în `docs/superpowers/specs/2026-11-09-netwatch-design.md` (secțiunea 2).
