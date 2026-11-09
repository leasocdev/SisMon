# NetWatch 1.0 — Plan 1: Nucleul backend — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un executabil Go `netwatch` care monitorizează prin ICMP nativ, păstrează istoricul ca intervale de stare, calculează statistici, expune API-ul public și de administrare (cu SSE), rulează joburile de fundal și se instalează ca serviciu pe Windows/Linux.

**Architecture:** Un singur proces: scheduler + pinger nativ → canal → un singur fir de scriere în SQLite (WAL) → hub `live` în memorie → API HTTP/SSE. Regulile de stare (`internal/state`) și calculul statisticilor (`internal/stats`) sunt pure și testate independent. Joburile (backup, agregare, retenție) rulează pe fire proprii.

**Tech Stack:** Go (≥1.25, `CGO_ENABLED=0`), `modernc.org/sqlite`, `golang.org/x/net/icmp`, `golang.org/x/sys`, `golang.org/x/crypto/argon2`, `github.com/kardianos/service`, `github.com/BurntSushi/toml`, `gopkg.in/natefinch/lumberjack.v2`, `log/slog`, `net/http` (rutare `ServeMux` cu metode și wildcard).

**Spec:** `docs/superpowers/specs/2026-11-09-netwatch-design.md` — secțiunile 2–7, 10, 11. Planurile ulterioare: Plan 2 (interfața web Svelte, §8), Plan 3 (import/export, dale OSM, logo, demo, release/CI, §6 tiles, §9).

## Global Constraints

- Modul Go: `netwatch`; executabil: `cmd/netwatch`; `CGO_ENABLED=0` trebuie să compileze pentru `windows/amd64`, `linux/amd64`, `linux/arm64`.
- Toate momentele în bază: milisecunde Unix UTC (`INTEGER`). În JSON: timestamp = întreg ms Unix; durate = secunde (`float64`). Chei JSON în `snake_case`.
- Stări: `up`, `down`, `degraded`, `unknown`, `error`. Motive de închidere: `recovered`, `monitoring_gap`, `monitor_error`, `configuration`, `archived`.
- Setări implicite/limite: `interval_s` 10–3600 (60), `packets` 1–10 (3), `timeout_s` 1–10 (2), `failures` 1–20 (3), `recoveries` 1–20 (2), `stale_s` 30–86400 (300).
- Reguli: `packets × timeout + (packets−1) × 0,2 s + 2 s < interval`; `stale ≥ interval × (max(failures, recoveries) + 1)`.
- Pool de verificări: 64; pauză între pachete: 200 ms; timeout DNS: 2 s; flush scriere: 1 s; supraveghere pauze: 30 s.
- Retenție `samples`: 90 de zile; `samples_hourly`: nelimitat. Backup: zilnic la `backup_hour` (implicit 3), rotație 7 zilnice / 4 săptămânale / 12 lunare, reîncercare după 1 h.
- Sesiuni: token 32 de octeți, în bază doar SHA-256; expirare 8 h inactivitate / 24 h absolut; cookie `nw_session` `HttpOnly`, `SameSite=Strict`, `Secure` pe HTTPS.
- Argon2id m=64 MiB, t=3, p=2; parolă 12–200 caractere; rate-limit 5 eșecuri / 5 min / IP.
- Limite cereri: 1 MB JSON; perioadă statistici maximum 400 de zile.
- Toate mesajele de eroare pentru utilizator în română, format `{"error":"…","field":"…"}`; nicio intrare invalidă nu produce 500.
- Fus orar de afișare: `Europe/Chisinau` (`time/tzdata` importat în `cmd/netwatch`).
- Port implicit `:8501`; date implicite Windows `C:\ProgramData\NetWatch`, Linux `/var/lib/netwatch`.

## Review Focus

1. **Ceasul sistemului sare înapoi** (corecție NTP) — un interval nu trebuie să aibă `end_at < start_at`; închiderea se limitează la `start_at` al intervalului deschis. Test: Task 3 `TestCloseAndOpenClampsBackwardsTime`.
2. **Host modificat în timp ce o verificare e în zbor** — rezultatul vechi este ignorat (comparare `rev`). Test: Task 8 `TestWriterDropsStaleRevision`.
3. **Repornire după oprire lungă** — la pornire, intervalul curent se închide la `ultima_verificare + stale`, incidentul se închide cu `monitoring_gap`. Test: Task 8 `TestWriterExpiresGapAtStartup`.
4. **Perioadă fără timp eligibil** (echipament creat după `to`, sau `from==to` respins) — disponibilitate `null`, fără împărțire la zero. Test: Task 10 `TestReportDeviceCreatedAfterPeriod`.
5. **SSE prin reverse proxy cu buffering** — răspunsul trebuie să conțină `X-Accel-Buffering: no` și `Cache-Control: no-cache`, cu flush după fiecare eveniment. Test: Task 13 `TestEventsHeadersAndFlush`.

---

### Task 1: Toolchain, schelet de proiect, versiune

**Files:**
- Create: `go.mod`, `.gitignore`, `README.md`, `cmd/netwatch/main.go`, `internal/version/version.go`, `cmd/netwatch/main_test.go`

**Interfaces:**
- Produces: `version.Version string` (setabil prin `-ldflags "-X netwatch/internal/version.Version=…"`, implicit `"1.0.0-dev"`); subcomanda `netwatch version` afișează `NetWatch 1.0.0-dev`.

- [ ] **Step 1: Instalează Go** — cere confirmarea utilizatorului, apoi `winget install --id GoLang.Go -e`; redeschide shell-ul. Verificare: `go version` afișează ≥ `go1.25`.
- [ ] **Step 2: Inițializează depozitul** — în `D:\Soft\netwatch-go`: `git init`, `go mod init netwatch`, `.gitignore` cu `/dist/`, `/data/`, `/demo-data/`, `*.db`, `*.db-wal`, `*.db-shm`, `/web/node_modules/`, `/internal/webui/dist/*` (cu excepția `.gitkeep`).
- [ ] **Step 3: Scrie testul eșuat** — `TestVersionCommand`: rulează `run([]string{"version"}, &out)` și verifică `out.String() == "NetWatch 1.0.0-dev\n"`.
- [ ] **Step 4: Rulează** `go test ./cmd/netwatch/` → FAIL (`run` nedefinit).
- [ ] **Step 5: Implementează** `func run(args []string, stdout io.Writer) int` în `cmd/netwatch/main.go` (dispatcher pe primul argument; necunoscut → mesaj de utilizare în română, cod 2); `main()` apelează `os.Exit(run(os.Args[1:], os.Stdout))`; `import _ "time/tzdata"`.
- [ ] **Step 6: Rulează** `go test ./...` → PASS; `go build ./cmd/netwatch` și `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/netwatch` reușesc.
- [ ] **Step 7: Commit** — `git add -A && git commit -m "chore: schelet proiect NetWatch 1.0"` (include spec-ul și planul).

---

### Task 2: Reguli de stare pure (`internal/state`)

**Files:**
- Create: `internal/state/state.go`, `internal/state/device.go`, `internal/state/location.go`
- Test: `internal/state/device_test.go`, `internal/state/location_test.go`

**Interfaces:**
- Produces:
  ```go
  type State string
  const (Up State = "up"; Down = "down"; Degraded = "degraded"; Unknown = "unknown"; Error = "error")
  type Thresholds struct{ Failures, Recoveries int }
  type Check struct{ At time.Time; OK bool; MonitorError bool }
  type Device struct {
      State       State
      Pending     State     // "" sau Up/Down: direcția seriei în curs, dacă diferă de State
      Streak      int
      StreakStart time.Time
  }
  type Transition struct{ From, To State; At time.Time; ConfirmedAt time.Time }
  func (d *Device) Apply(c Check, t Thresholds) *Transition // nil dacă starea nu se schimbă
  func (d *Device) Expire()                                  // State=Unknown, serie resetată
  type Member struct{ Primary bool; State State }
  type LocationResult struct{ State State; Incomplete bool }
  func Location(members []Member) LocationResult
  ```

Semantica `Apply` (spec §4): `MonitorError` → State `Error`, serie resetată, tranziție la `c.At` dacă State ≠ Error. Altfel, dacă direcția (OK/eșec) diferă de seria curentă, seria repornește cu `StreakStart = c.At`; `Streak++`. OK cu `Streak ≥ Recoveries` și State ≠ Up → tranziție la Up cu `At = StreakStart`. Eșec cu `Streak ≥ Failures` și State ≠ Down → tranziție la Down cu `At = StreakStart`, `ConfirmedAt = c.At`. `Pending` = direcția seriei dacă diferă de State, altfel `""`.

- [ ] **Step 1: Scrie testele eșuate** (table-driven):
  - `TestHysteresisDownAndRecovery`: Thresholds{3,2}; verificări la t0..t6 = OK,OK,F,F,F,OK,OK → stări după fiecare: unknown,up,up,up,down,down,up; tranziția Down are `At==t2`, `ConfirmedAt==t4`; tranziția Up are `At==t5`.
  - `TestPendingDirection`: după OK,OK,F → `Pending==Down`; după încă OK → `Pending==""`.
  - `TestMonitorErrorResetsStreak`: OK,OK, MonitorError → State Error, Streak 0; apoi F,F,F → Down cu `At` = prima F de după eroare.
  - `TestExpire`: din Down, `Expire()` → State Unknown, Streak 0, Pending "".
  - `TestLocationRules` cu cazurile: [P up,S up]→up; [P up,S down]→degraded; [P down,S up]→down; [P down,P unknown]→down; [P unknown,S up]→unknown; [P up,S unknown]→unknown; [P up,S down,S unknown]→degraded+Incomplete; [P error,S up]→unknown; []→unknown; [S up] (fără primar)→unknown.
- [ ] **Step 2: Rulează** `go test ./internal/state/` → FAIL.
- [ ] **Step 3: Implementează** tipurile și funcțiile de mai sus.
- [ ] **Step 4: Rulează** `go test ./internal/state/` → PASS.
- [ ] **Step 5: Commit** — `feat(state): histerezis, atribuire retroactivă, starea locației`.

---

### Task 3: Stocare SQLite, schemă, migrații, intervale (`internal/store`)

**Files:**
- Create: `internal/store/store.go`, `internal/store/migrations.go`, `internal/store/schema_v1.sql` (embed), `internal/store/intervals.go`, `internal/store/meta.go`
- Test: `internal/store/store_test.go`, `internal/store/intervals_test.go`

**Interfaces:**
- Consumes: `state.State`.
- Produces:
  ```go
  const SchemaVersion = 1
  type Options struct{ BeforeMigrate func(s *Store, from, to int) error }
  type Store struct{ DB *sql.DB; Path string }
  func Open(ctx context.Context, path string, opt Options) (*Store, error)
  func (s *Store) Close() error
  func (s *Store) VacuumInto(ctx context.Context, dest string) error
  func (s *Store) GetMeta(ctx context.Context, key string, v any) (bool, error) // JSON
  func (s *Store) SetMeta(ctx context.Context, key string, v any) error
  var ErrSchemaTooNew, ErrNotFound error
  type Entity string // "device" | "location"
  func Ms(t time.Time) int64; func FromMs(ms int64) time.Time
  func CloseAndOpen(ctx context.Context, tx *sql.Tx, e Entity, id int64, st state.State, at time.Time) error
  func CloseOpen(ctx context.Context, tx *sql.Tx, e Entity, id int64, at time.Time) error
  func OpenState(ctx context.Context, q Querier, e Entity, id int64) (st state.State, start time.Time, ok bool, err error)
  type Querier interface{ QueryRowContext(...); QueryContext(...); ExecContext(...) } // *sql.DB și *sql.Tx
  ```

Schema v1 = tabelele din spec §3, plus: `devices.rev INTEGER NOT NULL DEFAULT 1` (incrementat la schimbarea host-ului/arhivare) și `device_status(device_id PK, state, pending, streak, streak_start, last_check_at, latency_ms, loss_pct, error_code, dns_warning)` pentru reluarea după repornire. `meta.schema_version`. Pragma la fiecare conexiune: `journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=ON`, `synchronous=NORMAL` (prin DSN `_pragma=`).

`CloseAndOpen`: dacă intervalul deschis are aceeași stare → nimic. Altfel închide la `max(at, open.start)` și deschide noul interval la aceeași valoare. Dacă nu există interval deschis → doar inserează.

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestOpenCreatesSchema`: deschide în `t.TempDir()`; `schema_version` = 1; tabelele `locations, devices, device_status, state_intervals, samples, samples_hourly, incidents, audit, sessions, meta` există; `PRAGMA journal_mode` = `wal`.
  - `TestOpenRejectsNewerSchema`: setează `schema_version` = 99, redeschide → `errors.Is(err, ErrSchemaTooNew)`.
  - `TestBeforeMigrateCalled`: bază cu versiunea 0 simulată → callback primit cu (0,1).
  - `TestMetaRoundTrip`: SetMeta/GetMeta cu struct.
  - `TestCloseAndOpen`: up la t1, down la t3 → două rânduri: [t1,t3] up, [t3,NULL] down; același state din nou → tot două rânduri.
  - `TestCloseAndOpenClampsBackwardsTime` (Review Focus 1): up la t5, down la t2 → rândul up are `end_at == start_at == t5`, down începe la t5.
  - `TestVacuumInto`: fișierul rezultat trece `PRAGMA integrity_check` = `ok`.
- [ ] **Step 2: Rulează** `go test ./internal/store/` → FAIL.
- [ ] **Step 3: Implementează** conform interfețelor (`database/sql` cu driverul `sqlite` din `modernc.org/sqlite`; migrațiile ca listă ordonată de fișiere SQL încorporate, aplicate în tranzacție).
- [ ] **Step 4: Rulează** → PASS.
- [ ] **Step 5: Commit** — `feat(store): schemă v1, migrații, intervale de stare`.

---

### Task 4: Erori aplicație și setări (`internal/apperr`, `internal/settings`)

**Files:**
- Create: `internal/apperr/apperr.go`, `internal/settings/settings.go`, `internal/settings/holder.go`
- Test: `internal/settings/settings_test.go`

**Interfaces:**
- Produces:
  ```go
  // apperr
  type Error struct{ Status int; Field, Message string }
  func (e *Error) Error() string
  func Invalid(field, msg string) *Error   // 400
  func NotFound(msg string) *Error        // 404
  func Conflict(field, msg string) *Error // 409
  // settings
  type Settings struct {
      IntervalS int `json:"interval_s"`; Packets int `json:"packets"`; TimeoutS int `json:"timeout_s"`
      Failures int `json:"failures"`; Recoveries int `json:"recoveries"`; StaleS int `json:"stale_s"`
      IncidentsFor string `json:"incidents_for"` // "all" | "primary"
      PreferIPv4 bool `json:"prefer_ipv4"`; BackupHour int `json:"backup_hour"`; BackupSecondaryDir string `json:"backup_secondary_dir"`
  }
  type Overrides struct{ IntervalS, Failures, Recoveries *int } // json: interval_s, failures, recoveries
  type Effective struct{ Interval, Timeout time.Duration; Packets, Failures, Recoveries int; Stale time.Duration }
  func Defaults() Settings // 60,3,2,3,2,300,"all",true,3,""
  func (s Settings) Validate() error          // *apperr.Error cu Field
  func (s Settings) Apply(o Overrides) (Effective, error) // validează și combinația
  type Holder struct{ ... }                   // atomic
  func Load(ctx context.Context, st *store.Store) (*Holder, error) // meta "settings", altfel Defaults
  func (h *Holder) Get() Settings
  func (h *Holder) Save(ctx context.Context, st *store.Store, s Settings) error // Validate apoi SetMeta
  ```

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestDefaultsValid`: `Defaults().Validate() == nil`.
  - `TestLimits` table: fiecare câmp sub/peste limită → `*apperr.Error` cu `Field` egal cu cheia JSON.
  - `TestCheckFitsInterval`: `{IntervalS:10, Packets:3, TimeoutS:3}` → eroare pe `interval_s` (3×3+0,4+2=11,4 ≥ 10); `{IntervalS:12,…}` → ok.
  - `TestStaleRule`: interval 60, failures 3, recoveries 2, stale 239 → eroare pe `stale_s`; 240 → ok.
  - `TestOverridesValidated`: `Overrides{IntervalS:ptr(5)}` → eroare `interval_s`; `Overrides{Failures:ptr(10)}` cu stale 300 → eroare `stale_s` (60×11 > 300).
  - `TestHolderPersists`: Save apoi Load nou → aceleași valori.
  - `TestIncidentsForValues`: `"x"` → eroare.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** Mesajele în română indică valoarea minimă acceptată (ex.: „Intervalul trebuie să fie de cel puțin 12 secunde pentru 3 pachete cu timeout de 3 s.”). **Step 4: Rulează** → PASS.
- [ ] **Step 5: Commit** — `feat(settings): setări globale, suprascrieri, validare`.

---

### Task 5: Locații și echipamente — validare și persistare

**Files:**
- Create: `internal/domain/location.go`, `internal/domain/host.go`, `internal/store/locations.go`, `internal/store/recompute.go`
- Test: `internal/domain/location_test.go`, `internal/store/locations_test.go`

**Interfaces:**
- Consumes: `apperr`, `settings.Settings.Apply`, `store.CloseAndOpen/CloseOpen`, `state.Location`.
- Produces:
  ```go
  // domain
  type DeviceInput struct{ ID *int64 `json:"id"`; Name, Host, Kind string; Primary bool `json:"is_primary"`; Overrides settings.Overrides `json:"overrides"` }
  type LocationInput struct{ Name, Address, Notes string; Lat, Lon *float64; Devices []DeviceInput }
  func (in *LocationInput) Normalize(s settings.Settings) error
  func ValidHost(h string) bool
  // store
  type Device struct{ ID, LocationID, Rev int64; Name, Host, Kind string; Primary bool; SortOrder int; Overrides settings.Overrides; CreatedAt time.Time; ArchivedAt *time.Time }
  type Location struct{ ID int64; Name, Address, Notes string; Lat, Lon *float64; SortOrder int; CreatedAt time.Time; ArchivedAt *time.Time; Devices []Device }
  type SaveResult struct{ LocationID int64; Added, HostChanged, Archived []int64 }
  func (s *Store) ListLocations(ctx context.Context, includeArchived bool) ([]Location, error) // ordine sort_order, name
  func (s *Store) GetLocation(ctx context.Context, id int64) (Location, error)
  func (s *Store) SaveLocation(ctx context.Context, id *int64, in domain.LocationInput, now time.Time) (SaveResult, error)
  func (s *Store) ArchiveLocation(ctx context.Context, id int64, now time.Time) (SaveResult, error)
  func (s *Store) Audit(ctx context.Context, q Querier, action, entity string, id int64, details any, now time.Time) error
  func RecomputeLocation(ctx context.Context, tx *sql.Tx, locationID int64, at time.Time) (state.LocationResult, error)
  ```

`Normalize`: trim; nume 1–120; adresă ≤ 500; observații ≤ 4000; tip ≤ 50 (gol → „Altele”); lat/lon ambele sau niciuna, în domeniu și finite; 1–50 echipamente; cel puțin un primar; host-uri unice (case-insensitive) în locație; `ValidHost` = IP valid sau nume DNS (etichete 1–63, `[A-Za-z0-9-]`, fără `-` la capete, total ≤ 253, fără spații); suprascrierile validate prin `s.Apply`. Câmpurile echipamentelor raportează `Field` ca `devices.<index>.<câmp>`.

`SaveLocation` (o tranzacție): nume unic între locațiile nearhivate (altfel `Conflict("name", …)`); ID-ul de echipament trebuie să aparțină locației și să fie nearhivat; echipament nou → interval `unknown` la `now` + `device_status` unknown; host schimbat → `rev++`, `device_status` resetat, incident deschis închis cu `configuration`, `CloseAndOpen(unknown, now)`; echipament eliminat → `archived_at`, `rev++`, incident închis cu `archived`, `CloseOpen(now)`; apoi `RecomputeLocation(now)`; audit `save_location` cu configurația. `ArchiveLocation`: arhivează toate echipamentele (ca mai sus) și locația, închide intervalul locației, audit `archive_location`.

`RecomputeLocation`: citește `device_status` pentru echipamentele active, calculează `state.Location`, apelează `CloseAndOpen(location, …)`.

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestValidHost` table: `127.0.0.1`, `2001:db8::1`, `router.local` → true; `-c`, `x;whoami`, `a b`, `a..b`, `$(id)`, `""`, 254 caractere → false.
  - `TestNormalizeErrors` table: fără primar → field `devices`; host duplicat `A`/`a` → `devices.1.host`; doar lat → `lon`; 51 de echipamente → `devices`.
  - `TestSaveNewLocation`: creează; `ListLocations` returnează 1 locație cu 2 echipamente; există interval `unknown` deschis pentru fiecare echipament și pentru locație.
  - `TestSaveDuplicateNameConflict`: al doilea „centru” (case-insensitive) → `apperr.Error.Status == 409`.
  - `TestSaveForeignDeviceIDRejected`: ID de echipament din altă locație → 400, nimic modificat (tranzacție anulată).
  - `TestHostChangeResets`: rev crește cu 1; incidentul deschis are `end_reason='configuration'`; intervalul curent e `unknown`.
  - `TestRemovedDeviceArchived`: echipamentul lipsă din input are `archived_at`; intervalul lui e închis.
  - `TestArchiveLocation`: locația și echipamentele arhivate; `ListLocations(false)` gol; `ListLocations(true)` le conține.
  - `TestAuditWritten`: după save există rând `save_location`.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** → PASS.
- [ ] **Step 5: Commit** — `feat(store): locații și echipamente cu validare și arhivare`.

---

### Task 6: Pinger ICMP nativ (`internal/monitor/icmp`)

**Files:**
- Create: `internal/monitor/icmp/icmp.go`, `internal/monitor/icmp/fake.go`, `internal/monitor/icmp/pinger_linux.go`, `internal/monitor/icmp/classify_linux.go`, `internal/monitor/icmp/pinger_windows.go`, `internal/monitor/icmp/classify_windows.go`, `internal/monitor/icmp/pinger_other.go` (`//go:build !linux && !windows`, returnează eroare la `New`)
- Test: `internal/monitor/icmp/classify_linux_test.go`, `internal/monitor/icmp/classify_windows_test.go`, `internal/monitor/icmp/real_test.go`

**Interfaces:**
- Produces:
  ```go
  type Code string
  const (OK Code = ""; Timeout = "timeout"; HostUnreachable = "host_unreachable"; NetUnreachable = "net_unreachable"; DNS = "dns"; Permission = "permission"; Internal = "internal")
  type Result struct{ Sent, Received int; LatencyMs *float64; Code Code }
  func (r Result) OK() bool            // Received > 0
  func (r Result) MonitorError() bool  // Code ∈ {Permission, Internal}
  func (r Result) LossPct() float64
  type Pinger interface {
      Ping(ctx context.Context, addr netip.Addr, packets int, timeout time.Duration) Result
      Mode() string // "linux-unprivileged" | "linux-raw" | "windows" | "fake"
  }
  func New() (Pinger, error)
  type Fake struct{ Fn func(addr netip.Addr) Result }
  ```

Pachetele se trimit secvențial, cu 200 ms între ele; `Code` final = codul ultimului pachet fără răspuns dacă `Received==0`, altfel `OK`. Linux: încearcă `icmp.ListenPacket("udp4"/"udp6", "")` (neprivilegiat); la `EPERM`/`EACCES` încearcă `"ip4:icmp"`/`"ip6:ipv6-icmp"` (raw); dacă ambele eșuează, `New()` returnează un pinger care răspunde mereu `Code: Permission`, iar `Mode()` = `"unavailable"`. Răspunsurile ICMP Destination Unreachable care conțin ID-ul/secvența noastră → `HostUnreachable` (code 1/3) sau `NetUnreachable` (code 0). Windows: `IcmpCreateFile`/`Icmp6CreateFile` + `IcmpSendEcho2`/`Icmp6SendEcho2` prin `golang.org/x/sys/windows.NewLazySystemDLL("iphlpapi.dll")`.

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestClassifyErrnoLinux` (build linux): `ENETUNREACH`→NetUnreachable, `EHOSTUNREACH`→HostUnreachable, `EPERM`/`EACCES`→Permission, `os.ErrDeadlineExceeded`→Timeout, alt errno→Internal.
  - `TestClassifyDestUnreachableLinux`: ICMP type 3 code 0 → NetUnreachable; code 1 → HostUnreachable; code 3 → HostUnreachable.
  - `TestClassifyStatusWindows` (build windows): `IP_SUCCESS`(0)→OK, `IP_REQ_TIMED_OUT`(11010)→Timeout, `IP_DEST_HOST_UNREACHABLE`(11003)→HostUnreachable, `IP_DEST_NET_UNREACHABLE`(11002)→NetUnreachable, `IP_DEST_PORT_UNREACHABLE`(11005)→HostUnreachable, `IP_GENERAL_FAILURE`(11050)→Internal.
  - `TestRealLoopback` (sare cu `t.Skip` dacă `testing.Short()` sau `Mode()=="unavailable"`): `Ping(127.0.0.1, 2, 1s)` → `Received ≥ 1`, `LatencyMs != nil`.
  - `TestRealTestNet` (aceleași condiții de skip): `Ping(192.0.2.1, 1, 1s)` → `OK()==false`, `Code` ∈ {Timeout, HostUnreachable, NetUnreachable}.
  - `TestResultHelpers`: `{Sent:3,Received:1}` → `OK()`, `LossPct()==66.67` (±0,01).
- [ ] **Step 2: Rulează** `go test ./internal/monitor/icmp/` → FAIL.
- [ ] **Step 3: Implementează.**
- [ ] **Step 4: Rulează** pe Windows (local) → PASS; `GOOS=linux go vet ./internal/monitor/icmp/` fără erori. Testele Linux rulează în CI (Plan 3) și pe un server Linux.
- [ ] **Step 5: Commit** — `feat(icmp): pinger nativ Windows/Linux cu clasificarea erorilor`.

---

### Task 7: Rezolvare DNS cu cache și hub `live`

**Files:**
- Create: `internal/monitor/resolver.go`, `internal/live/hub.go`, `internal/live/views.go`
- Test: `internal/monitor/resolver_test.go`, `internal/live/hub_test.go`

**Interfaces:**
- Consumes: `state`, `store.ListLocations`, `device_status`, `store.OpenState`.
- Produces:
  ```go
  // monitor
  type LookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)
  type Resolver struct{ ... }
  func NewResolver(lookup LookupFunc, timeout time.Duration, preferV4 func() bool) *Resolver
  func (r *Resolver) Resolve(ctx context.Context, host string) (addr netip.Addr, dnsWarning bool, err error)
  // live
  type DeviceView struct{ ID, LocationID int64; Name, Host, Kind string; Primary bool; State, Pending state.State; Since int64; LastCheck *int64; LatencyMs, LossPct *float64; ErrorCode string; DNSWarning bool } // json snake_case
  type LocationView struct{ ID int64; Name, Address, Notes string; Lat, Lon *float64; State state.State; Incomplete bool; Since int64; Devices []DeviceView }
  type Health struct{ LastCycle *int64; LateChecks int; AvgCheckMs float64; ICMPMode string; Errors []string; DBSizeBytes int64; LastBackup *int64; BackupError string; Warning string }
  type Event struct{ Seq uint64; Type string; Data any } // Type: "state" (LocationView) | "health"
  type Snapshot struct{ Seq uint64; Locations []LocationView; Health Health }
  type Hub struct{ ... }
  func NewHub() *Hub
  func (h *Hub) Load(ctx context.Context, st *store.Store) error   // reconstruire completă, emite "state" pentru locațiile schimbate
  func (h *Hub) SetDevice(dv DeviceView)                            // recalculează locația, emite "state"
  func (h *Hub) SetLocationState(id int64, r state.LocationResult, since int64)
  func (h *Hub) SetHealth(hl Health)                                // emite "health"
  func (h *Hub) Snapshot() Snapshot
  func (h *Hub) Subscribe(afterSeq uint64) (events <-chan Event, cancel func())
  ```

`Resolve`: IP literal → fără lookup. Altfel lookup cu `timeout`; succes → alege prima IPv4 dacă `preferV4()`, altfel prima adresă, o memorează; eșec cu adresă memorată → (adresa, true, nil); eșec fără adresă → err. `Subscribe`: canal cu buffer 256; păstrează ultimele 512 evenimente pentru reluare de la `afterSeq`; un abonat lent (buffer plin) este deconectat (canal închis), nu blochează hub-ul. Ordinea locațiilor în `Snapshot`: gravitate (down, degraded, error/unknown, up), apoi nume.

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestResolveLiteral`: `"10.0.0.1"` → lookup neapelat.
  - `TestResolveCachesAndWarns`: primul lookup reușit, al doilea eșuat → aceeași adresă, `dnsWarning=true`.
  - `TestResolveNoCacheFails`: lookup eșuat din prima → err.
  - `TestResolveTimeout`: lookup care blochează → err în ~timeout (timeout de test 50 ms).
  - `TestResolvePrefersV4`: [::1, 127.0.0.1] → 127.0.0.1.
  - `TestHubSetDeviceRecomputesLocation`: locație cu P up, S up; `SetDevice(S down)` → `Snapshot` arată locația `degraded`, iar abonatul primește un eveniment `state`.
  - `TestHubReplayAfterSeq`: 3 evenimente; `Subscribe(1)` primește evenimentele 2 și 3.
  - `TestHubSlowSubscriberDropped`: abonat care nu citește + 300 de evenimente → canalul lui e închis; `SetDevice` nu blochează (test cu timeout de 1 s).
  - `TestSnapshotOrder`: up „A”, down „Z” → „Z” primul.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** cu `-race` → PASS.
- [ ] **Step 5: Commit** — `feat: resolver DNS cu cache și hub de stare live`.

---

### Task 8: Scriere rezultate, incidente, pauze (`internal/monitor/writer.go`)

**Files:**
- Create: `internal/monitor/writer.go`, `internal/monitor/result.go`
- Test: `internal/monitor/writer_test.go`

**Interfaces:**
- Consumes: `state.Device.Apply/Expire`, `store.CloseAndOpen`, `store.RecomputeLocation`, `settings.Holder`, `live.Hub`, `icmp.Result`.
- Produces:
  ```go
  type Result struct{ DeviceID, Rev int64; At time.Time; Ping icmp.Result; DNSWarning bool }
  type Writer struct{ FlushEvery time.Duration /* implicit 1s */; GapEvery time.Duration /* implicit 30s */ }
  func NewWriter(st *store.Store, hub *live.Hub, set *settings.Holder) *Writer
  func (w *Writer) Submit(r Result)               // neblocant; canal cu buffer 1024
  func (w *Writer) Run(ctx context.Context) error // la start: Reload + ExpireGaps(now); apoi bucla de flush/gap
  func (w *Writer) Reload(ctx context.Context) error   // reîncarcă device_status, rev, primar, suprascrieri
  func (w *Writer) Flush(ctx context.Context) error    // aplică imediat tot ce e în coadă (pentru teste)
  func (w *Writer) ExpireGaps(ctx context.Context, now time.Time) error
  ```

Pentru fiecare rezultat dintr-un lot (o tranzacție pe lot): ignoră dacă echipamentul e arhivat sau `Rev` ≠ rev curent; dacă `last_check_at + stale < At` → întâi aplică pauza (vezi mai jos); inserează în `samples`; `Device.Apply` cu pragurile efective; la tranziție: `CloseAndOpen(device, To, At)`; To=Down și (`incidents_for=="all"` sau primar) → incident cu `observed_at=At`, `confirmed_at=ConfirmedAt`; To≠Down și From=Down → incident închis la `At` cu `recovered`, sau cu `monitor_error` dacă To=Error; `RecomputeLocation(At)`; upsert `device_status`. După commit: `hub.SetDevice` pentru fiecare echipament modificat.

Pauză (`ExpireGaps` și la sosirea unui rezultat după pauză): pentru fiecare echipament activ cu `last_check_at + stale < now` și stare ≠ unknown: `g = last_check_at + stale`; `CloseAndOpen(device, unknown, g)`; incident deschis închis la `g` cu `monitoring_gap`; `Device.Expire()`; `RecomputeLocation(g)`. Echipamentele fără nicio verificare rămân `unknown`.

- [ ] **Step 1: Scrie testele eșuate** (bază temporară, praguri implicite, `Flush` explicit):
  - `TestWriterDownIncidentRetroactive`: rezultate OK,OK,F,F,F la t0..t4 (pas 60 s) → interval `up` [t0,t2], `down` de la t2; incident `observed=t2`, `confirmed=t4`; locația (echipament primar) `down` de la t2.
  - `TestWriterRecovery`: continuare OK,OK la t5,t6 → incident `ended=t5`, `recovered`; interval `up` de la t5.
  - `TestWriterMonitorError`: în Down, rezultat `Code: Permission` → stare `error`, incident închis `monitor_error`.
  - `TestWriterIncidentsPrimaryOnly`: `incidents_for="primary"`, echipament secundar cade → niciun incident, dar interval `down` există.
  - `TestWriterDropsStaleRevision` (Review Focus 2): host schimbat prin `SaveLocation` (rev 2), apoi `Submit` cu `Rev:1` → nicio înregistrare în `samples`.
  - `TestWriterExpiresGapAtStartup` (Review Focus 3): `device_status` down cu `last_check = now−1h`, incident deschis → `Run` pornește, iar după start intervalul curent e `unknown` de la `last_check+300s`; incidentul `ended = last_check+300s`, `monitoring_gap`.
  - `TestWriterGapOnLateResult`: OK la t0, următorul OK la t0+600 s → interval `unknown` [t0+300, …], apoi seria reîncepe (starea rămâne unknown până la `recoveries`).
  - `TestWriterUpdatesHub`: după o tranziție, `hub.Snapshot()` reflectă noua stare și latența.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** cu `-race` → PASS.
- [ ] **Step 5: Commit** — `feat(monitor): scriitor unic cu incidente, atribuire retroactivă și pauze`.

---

### Task 9: Scheduler și serviciul de monitorizare (`internal/monitor`)

**Files:**
- Create: `internal/monitor/scheduler.go`, `internal/monitor/service.go`
- Test: `internal/monitor/scheduler_test.go`, `internal/monitor/service_test.go`

**Interfaces:**
- Consumes: `icmp.Pinger`, `Resolver`, `Writer`, `store.ListLocations`, `settings.Holder.Get().Apply`.
- Produces:
  ```go
  type Target struct{ DeviceID, Rev int64; Host string; Interval, Timeout time.Duration; Packets int }
  type SchedStats struct{ LastCycle time.Time; LateChecks int; AvgCheck time.Duration }
  type Scheduler struct{ ... }
  func NewScheduler(p icmp.Pinger, r *Resolver, submit func(Result), workers int) *Scheduler
  func (s *Scheduler) SetTargets(ts []Target) // diff după DeviceID+Rev; țintele noi eșalonate uniform în [0, Interval)
  func (s *Scheduler) Run(ctx context.Context)
  func (s *Scheduler) Stats() SchedStats
  type Service struct{ ... }
  func NewService(st *store.Store, set *settings.Holder, hub *live.Hub, p icmp.Pinger, lookup LookupFunc) *Service
  func (m *Service) Run(ctx context.Context) error             // pornește Writer + Scheduler + raportarea Health la 5 s
  func (m *Service) Reload(ctx context.Context) error          // după modificări de configurare: Writer.Reload, SetTargets, hub.Load
  func (m *Service) TestHost(ctx context.Context, host string) (icmp.Result, error) // validează cu domain.ValidHost; rezolvă; ping cu setările curente
  func (m *Service) Health() live.Health
  ```

O verificare întârziată (pornită după `termen + 1 s`) incrementează `LateChecks` (fereastră glisantă de 10 min). Un echipament nu are niciodată două verificări simultane; termenul următor = `max(termen + Interval, sfârșitul verificării)`. Eșecul DNS fără cache → `Result{Ping: icmp.Result{Sent: packets, Code: icmp.DNS}}`. `Health.Warning` = „Monitorizarea rămâne în urmă” dacă `LateChecks > 0` în ultimele 10 min, sau „Monitorul nu a scris date recent” dacă ultimul flush e mai vechi de 2 × interval.

- [ ] **Step 1: Scrie testele eșuate** (`icmp.Fake`, intervale de 50 ms):
  - `TestSchedulerNoOverlap`: Fake care durează 120 ms, interval 50 ms → pentru un echipament, maximum o verificare concurentă (contor atomic), iar `LateChecks > 0`.
  - `TestSchedulerStaggers`: 10 ținte cu interval 1 s → primele porniri distribuite (diferența max−min ≥ 500 ms).
  - `TestSchedulerConcurrencyCap`: 100 de ținte, workers 64, Fake de 100 ms → concurență maximă observată ≤ 64.
  - `TestSchedulerSetTargetsRemoves`: o țintă eliminată nu mai e verificată după `SetTargets`.
  - `TestServiceEndToEnd`: bază cu o locație; Fake reușit; setările din test suprascrise la `IntervalS` minim prin injectare de interval (opțiune de test `Service.intervalScale`) → în 2 s `hub.Snapshot()` arată `up`.
  - `TestTestHostRejectsInvalid`: `"a b"` → `*apperr.Error` 400.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează** (timer per țintă într-un min-heap; semafor de `workers`). **Step 4: Rulează** cu `-race` → PASS.
- [ ] **Step 5: Commit** — `feat(monitor): scheduler eșalonat fără suprapuneri și serviciu de monitorizare`.

---

### Task 10: Statistici (`internal/stats`)

**Files:**
- Create: `internal/stats/query.go`, `internal/stats/report.go`, `internal/stats/timeline.go`, `internal/stats/latency.go`, `internal/stats/report24.go`, `internal/stats/reasons.go`
- Test: `internal/stats/report_test.go`, `internal/stats/latency_test.go`, `internal/stats/perf_test.go`

**Interfaces:**
- Consumes: `store` (intervale, incidente, samples, samples_hourly), `live.Snapshot`.
- Produces:
  ```go
  type Query struct{ From, To time.Time; LocationID, DeviceID *int64; Kind, Incidents string } // Incidents: all|active|closed
  func ParseQuery(v url.Values, now time.Time) (Query, error) // from/to ms; implicit ultimele 24 h; to ≤ now; to−from ∈ (0, 400 zile]
  type Durations struct{ Up, Down, Degraded, Unknown, Error float64 } // secunde
  type DeviceRow struct{ ID, LocationID int64; Location, Name, Host, Kind string; Archived bool; Durations Durations; Availability, Coverage, LatencyMs, LossPct *float64; Incidents int; MTTRS *float64 }
  type LocationRow struct{ ID int64; Name string; Archived bool; Durations Durations; Availability, Coverage *float64 }
  type IncidentRow struct{ ID, DeviceID, LocationID int64; Location, Device string; ObservedAt, ConfirmedAt int64; EndedAt *int64; Reason, ReasonText string; DurationS float64 }
  type Summary struct{ Availability, Coverage *float64; Incidents int; DownS float64; MTTRS *float64 }
  type Report struct{ From, To int64; Summary Summary; Devices []DeviceRow; Locations []LocationRow; Incidents []IncidentRow }
  func Build(ctx context.Context, st *store.Store, q Query, now time.Time) (Report, error)
  type Segment struct{ Start, End int64; State state.State }
  func Timeline(ctx context.Context, st *store.Store, deviceID int64, from, to, now time.Time) ([]Segment, error)
  type Point struct{ T int64; Avg, Min, Max, LossPct *float64 }
  func Latency(ctx context.Context, st *store.Store, deviceID int64, from, to time.Time) ([]Point, error)
  type Report24 struct{ From, To int64; Locations int; AffectedNow int; Current []CurrentDown; Incidents []IncidentRow }
  type CurrentDown struct{ LocationID int64; Location, Device string; Since int64 }
  func BuildReport24(ctx context.Context, st *store.Store, snap live.Snapshot, now time.Time) (Report24, error)
  func ReasonText(code string) string // recovered→„Revenire”, monitoring_gap→„Lipsă monitorizare”, monitor_error→„Eroare monitor”, configuration→„Configurație modificată”, archived→„Arhivat”, ""→„În curs”
  ```

Calcul (spec §4 Indicatori): perioada eligibilă = [max(from, created_at), min(to, archived_at ?? now)]; durata pe stare = suprapunerea intervalelor (intervalul deschis se termină la `now`) cu perioada eligibilă, printr-o singură interogare SQL agregată pe entitate (`SUM(MIN(end,?) − MAX(start,?))` cu `WHERE start < ? AND (end IS NULL OR end > ?)`); timpul eligibil neacoperit de intervale se adaugă la `Unknown`. Disponibilitate echipament = up/(up+down); locație = up/(up+degraded+down); acoperire = (up+down+degraded)/eligibil; `Summary.Availability` = ponderată (Σup/Σ(up+down)). Incidente: `observed_at < to AND (ended_at IS NULL OR ended_at > eligible_from)`, durată limitată la perioadă, de la `observed_at`. MTTR = media duratelor complete (`ended−observed`) ale incidentelor `recovered` încheiate în perioadă. Filtrele `location/device/kind` limitează echipamentele; locațiile listate = cele ale echipamentelor rămase. `Latency`: span ≤ 48 h → din `samples`, găleți de `max(span/500, 1 s)`; ≤ 7 zile → `samples`, găleți de 15 min; altfel → `samples_hourly`, găleți de `max(1 h, span/500)` rotunjite la oră.

- [ ] **Step 1: Scrie testele eșuate** (intervale inserate direct în bază):
  - `TestDurationsClipping`: up [100,150), down [150, deschis); query [125,400], now=400 → up 25, down 250, unknown 0.
  - `TestDurationsUncoveredIsUnknown`: interval up [200,300), eligibil [100,400] → unknown 200.
  - `TestAvailabilityExcludesUnknown`: up 60, down 20, unknown 20 → disponibilitate 75, acoperire 80.
  - `TestLocationDegradedCountsAgainst`: locație up 50, degraded 25, down 25 → 50.
  - `TestReportDeviceCreatedAfterPeriod` (Review Focus 4): `created_at > to` → echipamentul lipsește din raport; perioadă cu eligibil 0 → `Availability == nil`, fără panică.
  - `TestIncidentDurationFromObserved`: observed 100, confirmed 220, ended 400; query [0,300] → 200 s.
  - `TestIncidentsFilter`: `active` → doar `ended_at IS NULL`; `closed` → doar cele încheiate; disponibilitatea nu se schimbă.
  - `TestMTTR`: două incidente `recovered` de 60 s și 180 s → 120.
  - `TestParseQuery`: 401 zile → field `from`; `from ≥ to` → field `to`; `NaN`/text → 400; fără parametri → ultimele 24 h.
  - `TestTimelineFillsGaps`: up [100,250), down [300,400) în [100,450] → up, unknown, down, unknown (aceleași valori ca testul din 0.4).
  - `TestLatencyResolution`: 3 zile de samples la 60 s → bucket 15 min (≤ 500 de puncte, min ≤ avg ≤ max); 30 de zile de `samples_hourly` → ≤ 500 de puncte.
  - `TestReport24Current`: snapshot cu un primar down de la t → `Current[0].Since == t`, `AffectedNow == 1`.
  - `TestReportPerformanceOneYear` (sare dacă `testing.Short()`): 60 de echipamente, 20 de locații, un an cu ~10 schimbări de stare/echipament/zi → `Build` pe toată perioada < 500 ms; dimensiunea bazei (fără samples) < 300 MB.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** `go test ./internal/stats/` (inclusiv testul de performanță) → PASS.
- [ ] **Step 5: Commit** — `feat(stats): durate, disponibilitate, incidente, MTTR, latență, raport 24H`.

---

### Task 11: Autentificare și protecții (`internal/auth`)

**Files:**
- Create: `internal/auth/password.go`, `internal/auth/sessions.go`, `internal/auth/limiter.go`, `internal/auth/proxy.go`
- Test: `internal/auth/password_test.go`, `internal/auth/sessions_test.go`, `internal/auth/limiter_test.go`, `internal/auth/proxy_test.go`

**Interfaces:**
- Produces:
  ```go
  func HashPassword(pw string) (string, error)          // format PHC: $argon2id$v=19$m=65536,t=3,p=2$<salt b64>$<hash b64>
  func VerifyPassword(encoded, pw string) bool          // comparare în timp constant
  func ValidatePassword(pw string) error                // 12–200 caractere (runes) → apperr field "password"
  type Sessions struct{ Idle, Absolute time.Duration }
  func NewSessions(st *store.Store) *Sessions            // 8 h / 24 h
  func (s *Sessions) Create(ctx context.Context, now time.Time) (token, csrf string, err error)
  func (s *Sessions) Validate(ctx context.Context, token string, now time.Time) (csrf string, ok bool, err error)
  func (s *Sessions) Delete(ctx context.Context, token string) error
  func (s *Sessions) DeleteAll(ctx context.Context) error
  func (s *Sessions) Cleanup(ctx context.Context, now time.Time) error
  type Limiter struct{ ... }
  func NewLimiter(max int, window time.Duration, capacity int) *Limiter // 5, 5 min, 10000
  func (l *Limiter) Check(ip netip.Addr, now time.Time) (retryAfter time.Duration, ok bool)
  func (l *Limiter) Fail(ip netip.Addr, now time.Time) (delay time.Duration) // min(250ms × 2^(n−1), 4s)
  func (l *Limiter) Reset(ip netip.Addr)
  type Proxy struct{ Trusted []netip.Prefix }
  func ParseProxies(list []string) (Proxy, error)
  type ClientInfo struct{ IP netip.Addr; Scheme, Host string; ViaProxy bool }
  func (p Proxy) Client(r *http.Request) ClientInfo
  func (c ClientInfo) DirectLoopback() bool             // IP loopback și !ViaProxy și fără antete X-Forwarded-*
  func SameOrigin(r *http.Request, c ClientInfo) bool   // Origin (sau Referer) == c.Scheme+"://"+c.Host; lipsă ambele → false pentru metode care modifică date
  ```

`Validate` actualizează `last_seen_at` cel mult o dată pe minut. `Limiter` la capacitate maximă elimină intrarea cea mai veche (nu golește tot). `Client`: `X-Forwarded-For` (ultimul hop ne-de-încredere de la dreapta), `X-Forwarded-Proto`, `X-Forwarded-Host` sunt citite doar dacă `RemoteAddr` ∈ `Trusted`; altfel `Scheme` = `https` dacă `r.TLS != nil`.

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestPasswordRoundTrip`: hash/verify; parolă greșită → false; hash-ul conține `m=65536,t=3,p=2`.
  - `TestValidatePassword`: 11 caractere → eroare; 12 caractere cu diacritice (`ăăăăăăăăăăăă`) → ok; 201 → eroare.
  - `TestSessionLifecycle`: Create → Validate ok; în bază `token_hash` ≠ token; după 8 h + 1 s fără activitate → invalid; activitate continuă, dar după 24 h + 1 s → invalid; `DeleteAll` → invalid.
  - `TestLimiter`: 5 eșecuri → `Check` false, `retryAfter > 0`; după fereastră → ok; întârzieri 250 ms, 500 ms, 1 s, 2 s, 4 s, 4 s; capacitate 2 cu 3 IP-uri → primul IP eliminat, ceilalți păstrați.
  - `TestProxyUntrustedIgnoresHeaders`: RemoteAddr 192.0.2.9 cu `X-Forwarded-For: 1.2.3.4` → IP 192.0.2.9, `ViaProxy=false`.
  - `TestProxyTrusted`: RemoteAddr 10.0.0.5 (trusted `10.0.0.5/32`), `X-Forwarded-For: 1.2.3.4, 10.0.0.7`, `X-Forwarded-Proto: https`, `X-Forwarded-Host: nw.local` → IP 10.0.0.7 dacă 10.0.0.7 nu e de încredere; scheme https; host nw.local.
  - `TestSameOriginBehindProxy`: aceeași configurație, `Origin: https://nw.local` → true (regresia din 0.4).
  - `TestDirectLoopback`: 127.0.0.1 fără antete → true; cu `X-Forwarded-For` → false.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** → PASS.
- [ ] **Step 5: Commit** — `feat(auth): argon2id, sesiuni pe server, rate-limit, proxy de încredere`.

---

### Task 12: Fundația HTTP și API-ul public (`internal/api`)

**Files:**
- Create: `internal/api/server.go`, `internal/api/respond.go`, `internal/api/middleware.go`, `internal/api/public.go`, `internal/api/identity.go`
- Test: `internal/api/public_test.go`, `internal/api/middleware_test.go`

**Interfaces:**
- Consumes: `live.Hub`, `stats.*`, `store`, `settings.Holder`, `auth.Proxy`.
- Produces:
  ```go
  type Monitor interface {
      Reload(ctx context.Context) error
      TestHost(ctx context.Context, host string) (icmp.Result, error)
      Health() live.Health
  }
  type Backups interface {
      List() ([]jobs.BackupInfo, error)
      Create(ctx context.Context, kind string) (jobs.BackupInfo, error)
      Open(name string) (io.ReadSeekCloser, jobs.BackupInfo, error) // name validat: fără separatori de cale
  }
  type Deps struct {
      Store *store.Store; Hub *live.Hub; Settings *settings.Holder; Monitor Monitor; Backups Backups
      Sessions *auth.Sessions; Limiter *auth.Limiter; Proxy auth.Proxy
      Web fs.FS   // nil în Plan 1; Plan 2 servește UI-ul de aici
      Version string; Now func() time.Time
  }
  func New(d Deps) http.Handler
  type Identity struct{ Name, Subtitle, ReportTitle string; LogoVersion int64 } // meta "identity"; implicit „NetWatch”, „NETWORK MONITOR”, „Raport de disponibilitate”
  func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error // MaxBytesReader 1 MB, DisallowUnknownFields, un singur obiect; erorile → apperr cu mesaj în română
  func writeError(w http.ResponseWriter, err error) // *apperr.Error → status și JSON; altfel 500 cu mesaj generic în română și jurnalizare
  ```

Rute publice (spec §6): `GET /api/state` (Snapshot + identitate + `settings.interval_s` + `version`), `GET /api/catalog` (locații și echipamente, inclusiv arhivate), `GET /api/stats`, `GET /api/devices/{id}/timeline`, `GET /api/devices/{id}/latency`, `GET /api/report24`. Middleware: recuperare din panică → 500 JSON; antete de securitate (CSP `default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`); `Cache-Control: no-store` pe `/api/`; jurnal de acces `slog` la nivel Debug. 404 JSON pentru `/api/*` necunoscut. Dacă `Web` e nil, `/` returnează o pagină text minimă „NetWatch API”.

- [ ] **Step 1: Scrie testele eșuate** (`httptest`, Deps cu bază temporară, hub încărcat, Monitor fals):
  - `TestStateEndpoint`: 200, JSON cu `locations[0].devices[0].state`, `identity.name == "NetWatch"`, `version`.
  - `TestStatsInvalidParams`: `?from=abc` → 400 cu `error` în română și `field == "from"`; `?from=0&to=99999999999999` → 400 (perioadă > 400 de zile).
  - `TestTimelineUnknownDevice`: `/api/devices/999/timeline` → 404 JSON.
  - `TestSecurityHeaders`: CSP exact ca mai sus; `/api/state` are `Cache-Control: no-store`.
  - `TestPanicRecovered`: handler de test care panichează → 500 JSON, serverul continuă.
  - `TestDecodeJSONStrict` (unitar): câmp necunoscut → 400; corp > 1 MB → 413 cu mesaj în română; JSON trunchiat → 400; `null` → 400.
  - `TestUnknownAPIRoute`: `/api/nimic` → 404 JSON.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** → PASS.
- [ ] **Step 5: Commit** — `feat(api): fundație HTTP, erori uniforme, endpoint-uri publice`.

---

### Task 13: Server-Sent Events (`internal/api/events.go`)

**Files:**
- Create: `internal/api/events.go`
- Test: `internal/api/events_test.go`

**Interfaces:**
- Consumes: `live.Hub.Subscribe`, `live.Event`.
- Produces: `GET /api/events` — mesaje `id: <seq>\nevent: <type>\ndata: <json>\n\n`; la conectare, dacă lipsește `Last-Event-ID`, trimite întâi un eveniment `snapshot` cu `Snapshot` complet; cu `Last-Event-ID` reia de la acel seq (dacă nu mai e în istoric → `snapshot`); comentariu `: ping` la 15 s; antete `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`; flush după fiecare mesaj; la închiderea canalului de abonat (client lent) închide răspunsul.

- [ ] **Step 1: Scrie testele eșuate** (server `httptest.NewServer` real):
  - `TestEventsHeadersAndFlush` (Review Focus 5): antetele exacte de mai sus; primul eveniment `snapshot` este primit în < 1 s, fără ca serverul să închidă conexiunea.
  - `TestEventsReceivesStateChange`: după `hub.SetDevice(…down)` clientul primește `event: state` cu locația `down`.
  - `TestEventsResumeLastEventID`: reconectare cu `Last-Event-ID: <n>` → primește evenimentele > n, fără `snapshot`.
  - `TestEventsPing`: cu intervalul ping injectat la 50 ms → linie `: ping`.
  - `TestEventsClientDisconnect`: anularea contextului clientului → abonatul hub-ului este eliberat (numărul de abonați revine la 0).
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** cu `-race` → PASS.
- [ ] **Step 5: Commit** — `feat(api): flux SSE cu reluare și ping`.

---

### Task 14: API de administrare (`internal/api/admin*.go`)

**Files:**
- Create: `internal/api/admin_auth.go`, `internal/api/admin_locations.go`, `internal/api/admin_settings.go`, `internal/api/admin_misc.go`
- Test: `internal/api/admin_auth_test.go`, `internal/api/admin_locations_test.go`, `internal/api/admin_settings_test.go`, `internal/api/admin_misc_test.go`

**Interfaces:**
- Consumes: `auth.*`, `store.SaveLocation/ArchiveLocation/Audit`, `settings.Holder.Save`, `Monitor.Reload/TestHost/Health`, `Backups`.
- Produces (toate sub `/api/admin`, JSON):

| Rută | Comportament |
|---|---|
| `GET /session` | `{authenticated, configured, csrf?, setup_allowed, tls}`; `setup_allowed` = fără parolă și `DirectLoopback()`. |
| `POST /setup` | `{password}`; doar dacă `setup_allowed`, altfel 403; setează parola, creează sesiunea. |
| `POST /login` | `{password}`; `SameOrigin` obligatoriu; rate-limit → 429 cu `Retry-After`; eșec → `Fail` cu întârziere aplicată, 401; succes → `Reset`, cookie, `{csrf}`. |
| `POST /logout` | Șterge sesiunea, expiră cookie-ul. |
| `GET /locations` | `store.ListLocations(true)`. |
| `POST /locations`, `PUT /locations/{id}` | `domain.LocationInput` → `SaveLocation` → `Monitor.Reload` → `{id}`. |
| `DELETE /locations/{id}` | `ArchiveLocation` → `Reload`. |
| `POST /devices/test` | `{host}` → `TestHost` → `{ok, received, sent, latency_ms, code, message}` (`message` în română pentru fiecare cod). |
| `GET /settings`, `PUT /settings` | `settings.Holder.Save` → `Reload`; audit `settings`. |
| `GET /identity`, `PUT /identity` | Nume 1–60, subtitlu ≤ 80, titlu raport 1–120; audit. |
| `GET /audit?before&limit` | Paginat descrescător, `limit` ≤ 200 (implicit 50). |
| `GET /diagnostic` | `Monitor.Health()` + versiune + calea datelor + dimensiunea bazei. |
| `GET /backups`, `POST /backups`, `GET /backups/{name}` | Listă / creare `manual` / descărcare ca atașament. |
| `PUT /password` | `{old, new}`; verifică vechea parolă, `ValidatePassword`; `DeleteAll` sesiuni; audit. |

Middleware admin: sesiune validă (altfel 401); pentru `POST/PUT/DELETE`: `X-CSRF-Token` = token-ul sesiunii (comparare în timp constant) **și** `SameOrigin` (altfel 403). Cookie: `nw_session`, `Path=/`, `HttpOnly`, `SameSite=Strict`, `Secure` dacă `ClientInfo.Scheme == "https"`.

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestLoginFlow`: fără sesiune `/locations` → 401; login cu parolă greșită → 401; corectă → 200, cookie `HttpOnly`/`SameSite=Strict`; `POST /locations` fără CSRF → 403; cu CSRF → 200.
  - `TestLoginRequiresSameOrigin`: `Origin: http://evil` → 403.
  - `TestLoginBehindTrustedProxy`: RemoteAddr de încredere, `X-Forwarded-Proto: https`, `Origin: https://nw.local`, `X-Forwarded-Host: nw.local` → 200 și cookie `Secure`.
  - `TestLoginRateLimit`: 5 greșite → al 6-lea 429 cu `Retry-After`.
  - `TestSetupOnlyLoopback`: fără parolă: de la 127.0.0.1 → `setup_allowed: true` și `POST /setup` 200; de la 192.0.2.5 → 403; cu parolă existentă → 403.
  - `TestPasswordChangeInvalidatesSessions`: două sesiuni; schimbare din una → ambele 401 ulterior.
  - `TestSaveLocationCallsReload`: `Monitor` fals înregistrează apelul `Reload`.
  - `TestValidationErrorsAre400`: corp `{"name":5}` → 400 (nu 500); `{"devices":"x"}` → 400; corp gol → 400.
  - `TestDeviceTestEndpoint`: `Monitor.TestHost` fals → `{ok:true, latency_ms:…}`; host invalid → 400.
  - `TestSettingsPutValidation`: `stale_s: 50` → 400 `field: "stale_s"`.
  - `TestBackupDownloadRejectsTraversal`: `GET /backups/..%2Fnetwatch.db` → 400.
  - `TestAuditPaginated`: 3 modificări → `limit=2` returnează 2 rânduri, iar `before` returnează restul.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** → PASS.
- [ ] **Step 5: Commit** — `feat(api): administrare cu sesiuni, CSRF, setări, audit, backup`.

---

### Task 15: Joburi de fundal (`internal/jobs`)

**Files:**
- Create: `internal/jobs/backup.go`, `internal/jobs/rotate.go`, `internal/jobs/aggregate.go`, `internal/jobs/retention.go`, `internal/jobs/runner.go`
- Test: `internal/jobs/backup_test.go`, `internal/jobs/rotate_test.go`, `internal/jobs/aggregate_test.go`, `internal/jobs/runner_test.go`

**Interfaces:**
- Consumes: `store.VacuumInto`, `settings.Holder`, `auth.Sessions.Cleanup`, `live.Hub` (stare backup în Health).
- Produces:
  ```go
  type BackupInfo struct{ Name, Kind string; Created int64; SizeBytes int64 } // json snake_case
  type BackupService struct{ Dir string }
  func NewBackupService(st *store.Store, dataDir string, set *settings.Holder) *BackupService // Dir = <data>/backups
  func (b *BackupService) Create(ctx context.Context, kind string) (BackupInfo, error) // kind: auto|manual|pre-migrate|pre-restore
  func (b *BackupService) List() ([]BackupInfo, error)
  func (b *BackupService) Open(name string) (io.ReadSeekCloser, BackupInfo, error)
  func Rotate(all []BackupInfo, now time.Time) (remove []BackupInfo) // doar kind=auto
  func AggregateHour(ctx context.Context, st *store.Store, hour time.Time) error // idempotent (INSERT OR REPLACE)
  func Retention(ctx context.Context, st *store.Store, now time.Time, keep time.Duration) (deleted int64, err error)
  func Optimize(ctx context.Context, st *store.Store) error
  type Runner struct{ ... }
  func NewRunner(st *store.Store, set *settings.Holder, b *BackupService, s *auth.Sessions, onBackup func(last *int64, errMsg string)) *Runner
  func (r *Runner) Run(ctx context.Context) // fiecare job în goroutine proprie
  ```

Nume fișiere: `netwatch-YYYYMMDD-HHMMSS-<kind>.db` (ora în `Europe/Chisinau`). `Create`: `os.MkdirAll(Dir)`; `VacuumInto` într-un fișier `.tmp`; deschide copia și execută `DELETE FROM sessions`, apoi `VACUUM`; `PRAGMA integrity_check` = `ok`; redenumire atomică; dacă `BackupSecondaryDir` e setat, copiază și acolo (eșecul copiei secundare devine avertisment, nu eroare). `Rotate`: păstrează cel mai nou backup `auto` pentru fiecare dintre ultimele 7 zile, ultimele 4 săptămâni ISO și ultimele 12 luni; restul `auto` se șterg. Runner: backup zilnic la `backup_hour` (verificare la minut; la eșec, reîncercare după 1 h; `onBackup` raportează în Health); agregare la `HH:05` pentru ora anterioară (plus recuperarea orelor lipsă din ultimele 48 h la pornire); retenție + optimizare zilnic la 04:00 (șterge `samples` mai vechi de 90 de zile doar pentru orele existente în `samples_hourly`, în loturi de 10 000); curățare sesiuni la 10 min. Orele programului sunt în `Europe/Chisinau`.

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestBackupCreateExcludesSessions`: o sesiune în bază → backup-ul are 0 rânduri în `sessions`, `integrity_check` = ok, locațiile sunt prezente.
  - `TestBackupCreatesNestedDir`: `Dir` = `<tmp>/a/b/c` inexistent → reușit.
  - `TestBackupSecondaryFailureIsWarning`: director secundar invalid → backup-ul principal reușește; avertismentul e returnat prin `onBackup`.
  - `TestRotate`: backup-uri `auto` zilnice pe 400 de zile + 2 `manual` → rămân ≤ 7+4+12 `auto` și ambele `manual`; cel mai nou backup e păstrat.
  - `TestAggregateHour`: 60 de samples într-o oră (latențe 1..60, 6 eșuate) → `checks=60`, `ok_checks=54`, `lat_min=1`, `lat_max=60`; rularea dublă nu dublează rândul.
  - `TestRetentionOnlyAggregated`: samples vechi de 100 de zile, cu oră agregată și fără → se șterg doar cele agregate.
  - `TestRunnerBackupRetryAfterFailure`: ceas injectat; prima încercare eșuează → următoarea nu are loc înainte de +1 h.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează** (Runner primește `now func() time.Time` și `tick time.Duration` injectabile pentru teste). **Step 4: Rulează** → PASS.
- [ ] **Step 5: Commit** — `feat(jobs): backup cu rotație, agregare orară, retenție, optimizare`.

---

### Task 16: Asamblare aplicație, configurare, TLS, jurnale (`internal/app`, `internal/config`)

**Files:**
- Create: `internal/config/config.go`, `internal/config/dirs_windows.go`, `internal/config/dirs_unix.go`, `internal/app/app.go`, `internal/app/logging.go`, `internal/tlsutil/selfsigned.go`, `internal/lockfile/lock_windows.go`, `internal/lockfile/lock_unix.go`
- Test: `internal/config/config_test.go`, `internal/app/app_test.go`, `internal/tlsutil/selfsigned_test.go`, `internal/lockfile/lock_test.go`

**Interfaces:**
- Consumes: toate pachetele anterioare.
- Produces:
  ```go
  // config
  type Config struct{ DataDir, Listen, TLS, TLSCert, TLSKey string; TrustedProxies []string; Demo bool } // TLS: "" | "auto" | "files"
  func DefaultDataDir() string
  func Load(fs *flag.FlagSet, args []string) (Config, error) // flag-uri peste <data>/netwatch.toml; validare (TLS=files cere cert+key)
  // lockfile
  func Acquire(dir string) (release func(), err error) // ErrLocked dacă e deja blocat; fișierul nu crește
  var ErrLocked error
  // tlsutil
  func EnsureSelfSigned(dir string, hosts []string) (certFile, keyFile string, err error) // ECDSA P-256, 5 ani, reutilizat dacă există
  // app
  type Options struct{ Pinger icmp.Pinger; Lookup monitor.LookupFunc; Ready func(addr string); Web fs.FS }
  func Run(ctx context.Context, cfg Config, opt Options) error // blochează până la ctx.Done; oprire grațioasă în 10 s
  func SetupLogging(dataDir string, level slog.Level) (*slog.Logger, func())
  ```

`Run`: `Acquire(DataDir)`; `store.Open` cu `BeforeMigrate` care face `VacuumInto` în `backups/netwatch-…-pre-migrate.db`; `settings.Load`; `live.NewHub().Load`; `monitor.NewService` (`Pinger` implicit `icmp.New()`, `Lookup` implicit `net.DefaultResolver.LookupNetIP` cu rețeaua `"ip"`); `jobs.NewRunner`; `api.New`; `http.Server{ReadHeaderTimeout: 10s, IdleTimeout: 120s}` (fără `WriteTimeout` global, din cauza SSE); TLS după `cfg.TLS`; toate componentele într-un `errgroup` legat de `ctx`. Jurnale: `lumberjack` `logs/netwatch.log` (5 MB, 5 fișiere, JSON) + text pe stdout.

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestConfigFlagsOverrideToml`: toml cu `listen = ":9000"`, flag `--listen :9100` → `:9100`; `trusted_proxy` repetabil.
  - `TestConfigTLSFilesRequiresPaths`: `--tls files` fără cert → eroare în română.
  - `TestLockExclusive`: al doilea `Acquire` → `ErrLocked`; după `release` → reușit; dimensiunea fișierului rămâne aceeași după 3 cicluri.
  - `TestSelfSignedReused`: două apeluri → aceleași fișiere; certificatul e valabil ≥ 4 ani și include `localhost`.
  - `TestAppRunIntegration`: `Run` cu `icmp.Fake` reușit, `Listen: "127.0.0.1:0"`, `Ready` returnează adresa → `GET /api/state` 200; setup de pe loopback; `POST /api/admin/locations` cu o locație; în ≤ 3 s (interval scalat pentru test) starea e `up` prin SSE; anularea `ctx` → `Run` se întoarce în < 10 s cu `nil`.
  - `TestAppTLSAuto`: `TLS: "auto"` → clientul cu `InsecureSkipVerify` primește 200 pe HTTPS.
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** `go test -race ./internal/...` → PASS.
- [ ] **Step 5: Commit** — `feat(app): asamblare, configurare toml/flag-uri, TLS automat, lock, jurnale`.

---

### Task 17: CLI și serviciu de sistem (`cmd/netwatch`, `internal/svc`)

**Files:**
- Modify: `cmd/netwatch/main.go`
- Create: `cmd/netwatch/commands.go`, `internal/svc/svc.go`, `internal/svc/console_windows.go`, `internal/svc/console_other.go`, `internal/restore/restore.go`
- Test: `cmd/netwatch/commands_test.go`, `internal/restore/restore_test.go`

**Interfaces:**
- Consumes: `app.Run`, `config.Load`, `auth.HashPassword/ValidatePassword`, `jobs.BackupService`, `store.Open`, `lockfile.Acquire`.
- Produces: subcomenzile din spec §10 (`run`, `install`, `uninstall`, `start`, `stop`, `status`, `set-password`, `backup`, `restore FIȘIER`, `version`; `demo` este adăugat în Plan 3).
  ```go
  // svc
  func Program(cfg config.Config) service.Interface          // Start pornește app.Run în goroutine; Stop anulează ctx
  func Control(action string, cfg config.Config) error        // install|uninstall|start|stop|status; Linux: unitate cu User=netwatch, AmbientCapabilities=CAP_NET_RAW, CapabilityBoundingSet=CAP_NET_RAW, Restart=on-failure
  func Running(cfg config.Config) (bool, error)
  func SetupConsoleUTF8()                                      // Windows: SetConsoleOutputCP(65001); altfel nimic
  // restore
  func Verify(path string) error                               // integrity_check ok, schema_version ≤ SchemaVersion, tabele obligatorii
  func Restore(ctx context.Context, dataDir, src string) (preRestore jobs.BackupInfo, err error) // presupune lock-ul deținut
  ```

`set-password`: citește fără ecou (`golang.org/x/term`), de două ori; `ValidatePassword`; salvează hash-ul în meta `password_hash`; `Sessions.DeleteAll`. `restore`: dacă serviciul rulează → `Control("stop")`; `Acquire`; `Verify`; backup `pre-restore`; `VacuumInto` din sursă într-un `.tmp` lângă baza curentă; șterge `-wal`/`-shm`; redenumire atomică; `store.Open` (migrații); `DELETE FROM sessions`; eliberează lock-ul; dacă serviciul rula → `Control("start")`. Toate mesajele de consolă sunt în română; `SetupConsoleUTF8` se apelează primul în `main`.

- [ ] **Step 1: Scrie testele eșuate:**
  - `TestUnknownCommandUsage`: `run([]string{"x"})` → cod 2, text de utilizare în română.
  - `TestSetPasswordNonInteractive`: cu `stdin` injectat („parola-de-test-12\nparola-de-test-12\n”) → meta `password_hash` verificabil cu `auth.VerifyPassword`; parole diferite → cod 1 și mesajul „Parolele nu coincid.”.
  - `TestBackupCommandPrintsPath`: `backup --data <tmp>` → cod 0, linia afișată se termină în `-manual.db`, iar fișierul există.
  - `TestBackupCommandLocked`: lock deținut de test → cod 1 și mesajul „Directorul de date este folosit de o instanță activă.”.
  - `TestRestoreRoundTrip`: bază A cu locația „X”; backup; adaugă „Y”; `Restore` → doar „X”; există un backup `pre-restore` cu „Y”; `sessions` este gol.
  - `TestRestoreRejectsNewerSchema`: sursă cu `schema_version` 99 → eroare, baza curentă neschimbată.
  - `TestRestoreRejectsCorrupt`: fișier text → eroare „Fișierul nu este o bază NetWatch validă.”.
  - `TestOutputUTF8Redirected`: rulează binarul compilat cu ieșirea redirecționată în fișier → `set-password` cu parole diferite scrie „Parolele nu coincid.” în UTF-8 valid (regresia din 0.4).
- [ ] **Step 2: Rulează** → FAIL. **Step 3: Implementează.** **Step 4: Rulează** `go test ./...` → PASS.
- [ ] **Step 5: Verificare manuală pe Windows** (cu confirmarea utilizatorului, din PowerShell ca administrator): `netwatch install --data C:\ProgramData\NetWatch`, `netwatch status` → „rulează”, `GET http://localhost:8501/api/state` → 200, `netwatch uninstall`. Notează rezultatul în README.
- [ ] **Step 6: Commit** — `feat(cli): comenzi, serviciu Windows/systemd, restaurare sigură`.

---

### Task 18: Documentație de operare și verificare finală

**Files:**
- Modify: `README.md`
- Create: `docs/operare.md`

**Interfaces:**
- Consumes: toate comenzile și rutele din Task 12–17.

- [ ] **Step 1: Scrie `docs/operare.md`** (română): instalare Windows și Linux (inclusiv `ping_group_range` / `CAP_NET_RAW`), prima configurare (`set-password` sau `/administrare` de pe localhost), flag-uri și `netwatch.toml` cu exemplu complet, reverse proxy (exemplu nginx cu `proxy_buffering off` pentru `/api/events` și `--trusted-proxy`), TLS, backup/restaurare, actualizare, jurnale, diagnostic, porturi de firewall, lista rutelor API.
- [ ] **Step 2: Actualizează `README.md`**: descriere, cerințe, pornire rapidă, legătură spre `docs/operare.md`, dezvoltare (`go test ./...`, `go test -short ./...`).
- [ ] **Step 3: Verificare completă:** `go vet ./...` fără erori; `go test -race ./...` → PASS; `CGO_ENABLED=0` build pentru `windows/amd64`, `linux/amd64`, `linux/arm64` → trei binare; `netwatch run --data .\data` local cu o locație `127.0.0.1` → `up` în ≤ 3 minute (praguri implicite).
- [ ] **Step 4: Commit** — `docs: ghid de operare și README`.
