# NetWatch 1.0 — Plan 3: import/export, logo, mod demo, release și CI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Completează NetWatch 1.0 cu importul de locații (CSV/XLSX), exporturile (CSV/XLSX/PDF), logo-ul, modul demonstrativ, build-urile de release și CI, plus problemele minore amânate din Planul 2.

**Architecture:** Pachete Go noi și pure: `internal/importer` (citire și validare, fără bază de date), `internal/export` (scriere CSV/XLSX/PDF dintr-un `stats.Report`), `internal/branding` (logo), `internal/demo` (scenarii pentru `icmp.Fake`). API-ul primește rute noi; interfața primește pagina de import, butoanele de export, logo-ul și bannerul demo. `tools/release` produce binarele; `.github/workflows/ci.yml` rulează `-race`, ICMP pe Linux și Playwright.

**Tech Stack:** Go 1.27, `github.com/xuri/excelize/v2` v2.11.1, `github.com/go-pdf/fpdf` v0.9.0, `golang.org/x/text` (charmap), font DejaVu Sans încorporat; Svelte 5; GitHub Actions; actionlint (verificare locală a workflow-ului).

**Spec:** `docs/superpowers/specs/2026-11-09-netwatch-design.md` (secțiunile 6, 7 — logo și exporturi, 9, 10 — CLI `demo`, 11 — build și CI).

**Branch:** `plan3-extras`, creat din `plan2-web` (`31fe176`).

## Global Constraints

- Texte pentru utilizator în română, cu ș/ț cu virgulă; orele în `Europe/Chisinau`.
- Dimensiune maximă cerere: 1 MB JSON; **5 MB pentru import și logo** (multipart).
- Import: coloane `location`, `address`, `lat`, `lon`, `device`, `host`, `kind`, `is_primary`, `notes` (antete insensibile la majuscule și diacritice, cu sinonime în română); separator detectat (`,` `;` tab); codificare UTF-8 cu/fără BOM, Windows-1250, Windows-1252; zecimală cu virgulă la coordonate; rânduri complet goale ignorate; maxim **1000 de rânduri** și **200 de locații** per fișier; `is_primary`: 1/0, da/nu, true/false, x/gol; fără coloană → primul echipament al fiecărei locații este primar; locațiile existente (după nume) → eroare în previzualizare; aplicarea într-o singură tranzacție.
- Exporturi statistice: CSV UTF-8 cu BOM, separator `;`; XLSX cu foile **Echipamente, Locații, Incidente, Perioadă și filtre**; PDF A4 landscape (rezumat, tabel echipamente, bare de stare locații, incidente; DejaVu Sans; număr de pagină și fus orar în subsol). Aceiași parametri ca `/api/stats`.
- Protecție la injectarea de formule **doar** în CSV/XLSX statistice (valori care încep cu `=`, `+`, `-`, `@`, tab, CR primesc prefixul `'`); exportul locațiilor scrie valorile neschimbate, iar în XLSX ca text explicit.
- Logo: PNG validat (structură, CRC, maximum **1024 × 1024**, **500 KB**), reencodat de server; servit la `GET /branding/logo`.
- `netwatch demo`: pinger simulat, director separat `demo-data`, scenarii care evoluează în timp, banner „MOD DEMONSTRATIV”.
- Build: `go run ./tools/release` (fără `make`) produce `netwatch-windows-amd64.exe`, `netwatch-linux-amd64`, `netwatch-linux-arm64` și sume de control; `CGO_ENABLED=0`.
- CI: `go test -race`, ICMP real pe Linux (`127.0.0.1`/`::1` reușit, TEST-NET eșuat), Playwright.
- CSP neschimbat; niciun HTML generat din text introdus de utilizator.
- Commit-urile se termină cu `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **CSV salvat din Excel cu setări regionale românești** (separator `;`, Windows-1250, ş/ţ cu sedilă, coordonate „46,99”): importul trebuie să le citească corect. Test în Task 1 cu un fișier construit byte cu byte.
2. **Nume de locație care arată ca o formulă** (`=HYPERLINK(...)`, `+1`, `@cmd`) în exporturile statistice: deschis în Excel, nu trebuie executat. Exportul locațiilor păstrează valoarea exactă, ca reimportul să fie fără pierderi. Teste în Task 3.
3. **Fișier XLSX corupt sau „bombă zip”, fișier binar redenumit `.csv`, 1001 rânduri**: răspuns 400 cu mesaj în română, niciodată 500 și niciun consum nelimitat de memorie. Teste în Task 1–2.
4. **PNG „bombă de decompresie”** (antet 50000×50000 în sub 500 KB) sau fișier care nu e PNG: respins înainte de decodare. Test în Task 5.
5. **Raport mare sau gol în PDF** (500 de echipamente, nume lungi cu diacritice; perioadă fără date): PDF valid, paginat, fără panică. Test în Task 4.

---

## File Structure

```
internal/importer/importer.go     Parse (CSV/XLSX → tabel), Build (tabel → previzualizare + LocationInput)
internal/importer/decode.go       separator, codificare, sinonime antete, normalizare ş/ţ → ș/ț
internal/store/locations.go       SaveLocations (mai multe locații, o tranzacție)
internal/api/admin_import.go      POST /api/admin/import/preview, /apply (multipart, 5 MB)
internal/export/csv.go            StatsCSV, LocationsCSV
internal/export/xlsx.go           StatsXLSX, LocationsXLSX
internal/export/pdf.go            StatsPDF
internal/export/fonts/            DejaVuSans.ttf, DejaVuSans-Bold.ttf, LICENSE
internal/export/meta.go           Meta (titlu, perioadă, filtre, logo) și formatări comune
internal/api/export.go            GET /api/export/{csv|xlsx|pdf}, GET /api/admin/export/locations.{csv|xlsx}
internal/branding/logo.go         Validate+reencode, Save, Delete, Path
internal/api/branding.go          POST/DELETE /api/admin/identity/logo, GET /branding/logo
internal/demo/demo.go             Seed, Scenario (pinger), Password
cmd/netwatch/commands.go          cmdDemo
tools/release/main.go             build release
.github/workflows/ci.yml          CI
web/src/pages/admin/Import.svelte, admin/Identity.svelte (logo), pages/Stats.svelte (export), components/Header.svelte (logo), components/DemoBanner.svelte
```

---

### Task 1: Importer — citire și validare (pachet pur)

**Files:**
- Create: `internal/importer/importer.go`, `internal/importer/decode.go`
- Test: `internal/importer/importer_test.go`
- Modify: `go.mod` (`golang.org/x/text`, `github.com/xuri/excelize/v2`)

**Interfaces:**
- Produces:
  - `type Table struct { Format, Encoding, Separator string; Columns map[string]int; Rows []RawRow }`, `type RawRow struct { Line int; Cells []string }`.
  - `func Parse(filename string, data []byte) (*Table, error)` — `.xlsx` (după extensie sau semnătura ZIP `PK\x03\x04`) prin excelize cu `Options{UnzipSizeLimit: 50<<20, UnzipXMLSizeLimit: 20<<20}`, prima foaie, `GetRows(..., excelize.Options{RawCellValue: true})`; altfel CSV. Erorile de fișier sunt `apperr.Invalid("file", …)`: „Fișierul nu poate fi citit ca CSV sau XLSX.”, „Lipsesc coloanele obligatorii: …”, „Fișierul are mai mult de 1000 de rânduri.”, „Fișierul este gol.”. Un fișier binar (octeți NUL în primele 8 KB) nu e acceptat ca CSV.
  - Coloane canonice și sinonime (comparate după `fold`: litere mici, fără diacritice, spații normalizate): `location` {location, locatie, locatia, nume locatie}; `address` {address, adresa}; `lat` {lat, latitudine, latitude}; `lon` {lon, lng, longitudine, longitude}; `device` {device, echipament, nume echipament, denumire echipament}; `host` {host, ip, adresa ip, ip/host, hostname}; `kind` {kind, tip, type}; `is_primary` {is_primary, primar, principal, primary}; `notes` {notes, observatii, note}. Obligatorii: `location`, `device`, `host`.
  - Detectarea separatorului: numărul de `;`, `,`, tab din linia de antet în afara ghilimelelor; maximul câștigă; la egalitate `;` > `,` > tab. CSV citit cu `encoding/csv` (`LazyQuotes`, `FieldsPerRecord = -1`).
  - Codificare: BOM UTF-8 → UTF-8; text UTF-8 valid → UTF-8; altfel se decodează ca Windows-1250 și ca Windows-1252 și se alege varianta cu mai multe litere românești (`ăâîșțşţĂÂÎȘȚŞŢ`); egalitate → 1252.
  - După decodare, `ş ţ Ş Ţ` (sedilă) devin `ș ț Ș Ț` (virgulă), în toate celulele.
  - `type CellError struct { Column string \`json:"column"\`; Message string \`json:"message"\` }`
  - `type PreviewRow struct { Line int \`json:"line"\`; Values map[string]string \`json:"values"\`; Errors []CellError \`json:"errors"\` }`
  - `type Preview struct { Format, Encoding, Separator string; Rows []PreviewRow; Locations, Devices, ErrorCount int; Valid bool }` (tag-uri JSON `snake_case`).
  - `func Build(t *Table, existing map[string]bool, s settings.Settings) (Preview, []domain.LocationInput)` — `existing` are cheile `fold(nume)` ale locațiilor active. Rânduri complet goale ignorate. Grupare după `location` (comparat cu `fold`), în ordinea primei apariții. Validări pe celulă: nume locație 1–120, echipament 1–120, `domain.ValidHost`, tip ≤ 50, coordonate (virgulă sau punct; lat −90…90, lon −180…180; ambele sau niciuna), `is_primary` din valorile admise (altfel „Valoare neacceptată pentru primar: …”), câmpurile locației (adresă, coordonate, observații) contradictorii între rânduri → eroare pe celula rândului ulterior („Diferă de rândul N pentru aceeași locație.”), locație existentă → eroare pe `location` („Locația există deja; importul nu modifică locațiile existente.”). La final, fiecare locație fără erori trece prin `LocationInput.Normalize(s)`; o eroare `devices.i.<câmp>` se atașează rândului echipamentului i, iar una fără index (de exemplu lipsa unui primar) primului rând al locației. Peste 200 de locații → `ErrorCount++` și eroare pe primul rând al locației 201 („Fișierul are mai mult de 200 de locații.”). `Valid = ErrorCount == 0 && len(locations) > 0`; când `Valid` e fals, al doilea rezultat e `nil`.
- Consumers: Task 2, Task 3 (reimport).

- [ ] **Step 1: Write the failing tests** (`importer_test.go`):
  - `TestParseExcelRomanian`: octeți Windows-1250 pentru `Locație;Adresă;Latitudine;Longitudine;Echipament;IP;Tip;Primar\r\nŞtefan Vodă;str. Ţării 1;46,51;29,66;Router;10.0.0.1;Router;da\r\n` (construiți cu `charmap.Windows1250.NewEncoder()`) → `Encoding == "windows-1250"`, `Separator == ";"`, iar după `Build`: locația `Ștefan Vodă` (virgulă), adresa `str. Țării 1`, `Lat == 46.51`.
  - `TestParseUTF8BOMComma` (BOM + `,` + ghilimele cu virgulă în coordonate `"46,99"`) și `TestParseTab`.
  - `TestParseWindows1252`: `café` în 1252 → `Encoding == "windows-1252"`.
  - `TestHeaderSynonyms`: `LOCATIE`, `Echipament`, `adresa ip` sunt recunoscute; lipsa `host` → eroare `file` care conține „host”.
  - `TestBinaryRejected`: 2 KB aleatori cu NUL, numele `x.csv` → `apperr` 400 `field "file"`.
  - `TestCorruptXLSX`: `PK\x03\x04` + gunoi → `apperr` 400.
  - `TestXLSXRoundTrip`: un XLSX creat cu excelize (numerele ca număr pentru lat) → aceleași `LocationInput` ca CSV-ul echivalent.
  - `TestTooManyRows`: 1001 rânduri de date → eroare `file` „mai mult de 1000”; 1000 rânduri → OK.
  - `TestBuildGroupsAndPrimary`: fără coloană `is_primary`, două locații cu câte 2 echipamente → primul e primar; cu coloana și valori `x`/gol → corect; `poate` → eroare pe `is_primary`.
  - `TestBuildCellErrors`: host `999.1.1.1` → eroare pe `host` la linia corectă; lat fără lon → eroare pe `lon`; adresă diferită pe al doilea rând al aceleiași locații → eroare pe `address`; locație existentă → eroare pe `location`; host duplicat în aceeași locație → eroare pe `host` la al doilea rând; `Valid == false` și locațiile `nil`.
  - `TestBuildEmptyRowsIgnored` și `TestTooManyLocations` (201 locații).
- [ ] **Step 2: Run** `go test ./internal/importer` — Expected: FAIL (pachet inexistent).
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/importer && go vet ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(import): citire CSV/XLSX cu detectarea separatorului, codificării și zecimalei; validare pe celulă`.

### Task 2: Import — salvare atomică și API

**Files:**
- Modify: `internal/store/locations.go` (extrage `saveLocationTx` din `SaveLocation`), `internal/api/admin_auth.go` (rute)
- Create: `internal/api/admin_import.go`
- Test: `internal/store/locations_test.go`, `internal/api/import_test.go`

**Interfaces:**
- Consumes: Task 1.
- Produces:
  - `func (s *Store) SaveLocations(ctx, ins []domain.LocationInput, now time.Time) ([]SaveResult, error)` — o singură tranzacție; orice eroare anulează tot; audit `import` cu `{"locations": n, "devices": m}`.
  - `POST /api/admin/import/preview` (multipart, câmp `file`, limită 5 MB → 413 „Fișierul depășește 5 MB.”) → `Preview`.
  - `POST /api/admin/import/apply` (același fișier) → reface `Parse`+`Build` cu locațiile existente la momentul aplicării; dacă `!Valid` → 400 cu `{"error":"Fișierul conține erori; verificați previzualizarea.","preview":…}`; altfel `SaveLocations`, `Monitor.Reload`, răspuns `{"locations": n, "devices": m}`.
  - Ambele rute sunt protejate de `guard` (sesiune + CSRF + Origin).
- Consumers: Task 7.

- [ ] **Step 1: Write the failing tests:**
  - `TestSaveLocationsAtomic` (store): a doua locație are un nume care există deja → eroare, iar prima nu este salvată (numărul de locații rămâne neschimbat).
  - `TestImportPreviewAndApply` (api): CSV valid cu 2 locații → preview `valid:true`, `locations:2`; apply → 200; `/api/admin/locations` conține ambele; audit conține `import`; `Monitor.Reload` apelat.
  - `TestImportApplyRejectsInvalid`: fișier cu o eroare → 400 și nicio locație nouă.
  - `TestImportApplyRechecksExisting`: preview valid, apoi se creează separat o locație cu același nume, apoi apply → 400.
  - `TestImportLimits`: corp de 6 MB → 413; fără CSRF → 403; fără sesiune → 401; câmp `file` lipsă → 400 „Alegeți un fișier.”.
- [ ] **Step 2: Run** `go test ./internal/store ./internal/api -run 'Import|SaveLocations'` — Expected: FAIL.
- [ ] **Step 3: Implement** (`r.Body = http.MaxBytesReader(w, r.Body, 5<<20)`, `r.ParseMultipartForm(5<<20)`).
- [ ] **Step 4: Run** `go test ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(import): previzualizare și aplicare atomică a importului de locații`.

### Task 3: Exporturi CSV și XLSX (statistici și locații)

**Files:**
- Create: `internal/export/meta.go`, `internal/export/csv.go`, `internal/export/xlsx.go`
- Test: `internal/export/export_test.go`

**Interfaces:**
- Consumes: `stats.Report`, `store.Location`, Task 1 (reimport în test).
- Produces:
  - `type Meta struct { Title, AppName string; From, To time.Time; Filters []Filter; Generated time.Time; Logo []byte }`, `type Filter struct{ Label, Value string }`.
  - `func Safe(v string) string` — prefix `'` pentru `=`, `+`, `-`, `@`, `\t`, `\r` la început.
  - `func StatsCSV(w io.Writer, rep stats.Report, m Meta) error` — BOM, `;`, CRLF; blocuri separate printr-un rând gol, fiecare cu titlu: „Perioadă și filtre” (perioada `dd.mm.yyyy HH:MM – …`, „Ora: Europe/Chisinau”, filtrele), „Echipamente” (Locație; Echipament; Host; Tip; Primar; Disponibilitate (%); Acoperire (%); Funcțional (h); Degradat (h); Indisponibil (h); Fără date (h); Eroare monitor (h); Latență medie (ms); Pierderi (%); Incidente; MTTR (min)), „Locații” (Locație; Disponibilitate (%); Acoperire (%); orele pe stări), „Incidente” (Locație; Echipament; Început; Confirmat; Sfârșit; Durată (min); Motiv). Numere cu virgulă zecimală, 2 zecimale; lipsă → gol. Toate textele trec prin `Safe`.
  - `func StatsXLSX(w io.Writer, rep stats.Report, m Meta) error` — foile „Echipamente”, „Locații”, „Incidente”, „Perioadă și filtre”; numerele ca numere (format `0.00`), datele ca text `dd.mm.yyyy HH:MM`; textele prin `Safe`; antet îngroșat, rând înghețat, lățimi de coloană.
  - `func LocationsCSV(w io.Writer, locs []store.Location) error` și `func LocationsXLSX(w io.Writer, locs []store.Location) error` — antet `Locație;Adresă;Latitudine;Longitudine;Echipament;Host;Tip;Primar;Observații`, un rând pe echipament, câmpurile locației repetate, coordonatele cu `strconv.FormatFloat(v,'f',-1,64)`, `Primar` = `da`/`nu`; **fără** `Safe`; în XLSX toate celulele de tip text explicit (stil `@`, `SetCellStr`).
- Consumers: Task 4 (`Meta`), Task 4 (rute).

- [ ] **Step 1: Write the failing tests:**
  - `TestStatsCSVFormat`: începe cu BOM; prima linie conține „Perioadă și filtre”; rândul unui echipament are `99,50` pentru disponibilitate; separatorul `;`; liniile se termină în CRLF.
  - `TestFormulaInjectionStats`: echipament `=HYPERLINK("http://x")`, locație `+1`, tip `@cmd` → în CSV apar `'=HYPERLINK…`, `'+1`, `'@cmd`; în XLSX (citit înapoi cu excelize) aceleași valori cu prefix.
  - `TestStatsXLSXSheets`: foile au exact numele din specificație; „Echipamente”!F2 este numeric.
  - `TestLocationsRoundTrip`: 3 locații cu diacritice, coordonate `46.123456789`, un nume `=SUM(A1)` și echipament secundar → `LocationsCSV` → `importer.Parse`+`Build` (fără locații existente) → `LocationInput`-uri egale cu cele originale (nume, adresă, coordonate exacte, hosturi, tipuri, primar, observații); la fel pentru `LocationsXLSX`, iar celula `A2` are tipul text.
  - `TestStatsEmptyReport`: raport fără rânduri → CSV și XLSX valide, cu antete.
- [ ] **Step 2: Run** `go test ./internal/export` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/export ./internal/importer` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(export): CSV și XLSX pentru statistici și locații, cu protecție la formule`.

### Task 4: Export PDF și rutele de export

**Files:**
- Create: `internal/export/pdf.go`, `internal/export/fonts/DejaVuSans.ttf`, `internal/export/fonts/DejaVuSans-Bold.ttf`, `internal/export/fonts/LICENSE`, `internal/api/export.go`
- Modify: `internal/api/server.go`, `internal/api/admin_auth.go`
- Test: `internal/export/pdf_test.go`, `internal/api/export_test.go`

**Interfaces:**
- Consumes: Task 3 (`Meta`, `StatsCSV`, `StatsXLSX`, `LocationsCSV`, `LocationsXLSX`).
- Produces:
  - `func StatsPDF(w io.Writer, rep stats.Report, m Meta) error` — `fpdf.New("L","mm","A4","")`, fonturi prin `AddUTF8FontFromBytes` (din `//go:embed fonts`), antet cu logo (dacă `m.Logo` nu e gol), titlu, perioadă și filtre; rezumat (disponibilitate, acoperire, incidente, timp indisponibil, MTTR); tabel echipamente cu antet repetat pe fiecare pagină și text trunchiat cu „…” la lățimea coloanei; bare de stare pe locații (dreptunghiuri colorate proporțional, cu legendă); tabel incidente; subsol „Pagina X din Y · Ora: Europe/Chisinau · generat dd.mm.yyyy HH:MM” (`AliasNbPages`).
  - Fonturile DejaVu Sans 2.37 (licență Bitstream Vera/DejaVu, liberă) se descarcă o dată din arhiva oficială și se comit împreună cu `LICENSE`.
  - `GET /api/export/{format}` (`csv|xlsx|pdf`, public, aceiași parametri ca `/api/stats`, 400 pentru alt format) — `Content-Type` corect, `Content-Disposition: attachment; filename="netwatch-statistici-YYYYMMDD-HHMM.<ext>"` (ora Chișinău), `Meta` construit din identitate, filtrele afișate cu numele locației/echipamentului din catalog, logo-ul din Task 5 când există (până atunci gol).
  - `GET /api/admin/export/locations.{csv|xlsx}` (sesiune) — locațiile active, fișierul `netwatch-locatii-YYYYMMDD.<ext>`.
- Consumers: Task 7.

- [ ] **Step 1: Write the failing tests:**
  - `TestPDFLarge`: 500 de echipamente cu nume de 200 de caractere cu diacritice și 300 de incidente → fără eroare; începe cu `%PDF-`; numărul de pagini (numărul de apariții `/Type /Page` fără `s`) ≥ 10.
  - `TestPDFEmpty`: raport gol → un PDF valid, o pagină.
  - `TestPDFWithLogo`: un PNG 64×64 în `Meta.Logo` → PDF-ul conține `/Subtype /Image`.
  - `TestExportRoutes` (api): `/api/export/csv` → 200, `text/csv; charset=utf-8`, `Content-Disposition` cu `netwatch-statistici-`; `xlsx` → tipul `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`; `pdf` → `application/pdf`; `/api/export/doc` → 400; `?from=abc` → 400 JSON cu `field:"from"`.
  - `TestExportLocationsAdmin`: fără sesiune → 401; cu sesiune → CSV cu antetul `Locație;Adresă;…`.
- [ ] **Step 2: Run** `go test ./internal/export ./internal/api -run 'PDF|Export'` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(export): raport PDF A4 cu DejaVu Sans și rutele de export`.

### Task 5: Logo

**Files:**
- Create: `internal/branding/logo.go`, `internal/api/branding.go`
- Modify: `internal/api/admin_auth.go`, `internal/api/server.go`, `internal/api/export.go` (logo în `Meta`)
- Test: `internal/branding/logo_test.go`, `internal/api/branding_test.go`

**Interfaces:**
- Produces:
  - `func Prepare(data []byte) ([]byte, error)` — mărime > 500 KB → „Logo-ul poate avea cel mult 500 KB.”; `png.DecodeConfig` înainte de decodare: alt format → „Logo-ul trebuie să fie o imagine PNG.”; lățime sau înălțime > 1024 sau 0 → „Logo-ul poate avea cel mult 1024 × 1024 pixeli.”; `png.Decode` (verifică structura și CRC) → eroare „Fișierul PNG este deteriorat.”; reencodare `png.Encode` din imaginea decodată. Toate erorile `apperr.Invalid("logo", …)`.
  - `func Path(dataDir string) string` (`<data>/branding/logo.png`), `func Save(dataDir string, png []byte) error` (atomic), `func Delete(dataDir string) error`, `func Load(dataDir string) ([]byte, error)` (lipsă → `nil, nil`).
  - `POST /api/admin/identity/logo` (multipart `file`, 5 MB) → salvează, `Identity.LogoVersion = now ms`, audit `logo`, răspuns `Identity`; `DELETE /api/admin/identity/logo` → șterge, `LogoVersion = 0`, audit `logo_delete`.
  - `GET /branding/logo` (public) → `image/png`, `Cache-Control: public, max-age=86400`, 404 dacă nu există. Interfața folosește `/branding/logo?v=<logo_version>`.
- Consumers: Task 4 (logo în PDF), Task 7 (interfață).

- [ ] **Step 1: Write the failing tests:**
  - `TestPrepareRejectsBomb`: PNG cu IHDR 50000×50000 și IDAT minim (sub 1 KB) → eroarea de dimensiuni, fără alocarea imaginii (testul rulează sub 1 s).
  - `TestPrepareRejectsCorruptCRC`: un PNG valid cu un octet modificat în IDAT → „deteriorat”.
  - `TestPrepareRejectsJPEG` și `TestPrepareRejectsLarge` (600 KB).
  - `TestPrepareReencodes`: PNG valid 100×40 cu un chunk text `tEXt` → rezultatul se decodează la 100×40 și nu conține `tEXt`.
  - `TestLogoRoutes` (api): upload → 200, `logo_version > 0`, `GET /branding/logo` → 200 `image/png`; `/api/state` → `identity.logo_version` egal; `DELETE` → `/branding/logo` 404; upload fără CSRF → 403.
- [ ] **Step 2: Run** `go test ./internal/branding ./internal/api -run Logo` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(branding): logo PNG validat, reencodat și servit`.

### Task 6: Modul demonstrativ

**Files:**
- Create: `internal/demo/demo.go`
- Modify: `cmd/netwatch/main.go`, `cmd/netwatch/commands.go`, `internal/api/server.go` (`Deps.Demo`), `internal/api/public.go` (`demo` în `/api/state`), `internal/app/app.go` (`Options.Demo`)
- Test: `internal/demo/demo_test.go`, `cmd/netwatch/commands_test.go`, `internal/api/public_test.go`

**Interfaces:**
- Produces:
  - `const Password = "netwatch-demo"` (13 caractere).
  - `func Seed(ctx, st *store.Store) error` — doar dacă baza nu are locații: 12 locații reale din Moldova (Chișinău Centru, Bălți, Cahul, Orhei, Ungheni, Soroca, Comrat, Edineț, Hîncești, Strășeni, Căușeni, Ștefan Vodă) cu coordonate, câte 1–4 echipamente (router primar, switch/AP/radiolink secundare, IP-uri `10.<i>.0.<n>`), identitatea „NetWatch Demo”, parola `Password`.
  - `type Scenario struct{ Start time.Time }`; `func (s Scenario) Result(addr netip.Addr, now time.Time) icmp.Result` — determinist, în funcție de minutul scurs de la `Start`: Cahul (router) cade 6 min din fiecare 30; Orhei (radiolink secundar) cade 10 min din 20 → degradat; Soroca (router) are latență 40–120 ms și 30% pierderi; Comrat (router) răspunde intermitent la fiecare a treia verificare (exersează confirmarea); Edineț (AP secundar) „Eroare monitor” (`Permission`) 3 min din 60; restul OK cu latență 2–15 ms (variație sinus determinist).
  - `cmd netwatch demo [--data demo-data] [--listen :8502]` — `Seed`, apoi `app.Run` cu `icmp.Fake{Fn: scenario.Result}`, rezolvare doar pentru IP-uri, `IntervalScale: 1.0/6` (intervalele de 60 s devin 10 s), `Options.Demo = true`; afișează în consolă adresa și parola.
  - `/api/state` → câmpul `"demo": true|false`.
- Consumers: Task 7 (banner).

- [ ] **Step 1: Write the failing tests:**
  - `TestScenarioDeterministic`: același `(addr, now)` → același rezultat; Cahul la minutul 2 → eșec, la minutul 10 → OK; Edineț AP la minutul 1 → `Permission`.
  - `TestSeedOnce`: `Seed` de două ori → 12 locații; parola verifică `Password`; identitatea are numele „NetWatch Demo”.
  - `TestStateDemoFlag` (api): `Deps.Demo = true` → `"demo":true`.
  - `TestCmdDemoStarts` (cmd): `run([]string{"demo","--data",tmp,"--listen","127.0.0.1:0"}, …)` cu variabila de pachet `demoReady func(addr string)` (cârlig de test, implicit nil, transmis ca `Options.Ready`) și o variabilă `demoStop` (context anulabil) ca testul să oprească serverul după ce citește `/api/state` → 12 locații în răspuns și `demo:true`.
- [ ] **Step 2: Run** `go test ./internal/demo ./internal/api ./cmd/netwatch -run 'Demo|Scenario|Seed'` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(demo): netwatch demo cu scenarii simulate în demo-data`.

### Task 7: Interfață — import, exporturi, logo, banner demo

**Files:**
- Create: `web/src/pages/admin/Import.svelte`, `web/src/components/DemoBanner.svelte`, `web/e2e/import-export.spec.ts`, `web/e2e/fixtures/` (fișiere CSV de test generate în test)
- Modify: `web/src/pages/admin/AdminShell.svelte` (meniu „Import / export”, ruta `/administrare/import`), `web/src/lib/router.svelte.ts` (`ADMIN_PAGES` + `import`), `web/src/lib/api.ts` (`upload<T>(path, file)` multipart cu CSRF), `web/src/pages/Stats.svelte`, `web/src/pages/admin/Identity.svelte`, `web/src/components/Header.svelte`, `web/src/pages/Report24.svelte`, `web/src/pages/Dispatch.svelte`, `web/src/App.svelte`, `web/src/lib/types.ts` (`demo`, `Preview`), `web/src/lib/live.svelte.ts` (`demo`), `web/src/lib/format.ts` (acțiunile de audit `import`, `logo`, `logo_delete`)
- Test: `web/e2e/import-export.spec.ts`, `web/e2e/logo.spec.ts`, `web/src/lib/api.test.ts`

**Interfaces:**
- Consumes: Task 2, 4, 5, 6.
- Produces:
  - `upload<T>(path: string, file: File): Promise<T>` — `FormData`, antet `X-CSRF-Token`, aceleași erori ca `send`.
  - Import (`/administrare/import`): alegere fișier (`.csv,.xlsx`), „Previzualizează” → tabel cu coloanele canonice, rândurile cu erori marcate (`aria-invalid` pe celulă, mesajul sub valoare), rezumat („N locații, M echipamente, E erori; codificare windows-1250, separator ;”), „Importă” activ doar când `valid`; după succes, mesaj „S-au importat N locații.” și legătura spre listă. Tot pe pagină: „Exportă locațiile (CSV)” / „(Excel)” → legăturile `/api/admin/export/locations.csv|xlsx`.
  - Statistici: butoanele „CSV”, „Excel”, „PDF” — legături `/api/export/{fmt}?<aceiași parametri ca cererea afișată>` (`download`).
  - Identitate: secțiunea „Logo” — previzualizare `/branding/logo?v=…`, încărcare PNG (eroarea serverului afișată), „Elimină logo-ul” cu confirmare; după schimbare `live.identity` se actualizează.
  - Logo afișat în antet (înălțime 32 px, `alt` = numele aplicației), în Raport 24H și în dispecerat, doar când `logo_version > 0`.
  - `DemoBanner`: bandă sus pe toate paginile, inclusiv dispeceratul, când `live.demo`: „MOD DEMONSTRATIV — date simulate. Parola de administrare: netwatch-demo”.
- [ ] **Step 1: Write the failing tests:**
  - `api.test.ts` › `upload`: trimite `FormData` cu câmpul `file`, antetul CSRF, fără `Content-Type` setat manual.
  - `import-export.spec.ts`:
    - `previzualizare cu erori`: încarcă un CSV (generat în test, `;`, Windows-1250) cu o locație bună și un rând cu host `999.1.1.1` → tabelul arată eroarea pe celula host, „Importă” dezactivat.
    - `import reușit`: CSV valid cu 2 locații noi → „Importă” → „S-au importat 2 locații.”; locațiile apar pe `/` (curățenie: arhivare prin API).
    - `export locații reimportabil`: descarcă `locations.csv` (`page.waitForEvent('download')`) → conține antetul și „Cahul”; încărcat la previzualizare → toate locațiile apar ca „există deja”.
    - `exporturi statistici`: pe `/statistici?preset=7z`, „CSV” descarcă un fișier `netwatch-statistici-*.csv` care începe cu BOM; „PDF” descarcă un fișier care începe cu `%PDF`; URL-ul legăturii conține `from=` și `to=`.
  - `logo.spec.ts`: încarcă un PNG 64×64 (generat în test) → antetul afișează `img` cu `src` `/branding/logo?v=`; un fișier text `.png` → mesajul „Logo-ul trebuie să fie o imagine PNG.”; „Elimină logo-ul” → imaginea dispare.
  - `smoke.spec.ts` › `banner demo`: pe e2eserver pornit cu `--demo-flag` (flag nou în `tools/e2eserver` care setează `Options.Demo`), banner-ul „MOD DEMONSTRATIV” este vizibil — folosește al doilea server (`--no-password --demo-flag` pe 18511).
- [ ] **Step 2: Run** `npm test && npm run e2e -- import-export logo smoke` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm test && npm run check && npm run e2e` (toate) — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): import/export, logo și bannerul modului demonstrativ`.

### Task 8: Build de release

**Files:**
- Create: `tools/release/main.go`
- Test: `tools/release/main_test.go`

**Interfaces:**
- Produces: `go run ./tools/release [--version V] [--out dist] [--targets windows/amd64,linux/amd64,linux/arm64] [--skip-web]`:
  - fără `--skip-web`: `npm ci` și `npm run build` în `web/` (pe Windows prin `npm.cmd`); apoi verifică existența `internal/webui/dist/index.html` (altfel eroare „Interfața web nu este compilată; rulați fără --skip-web.”);
  - versiunea implicită: `git describe --tags --always --dirty`, cu prefixul `v` eliminat;
  - pentru fiecare țintă: `go build -trimpath -ldflags "-s -w -X netwatch/internal/version.Version=<V>" -o <out>/<nume> ./cmd/netwatch` cu `CGO_ENABLED=0`, `GOOS`, `GOARCH`;
  - `func artifactName(goos, goarch string) string` → `netwatch-windows-amd64.exe`, `netwatch-linux-amd64`, `netwatch-linux-arm64`;
  - `func writeChecksums(dir string, names []string) error` → `<out>/SHA256SUMS`, linii `<hex>  <nume>`, sortate după nume.
- [ ] **Step 1: Write the failing tests:** `TestArtifactName`; `TestWriteChecksums` (două fișiere cunoscute → conținutul exact); `TestReleaseBuildHost` (sărit cu `-short` sau dacă `internal/webui/dist/index.html` lipsește): `--skip-web --targets <GOOS>/<GOARCH> --version 9.9.9 --out <tmp>` → binarul există, rulat cu `version` afișează `9.9.9`, iar `SHA256SUMS` conține suma lui.
- [ ] **Step 2: Run** `go test ./tools/release` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./tools/release` și `go run ./tools/release --skip-web --version 1.0.0-test --out <tmp>` (toate cele trei ținte) — Expected: PASS; trei binare și `SHA256SUMS`.
- [ ] **Step 5: Commit** `build: go run ./tools/release pentru Windows și Linux, cu sume de control`.

### Task 9: CI pe GitHub Actions

**Files:**
- Create: `.github/workflows/ci.yml`
- Modify: `internal/monitor/icmp/real_test.go` (dacă e nevoie: testul ICMP real se rulează și pe Linux, în ambele moduri)

**Interfaces:**
- Produces: workflow cu declanșare la `push` și `pull_request`, plus la etichete `v*`:
  - `go` (matrice `ubuntu-latest`, `windows-latest`): `actions/setup-go` cu `go-version-file: go.mod`; `go vet ./...`; `go test -race ./...` (Linux: înainte `sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"` pentru modul neprivilegiat); pe Linux încă o rulare `sudo -E env "PATH=$PATH" go test -run Real -v ./internal/monitor/icmp` pentru modul raw;
  - `web` (`ubuntu-latest`): Node 24, `npm ci`, `npm run check`, `npm test`, `npx playwright install --with-deps chromium`, `npm run e2e`; la eșec încarcă `web/test-results` ca artefact;
  - `release` (doar pe etichete `v*`, după `go` și `web`): `go run ./tools/release --version ${GITHUB_REF_NAME#v}`, încarcă `dist/*` ca artefact și creează un GitHub Release cu ele (`softprops/action-gh-release` cu versiune fixată).
  - Toate acțiunile terțe fixate la o versiune majoră; `permissions: contents: read` implicit, `contents: write` doar pentru `release`.
- [ ] **Step 1: Write the workflow** și verifică-l local: `go run github.com/rhysd/actionlint/cmd/actionlint@<ultima versiune>` — Expected: înainte de corecturi pot apărea avertismente; după, fără erori.
- [ ] **Step 2: Verifică testul ICMP real:** confirmă (citind `real_test.go`) că pe Linux testează `127.0.0.1` și `::1` cu succes și o adresă TEST-NET cu eșec; dacă nu, adaugă cazurile (rulate local pe Windows).
- [ ] **Step 3: Run** actionlint și `go test ./internal/monitor/icmp` — Expected: fără erori; PASS.
- [ ] **Step 4: Commit** `ci: GitHub Actions cu -race, ICMP pe Linux, Playwright și release`.

### Task 10: Probleme minore amânate din Planul 2 — backend și igienă

**Files:**
- Modify: `internal/store/locations.go` (`SetLocationOrder`), `internal/tiles/tiles.go`, `.gitignore`, `web/e2e/fixtures.ts`, `web/src/components/Announcer.svelte`
- Delete: `web/src/node_modules/.vite/vitest/*` (din index)
- Test: `internal/store/locations_test.go`, `internal/tiles/tiles_test.go`, `web/e2e/smoke.spec.ts`

**Interfaces:** comportamente noi, fără API nou.
- `SetLocationOrder` cere exact mulțimea locațiilor active (lipsă sau în plus → `apperr.Invalid("ids", "Lista nu corespunde locațiilor active; reîncărcați pagina.")`).
- `tiles.New` șterge fișierele `.tile-*` rămase.
- `.gitignore`: `node_modules/` oriunde; fișierele de cache Vitest scoase din depozit.
- Fixture-ul E2E: în loc să ignore toate mesajele „Failed to load resource”, înregistrează răspunsurile ≥ 400 și eșuează testul pentru cele din afara `/api/` și `/tiles/` (resursele interfeței trebuie să existe mereu); mesajele de consolă „Failed to load resource” rămân ignorate doar pentru acele două prefixe.
- Announcer: cheie numerică crescătoare pentru fiecare mesaj.
- [ ] **Step 1: Write the failing tests:** `TestSetLocationOrderRequiresFullSet` (store: lista fără o locație activă → eroare `ids`); `TestTileCleansTemps` (un `.tile-123` în director → dispare după `New`); `smoke.spec.ts` › `fixture detectează un fișier lipsă al interfeței` (`test.fail()`; `page.route` pe `**/assets/*.css` → 404).
- [ ] **Step 2: Run** testele — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./...` și `npm run e2e` — Expected: PASS.
- [ ] **Step 5: Commit** `fix: ordinea completă a locațiilor, curățenie în cache-ul de dale, fixture E2E mai strict`.

### Task 11: Probleme minore amânate din Planul 2 — interfață

**Files:**
- Modify: `web/src/pages/LocationPanel.svelte`, `web/src/pages/admin/AdminShell.svelte`, `web/src/App.svelte`, `web/src/lib/theme.ts`, `web/src/pages/Dispatch.svelte`, `web/src/pages/admin/Locations.svelte`
- Test: `web/e2e/panel.spec.ts`, `web/e2e/admin-locations.spec.ts`, `web/e2e/dispatch.spec.ts`, `web/src/lib/theme.test.ts`

**Interfaces:**
- Panoul: secvență de cereri (doar ultimul `load()` se aplică); focusul rămâne în panou (Tab și Shift+Tab ciclează între elementele focalizabile ale panoului).
- Sesiune expirată în administrare: formularul de login apare **peste** pagina curentă (overlay `role="dialog"`), pagina rămâne montată; după autentificare, modificările nesalvate sunt intacte.
- Tema: dacă utilizatorul nu a ales explicit, tema urmează sistemul în timp real (`matchMedia(...).addEventListener('change')`), iar dispeceratul e întunecat și la navigarea internă (`effectiveTheme(stored, prefersDark, route)`); `?tema=` are prioritate.
- Dispecerat: banda de probleme afișează cel mult câte încap și un indicator „+N” pentru restul (calculat după măsurarea lățimii); plăcile în starea neactualizată afișează „· neactualizat”.
- Lista de locații din administrare: clicurile ↑/↓ succesive se serializează (o singură cerere în curs; în caz de eroare lista se reîncarcă de pe server).
- [ ] **Step 1: Write the failing tests:**
  - `theme.test.ts`: `effectiveTheme(null, false, 'dispatch')` → `dark`; `effectiveTheme(null, true, 'monitor')` → `dark`; `effectiveTheme('light', true, 'dispatch')` → `light`.
  - `panel.spec.ts` › `perioade în succesiune rapidă`: întârzie răspunsurile pentru 30 z cu `page.route` (1,5 s), click 30 z apoi imediat 24 h → după 2 s, ultima cerere de cronologie aplicată are intervalul de 24 h (verificat prin axa „de la” a cronologiei, cu dată din ultimele 24 h). › `focusul rămâne în panou`: 30 de apăsări Tab → elementul activ este mereu în dialog.
  - `admin-locations.spec.ts` › `sesiune expirată în editor păstrează modificările`: modifică numele, șterge cookie-urile, „Salvează” → dialogul de autentificare; autentificare → câmpul păstrează valoarea; „Salvează” → „Locația a fost salvată.”; (revine la numele original la final).
  - `dispatch.spec.ts` › `banda arată +N`: 12 locații căzute (create prin API, cu host-uri setate `timeout`) la 1280×720 → textul `+` urmat de un număr este vizibil; › `temă întunecată la navigarea internă`: `colorScheme: 'light'`, `/` → click „Dispecerat” → `data-theme="dark"`.
- [ ] **Step 2: Run** `npm test && npm run e2e -- panel admin-locations dispatch` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm test && npm run check && npm run e2e` — Expected: PASS.
- [ ] **Step 5: Commit** `fix(web): panou, sesiune expirată fără pierderi, temă, banda dispeceratului`.

### Task 12: Documentație și verificare finală

**Files:**
- Modify: `README.md`, `docs/operare.md`

**Interfaces:**
- Documentație: importul (format, sinonime, exemple CSV din Excel), exporturile, logo-ul, `netwatch demo` (adresă, parolă, directorul `demo-data`), build-ul de release (`go run ./tools/release`), CI și publicarea unei versiuni (etichetă `vX.Y.Z`).
- [ ] **Step 1: Update docs.**
- [ ] **Step 2: Full verification:** `go vet ./...`, `GOOS=linux go vet ./...`, `go test ./...`, actionlint, `npm run check`, `npm test`, `npm run e2e`, `go run ./tools/release --version 1.0.0-rc1 --out <tmp>` (cu interfața), apoi pornește `netwatch-windows-amd64.exe demo --data <tmp>\demo-data --listen 127.0.0.1:18701` și verifică prin `curl`: `/api/state` → 12 locații și `demo:true`; `/api/export/pdf` → `%PDF`; `/` → HTML-ul aplicației. Expected: totul trece.
- [ ] **Step 3: Commit** `docs: import/export, logo, mod demo, release și CI`.
