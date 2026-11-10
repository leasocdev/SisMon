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
| `/dispecerat` | ecran mare, fără meniu; `?rotatie=60` alternează grila cu harta, `?sunet=1` pornește sunetul, `?tema=luminoasa` |
| `/harta` | hartă OpenStreetMap (prin proxy-ul serverului, cu cache); contururi locale când OSM nu răspunde |
| `/statistici` | disponibilitate, acoperire, incidente, MTTR, pe orice perioadă de până la 400 de zile |
| `/raport-24h` | situația curentă și întreruperile ultimelor 24 h, pregătit pentru tipărire A4 |
| `/administrare` | locații și echipamente, setări, identitate, backup-uri, jurnal, diagnostic, parolă |

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
