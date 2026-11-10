# NetWatch 1.0 — Plan 2: interfața web — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Interfața web completă a NetWatch (Monitorizare cu panou de detalii, Dispecerat, Hartă, Statistici, Raport 24H, Administrare), încorporată în executabil, plus cele câteva completări de backend de care are nevoie.

**Architecture:** SPA Svelte 5 + TypeScript compilată de Vite în `internal/webui/dist` și servită prin `go:embed` cu fallback SPA. Starea live vine dintr-un singur flux SSE (`/api/events`) ținut într-un store cu rune; paginile grele (hartă, statistici, administrare) se încarcă leneș. Testele: Vitest pentru logica pură din `web/src/lib`, Playwright end-to-end contra unui server Go real cu pinger simulat controlabil (`tools/e2eserver`).

**Tech Stack:** Go 1.27 (backend existent), Svelte 5, TypeScript, Vite, Leaflet + Leaflet.markercluster, uPlot, Vitest, Playwright (Chromium), @axe-core/playwright.

**Spec:** `docs/superpowers/specs/2026-11-09-netwatch-design.md` (secțiunile 4, 6, 7, 8; proxy-ul de dale din secțiunea 6).

**Plan anterior:** `docs/superpowers/plans/2026-11-09-netwatch-1-backend-core.md` (încheiat pe branch-ul `plan1-backend`, commit `0240df7`). Lucrul continuă pe branch nou `plan2-web` creat din `plan1-backend`.

**Mutare de domeniu față de Planul 1:** proxy-ul de dale OSM (`internal/tiles`) trece din Planul 3 în acest plan, pentru că pagina Hartă nu poate fi testată fără el. Rămân în Planul 3: import/export, logo, mod demo, build-uri de release și CI.

## Global Constraints

- Toate textele pentru utilizator în română, cu diacritice corecte (ș, ț cu virgulă).
- Toate orele afișate în `Europe/Chisinau`, indiferent de fusul browserului; paginile de raport afișează „Ora: Europe/Chisinau”.
- Stări: culoare **și** simbol/text — `up` „✓ Funcțională”, `degraded` „▲ Degradată”, `down` „✕ Indisponibilă”, `unknown` „? Fără date”, `error` „! Eroare monitor”.
- Gravitate pentru sortare: `down` < `degraded` < `unknown`/`error` < `up`.
- CSP-ul existent rămâne neschimbat: `default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; …`. Consecință: niciun atribut `style="…"` static în markup, niciun `<script>`/`<style>` inline, niciun HTML generat cu atribute `style`. Stilurile dinamice doar prin directive Svelte `style:prop={…}` sau CSSOM (`el.style.x = …`), care nu sunt blocate de CSP.
- Fără resurse externe la rulare (fonturi de sistem, fără CDN); singura sursă externă este OSM, prin proxy-ul propriu `/tiles`.
- Teme luminoasă și întunecată: implicit urmează sistemul; comutarea manuală se reține în `localStorage["nw-theme"]` (`light`|`dark`); Dispeceratul este implicit întunecat.
- Indicator de conexiune pe toate paginile; dacă nu sosește niciun mesaj SSE timp de **30 s**, stările se marchează „neactualizate” (fără verde fals).
- Navigare completă din tastatură; contrast WCAG AA; regiune `aria-live="polite"` pentru schimbări de stare.
- Node este necesar doar la dezvoltare; executabilul servește `internal/webui/dist` încorporat.
- Proxy dale: `z` 2–18, `x`,`y` în [0, 2^z); cache pe disc `data/tiles/`, 500 MB, LRU; dale valabile 7 zile, servite expirate dacă OSM nu răspunde; maxim 4 descărcări simultane, deduplicate; timeout total 5 s; pool plin → 503 imediat; `User-Agent` cu numele și versiunea aplicației.
- Graficul de latență: maxim ~500 de puncte pe orice interval.
- Comenzi: Go `export PATH="/c/Program Files/Go/bin:$PATH"` în bash; front-end din `web/`: `npm run check`, `npm test`, `npm run build`, `npm run e2e`.
- Commit-urile se termină cu `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Încălcări CSP la rulare** (un atribut `style` static, HTML de popup Leaflet cu stil inline): pagina pare funcțională, dar elemente lipsesc în tăcere. Fixture-ul Playwright din Task 8 eșuează testul la orice `securitypolicyviolation` sau eroare de consolă, pe fiecare pagină.
2. **Text controlat de administrator interpretat ca HTML** (nume de locație `<img src=x onerror=…>` în carduri, popup-uri de hartă, bandă de dispecerat). Test E2E în Task 12 (hartă) și Task 9 (carduri) cu un astfel de nume, care verifică afișarea literală.
3. **Tab de dispecerat deschis zile întregi peste o repornire a serverului:** epoca SSE se schimbă, iar clientul trebuie să primească un snapshot nou (locațiile șterse dispar, nu se dublează). Test Go în Task 1 și test Vitest în Task 7.
4. **Ceasul calculatorului operatorului decalat față de server:** duratele „de 12 min” devin negative sau greșite. Store-ul folosește decalajul față de `now` de la server, iar duratele sunt limitate la ≥ 0. Test Vitest în Task 6/7.
5. **Multe locații pe ecranul de dispecerat** (60+): grila trebuie să încapă fără derulare la 1920×1080. Test E2E în Task 11, care creează 60 de locații prin API.
6. **Perioadă personalizată peste schimbarea orei de vară/iarnă** în `Europe/Chisinau`: intrările de dată/oră se convertesc corect în ms UTC. Test Vitest în Task 6.

---

## File Structure

```
internal/live/hub.go              + Epoch(), id-uri SSE "epoch-seq"
internal/live/views.go            + SortOrder în LocationView; Epoch în Snapshot
internal/api/events.go            id "epoch-seq", ?last=, eveniment ping cu date
internal/api/public.go            + /api/timelines, /api/locations/{id}/timeline
internal/api/admin_locations.go   + PUT /api/admin/locations/order
internal/api/spa.go               handler de fișiere încorporate cu fallback SPA
internal/api/server.go            Deps.Tiles, rute /tiles, SPA
internal/stats/latency.go         găleți ≤ 500 între 48 h și 7 zile
internal/stats/timeline.go        TimelineFor(entity)
internal/store/locations.go       SetLocationOrder
internal/tiles/tiles.go           proxy OSM: validare, cache disc LRU, pool, dedup
internal/webui/webui.go           go:embed all:dist → FS()
internal/app/app.go               Options.Wrap; tiles; Web implicit din webui
cmd/netwatch/commands.go          (neschimbat dacă app folosește webui implicit)
tools/e2eserver/main.go           server real + pinger simulat + port de control
web/                              proiectul Svelte
  package.json, vite.config.ts, tsconfig.json, svelte.config.js, playwright.config.ts
  index.html
  src/main.ts, src/App.svelte
  src/styles/tokens.css, base.css, print.css
  src/lib/api.ts            fetch tipizat, erori {error, field}, CSRF
  src/lib/types.ts          tipurile JSON ale API-ului
  src/lib/router.svelte.ts  router pe History API
  src/lib/time.ts           formatare Europe/Chisinau, conversii, durate
  src/lib/status.ts         etichete, simboluri, gravitate, sortare
  src/lib/periods.ts        perioade rapide
  src/lib/live.svelte.ts    store SSE, conexiune, decalaj ceas
  src/lib/sound.ts          decizii de sunet + Web Audio
  src/lib/theme.ts          temă
  src/lib/*.test.ts         Vitest
  src/components/           Header, ConnectionBadge, StatusBadge, Timeline, LatencyChart, LocationCard, Announcer, SoundToggle, Modal
  src/pages/                Monitor, LocationPanel, Dispatch, MapPage, Stats, Report24, admin/*
  src/assets/contours.json  contururi Natural Earth (regiune)
  e2e/*.spec.ts, e2e/fixtures.ts
```

---

### Task 1: SSE cu epocă, reluare prin query și ping cu date

**Files:**
- Modify: `internal/live/hub.go`, `internal/live/views.go`, `internal/api/events.go`
- Test: `internal/live/hub_test.go`, `internal/api/events_test.go`

**Interfaces:**
- Produces: `func (h *Hub) Epoch() int64` (ms la crearea hub-ului); `Snapshot.Epoch int64 \`json:"epoch"\``; id-uri SSE `"<epoch>-<seq>"`; parametru `GET /api/events?last=<epoch>-<seq>` echivalent cu antetul `Last-Event-ID` (antetul are prioritate); eveniment `event: ping` cu `data: {"now": <ms>}` și fără câmp `id` la fiecare `SSEPing`.
- Consumers: Task 7 (store live).

- [ ] **Step 1: Write the failing tests** în `internal/api/events_test.go`:
  - `TestSSEIDsCarryEpoch`: primul eveniment `snapshot` are `id: <hub.Epoch()>-<seq>` și `data` conține `"epoch":<hub.Epoch()>`.
  - `TestSSEResumeByQuery`: după 2 evenimente publicate, o cerere cu `?last=<epoch>-<seq1>` primește doar evenimentul 2, fără snapshot.
  - `TestSSEEpochMismatchSendsSnapshot`: `Last-Event-ID: 1-1` (epocă străină) → primul eveniment este `snapshot`.
  - `TestSSEPingIsNamedEvent`: cu `SSEPing: 20ms` apare `event: ping` cu `data` JSON care are `now` > 0 și fără linie `id:`.
  - `TestSSEMalformedLastIDSendsSnapshot`: `?last=abc` → `snapshot`, status 200.
- [ ] **Step 2: Run** `go test ./internal/api -run SSE -v` — Expected: FAIL (id-uri fără epocă, ping ca comentariu).
- [ ] **Step 3: Implement.** `Hub.epoch` se setează în `NewHub` la `time.Now().UnixMilli()`. `writeEvent` scrie `id: %d-%d`. În `events` se parsează `epoch-seq` (antet, apoi `last`); orice epocă diferită sau format invalid → snapshot. Ping: `writeEvent` fără id → funcție separată `writePing(w, now)`.
- [ ] **Step 4: Run** `go test ./internal/live ./internal/api -v` — Expected: PASS, inclusiv testele SSE existente (adaptate la noul format de id, dacă verifică id-ul literal).
- [ ] **Step 5: Commit** `feat(sse): id-uri cu epocă, reluare prin ?last, ping cu date`.

### Task 2: Cronologii pe locații, latență ≤ 500 puncte, ordinea locațiilor

**Files:**
- Modify: `internal/stats/timeline.go`, `internal/stats/latency.go`, `internal/api/public.go`, `internal/api/server.go`, `internal/api/admin_auth.go` (rută), `internal/api/admin_locations.go`, `internal/store/locations.go`, `internal/live/views.go`, `internal/live/hub.go` (Load copiază `SortOrder`)
- Test: `internal/stats/latency_test.go`, `internal/stats/report_test.go`, `internal/api/public_test.go`, `internal/api/admin_test.go`, `internal/store/locations_test.go`

**Interfaces:**
- Produces:
  - `func TimelineFor(ctx, st *store.Store, e store.Entity, id int64, from, to, now time.Time) ([]Segment, error)`; `Timeline` rămâne ca înveliș pentru `EntityDevice`.
  - `GET /api/locations/{id}/timeline?from&to` → `{from, to, segments}` (404 dacă locația nu există).
  - `GET /api/timelines?from&to` → `{from, to, locations: {"<id>": [Segment]}}` pentru toate locațiile nearhivate; perioada maximă 30 de zile (altfel 400 cu `field:"from"`).
  - `func (s *Store) SetLocationOrder(ctx, ids []int64, now time.Time) error` — `sort_order` = indexul în listă; id necunoscut sau arhivat → `apperr.Invalid("ids", …)`; audit `locations_order`.
  - `PUT /api/admin/locations/order` cu `{"ids":[…]}` → `{"ok":true}`, apoi `Monitor.Reload`.
  - `LocationView.SortOrder int \`json:"sort_order"\``.
- Consumers: Task 9 (mini-cronologii, sortare manuală), Task 10 (cronologie locație), Task 16 (reordonare locații).

- [ ] **Step 1: Write the failing tests:**
  - `TestLatencyPointBudget` (stats): mostre la fiecare 60 s pe 7 zile → `len(points) <= 500`; la 72 h → `<= 500`; la 24 h rămâne pe brut (lățimea găleții < 15 min).
  - `TestTimelineForLocation` (stats): intervale de locație `up`→`degraded` → două segmente cu stările corecte, golul → `unknown`.
  - `TestTimelinesEndpoint` (api): două locații → cheile `"<id1>"`, `"<id2>"`; `from`/`to` lipsă → ultimele 24 h; 31 de zile → 400 `field:"from"`.
  - `TestLocationTimelineNotFound` (api): id inexistent → 404 cu mesaj în română.
  - `TestSetLocationOrder` (store): `[B, A]` → `ListLocations` le întoarce în ordinea B, A; id inexistent → eroare cu `Field == "ids"`.
  - `TestAdminLocationsOrder` (api): cu sesiune + CSRF → 200; fără CSRF → 403; `/api/state` reflectă `sort_order`.
- [ ] **Step 2: Run** `go test ./internal/stats ./internal/store ./internal/api -run 'Latency|Timeline|Order' -v` — Expected: FAIL.
- [ ] **Step 3: Implement.** Latență pentru 48 h < span ≤ 7 zile: `w = max(15min, roundUp(span/500, 15min))`. `Hub.Load` copiază `SortOrder` din `store.Location`; `SaveLocation` pentru locație nouă pune `sort_order = MAX(sort_order)+1`.
- [ ] **Step 4: Run** `go test ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(api): cronologii pe locații, latență ≤ 500 puncte, ordinea locațiilor`.

### Task 3: Servirea interfeței încorporate (SPA) și cârligul Wrap

**Files:**
- Create: `internal/webui/webui.go`, `internal/webui/dist/.gitkeep` (deja ignorat cu excepție în `.gitignore`), `internal/api/spa.go`
- Modify: `internal/api/server.go`, `internal/app/app.go`
- Test: `internal/api/spa_test.go`, `internal/webui/webui_test.go`

**Interfaces:**
- Produces:
  - `func webui.FS() fs.FS` — sub-FS-ul `dist`; `nil` dacă `dist/index.html` lipsește (build fără front-end).
  - `func spaHandler(web fs.FS) http.Handler`: fișier existent → servit; `assets/*` cu `Cache-Control: public, max-age=31536000, immutable`; `index.html` cu `Cache-Control: no-cache`; cale fără extensie inexistentă (`/dispecerat`, `/locatie/7`, `/administrare/setari`) → `index.html` cu status 200; cale cu extensie inexistentă → 404; directoare nu se listează.
  - `app.Options.Wrap func(http.Handler) http.Handler` — aplicat handler-ului final (folosit doar de `tools/e2eserver`).
  - În `app.Run`: dacă `opt.Web == nil` se folosește `webui.FS()`.
- Consumers: Task 8 (e2eserver), toate paginile.

- [ ] **Step 1: Write the failing tests** `TestSPAFallback` (fstest.MapFS cu `index.html`, `assets/app-1.js`): `/` și `/dispecerat` → conținutul index, 200, `no-cache`; `/assets/app-1.js` → `immutable`; `/assets/lipsa.js` → 404; `/api/nimic` → JSON 404 neschimbat; antetul CSP prezent pe index. `TestWebuiFSNilWithoutIndex`: rulat fără build → `FS()` întoarce nil (testul acceptă și non-nil dacă `dist/index.html` există, verificând atunci că se poate deschide).
- [ ] **Step 2: Run** `go test ./internal/api ./internal/webui -run 'SPA|Webui' -v` — Expected: FAIL (lipsesc pachetul și handler-ul).
- [ ] **Step 3: Implement** cu `//go:embed all:dist`; `spaHandler` cu `http.ServeFileFS`.
- [ ] **Step 4: Run** `go test ./...` și `go vet ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): servire SPA încorporată cu fallback și cache`.

### Task 4: Proxy de dale OSM

**Files:**
- Create: `internal/tiles/tiles.go`
- Modify: `internal/api/server.go` (`Deps.Tiles http.Handler`, rută `GET /tiles/{z}/{x}/{file}`), `internal/app/app.go`
- Test: `internal/tiles/tiles_test.go`

**Interfaces:**
- Produces: `type Proxy struct{ Upstream string; MaxBytes int64; TTL time.Duration; Client *http.Client; Now func() time.Time }`; `func New(dir, version string) (*Proxy, error)` (indexează cache-ul existent); `func (p *Proxy) ServeHTTP(w, r)`. Valori: `Upstream = "https://tile.openstreetmap.org"`, `MaxBytes = 500 << 20`, `TTL = 7*24h`, 4 descărcări simultane, timeout total 5 s, `User-Agent: NetWatch/<versiune> (monitorizare rețea, instalare locală)`. Răspuns: `Content-Type: image/png`, `Cache-Control: public, max-age=86400`. Validare răspuns upstream: 200, `image/png`, ≤ 1 MB.
- `app.Options.TilesUpstream string` — suprascrie `Upstream` (folosit de e2eserver).
- Consumers: Task 12.

- [ ] **Step 1: Write the failing tests** (upstream `httptest.Server`):
  - `TestTileValidation`: `z=1`, `z=19`, `x=4` la `z=2`, `y=-1`, fișier fără `.png` → 400; `z=2,x=3,y=3` → 200.
  - `TestTileCachedOnDisk`: a doua cerere nu atinge upstream; fișierul există în `dir/2/1/1.png`.
  - `TestTileStaleServedWhenUpstreamDown`: `Now` + 8 zile, upstream întoarce 500 → 200 cu dala veche.
  - `TestTileExpiredRefetched`: `Now` + 8 zile, upstream OK → upstream apelat a doua oară.
  - `TestTileDedup`: 10 cereri concurente pentru aceeași dală → upstream apelat o dată.
  - `TestTilePoolFull503`: upstream blocat, 4 dale diferite în curs, a 5-a (fără cache) → 503 imediat (< 100 ms).
  - `TestTileTimeout`: upstream care nu răspunde → 502/504 în ≤ 5,5 s, fără cache → răspuns de eroare.
  - `TestTileEvictsLRU`: `MaxBytes` mic → după depășire, cea mai veche dală accesată este ștearsă de pe disc.
  - `TestTileUserAgent`: upstream primește `User-Agent` care începe cu `NetWatch/`.
  - `TestTileRejectsNonPNG`: upstream întoarce `text/html` → nu se salvează, răspuns 502.
- [ ] **Step 2: Run** `go test ./internal/tiles -v` — Expected: FAIL (pachet inexistent).
- [ ] **Step 3: Implement.** Indexul LRU în memorie (cheie → dimensiune, ultima accesare), reconstruit la `New` din mtime; scriere atomică (fișier temporar + rename); deduplicare cu map de apeluri în curs + `sync.WaitGroup`; semafor neblocant (`select default` → 503 sau dală expirată, dacă există).
- [ ] **Step 4: Run** `go test ./internal/tiles ./internal/api ./internal/app -v` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tiles): proxy OSM cu cache LRU pe disc, pool limitat și deduplicare`.

### Task 5: Schelet front-end: Vite, Svelte 5, teme, router, client API

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/svelte.config.js`, `web/index.html`, `web/src/main.ts`, `web/src/App.svelte`, `web/src/styles/tokens.css`, `web/src/styles/base.css`, `web/src/lib/api.ts`, `web/src/lib/types.ts`, `web/src/lib/router.svelte.ts`, `web/src/lib/theme.ts`, `web/src/components/Header.svelte`
- Test: `web/src/lib/router.test.ts`, `web/src/lib/api.test.ts`, `web/src/lib/theme.test.ts`

**Interfaces:**
- Produces:
  - Scripturi npm: `dev` (Vite cu proxy `/api`, `/tiles` spre `http://127.0.0.1:8501`), `build` (în `../internal/webui/dist`, `emptyOutDir: true`; un plugin `closeBundle` recreează `.gitkeep`; `build.modulePreload.polyfill: false`), `check` (`svelte-check`), `test` (`vitest run`), `e2e` (`playwright test`).
  - `types.ts`: `State = 'up'|'down'|'degraded'|'unknown'|'error'`, `DeviceView`, `LocationView`, `Health`, `Snapshot`, `StateResponse`, `Segment`, `Point`, `Report`, `Report24`, `Settings`, `Overrides`, `AdminLocation`, `DeviceInput`, `LocationInput`, `BackupInfo`, `AuditEntry` — câmpurile exact ca tag-urile JSON din Go (`is_primary`, `last_check`, `latency_ms`, `loss_pct`, `sort_order`, `epoch` etc.).
  - `api.ts`: `class ApiError extends Error { status: number; field?: string }`; `get<T>(path, params?)`, `send<T>(method, path, body?)` — trimite `X-CSRF-Token` din `setCsrf(token)`, `credentials: 'same-origin'`; răspuns ne-JSON sau eroare de rețea → `ApiError(0, "Serverul nu răspunde.")`.
  - `router.svelte.ts`: `route` (rună `$state` cu `{name, params, query}`), `navigate(path, {replace?})`, `match(path): Route`, interceptare click pe `<a>` interne, `beforeLeave(fn): () => void` (gardă pentru modificări nesalvate). Rute: `monitor` `/`, `location` `/locatie/:id`, `dispatch` `/dispecerat`, `map` `/harta`, `stats` `/statistici`, `report` `/raport-24h`, `admin` `/administrare`, `adminLocation` `/administrare/locatie/:id` (`id` = `nou` pentru creare), `adminPage` `/administrare/:page` (`setari|identitate|backup|audit|diagnostic|parola|despre`), `notFound`.
  - `theme.ts`: `initialTheme(stored, prefersDark, route): 'light'|'dark'` și `applyTheme(t)` → `document.documentElement.dataset.theme`.
  - `Header.svelte`: denumirea din identitate, navigare (Monitorizare, Hartă, Statistici, Raport 24H, Administrare), comutator temă, slot pentru indicatorul de conexiune.
  - Tokens CSS: variabile `--c-up`, `--c-down`, `--c-degraded`, `--c-unknown`, `--c-error`, `--bg`, `--fg`, `--muted`, `--surface`, `--border`, `--focus` pentru ambele teme, cu contrast AA față de `--bg`/`--surface`.
- Consumers: toate task-urile front-end.

- [ ] **Step 1: Scaffold** proiectul (`npm create vite@latest web -- --template svelte-ts` sau manual) și instalează `leaflet leaflet.markercluster uplot` și dev `vitest @playwright/test @axe-core/playwright svelte-check @types/leaflet @types/leaflet.markercluster`. Fixează versiunile exacte în `package.json` (fără `^`); commit și `package-lock.json`.
- [ ] **Step 2: Write the failing tests:**
  - `router.test.ts`: `match('/locatie/12')` → `{name:'location', params:{id:'12'}}`; `match('/administrare/locatie/nou')` → `adminLocation`, `id:'nou'`; `match('/administrare/setari')` → `adminPage`, `page:'setari'`; `match('/administrare/necunoscut')` → `notFound`; `match('/statistici?from=1&to=2')` → `query.from === '1'`; `match('/x/y')` → `notFound`.
  - `api.test.ts` (fetch simulat): 400 `{"error":"Mesaj","field":"name"}` → `ApiError` cu `status 400`, `field 'name'`, `message 'Mesaj'`; `send('PUT', …)` după `setCsrf('t')` trimite antetul `X-CSRF-Token: t`; fetch respins → `status 0`, mesaj „Serverul nu răspunde.”; răspuns HTML 502 → `status 502`, mesaj „Serverul nu răspunde.”.
  - `theme.test.ts`: `initialTheme(null, false, 'dispatch')` → `dark`; `initialTheme('light', true, 'dispatch')` → `light`; `initialTheme(null, true, 'monitor')` → `dark`; valoare stocată invalidă → urmează sistemul.
- [ ] **Step 3: Run** `npm test` (în `web/`) — Expected: FAIL (module inexistente).
- [ ] **Step 4: Implement** modulele și un `App.svelte` care montează `Header` și o pagină placeholder per rută.
- [ ] **Step 5: Run** `npm test && npm run check && npm run build` — Expected: teste PASS, 0 erori svelte-check, `internal/webui/dist/index.html` creat și `.gitkeep` prezent. Apoi `go build ./cmd/netwatch` — Expected: OK.
- [ ] **Step 6: Commit** `feat(web): schelet Svelte 5 cu router, teme, client API`.

### Task 6: Logică pură: timp, stări, perioade

**Files:**
- Create: `web/src/lib/time.ts`, `web/src/lib/status.ts`, `web/src/lib/periods.ts`
- Test: `web/src/lib/time.test.ts`, `web/src/lib/status.test.ts`, `web/src/lib/periods.test.ts`

**Interfaces:**
- Produces:
  - `TZ = 'Europe/Chisinau'`; `fmtDateTime(ms): string` („09.11.2026 14:05”); `fmtTime(ms, seconds?)`; `fmtDate(ms)`; `fmtDuration(seconds): string` („45 s”, „12 min”, „3 h 05 min”, „2 z 4 h”); `fmtSince(sinceMs, nowMs): string` („de 12 min”; diferențe negative → „de 0 s”); `zonedToUtc(dateStr 'YYYY-MM-DD', timeStr 'HH:MM'): number` (ora locală din Chișinău → ms); `utcToZonedParts(ms): {date, time}` (pentru inputuri); `startOfDayChisinau(ms)`.
  - `status.ts`: `STATE_LABEL`, `STATE_SYMBOL`, `stateText(s)` („✕ Indisponibilă”), `severity(s)`, `sortLocations(list, mode: 'severity'|'alpha'|'manual')` (gravitate, apoi nume cu `localeCompare('ro')`; manual după `sort_order`, apoi nume), `pendingText(d)` („cădere în confirmare” când `pending==='down'`, „revenire în confirmare” când `pending==='up'`), `totals(list): Record<State, number>`, `fmtPct(v: number|null)` („99,95%” / „—”), `fmtLatency(ms|null)` („12,3 ms” / „—”).
  - `periods.ts`: `type PresetKey = '24h'|'7z'|'30z'|'luna'|'luna-trecuta'`; `presetRange(key, nowMs): {from, to}` — luna curentă = de la 1 ale lunii, 00:00 Chișinău, până acum; luna trecută = luna calendaristică întreagă în Chișinău; `PRESET_LABEL`.
- Consumers: Task 7–17.

- [ ] **Step 1: Write the failing tests** (rulează cu `TZ=America/New_York` setat în `vitest.config`/`vite.config` `test.env`, ca să dovedească independența de fusul browserului):
  - `zonedToUtc('2026-03-29','02:30')` → ora inexistentă: se mută înainte la `03:30` local (= `2026-03-29T00:30:00Z`); `zonedToUtc('2026-10-25','03:30')` → prima apariție (`2026-10-25T00:30:00Z`); `zonedToUtc('2026-07-01','00:00')` = `Date.UTC(2026,5,30,21,0)`; `zonedToUtc('2026-01-15','12:00')` = `Date.UTC(2026,0,15,10,0)`.
  - `utcToZonedParts(Date.UTC(2026,5,30,21,0))` → `{date:'2026-07-01', time:'00:00'}`.
  - `fmtDateTime(Date.UTC(2026,10,9,12,5))` → `'09.11.2026 14:05'`.
  - `fmtDuration`: 45 → `'45 s'`; 720 → `'12 min'`; 11100 → `'3 h 05 min'`; 187200 → `'2 z 4 h'`.
  - `fmtSince(1000, 500)` → `'de 0 s'`.
  - `sortLocations` cu `[up 'Bălți', down 'Cahul', unknown 'Anenii', degraded 'Orhei', error 'Edineț']`, mod `severity` → Cahul, Orhei, Anenii, Edineț, Bălți; mod `alpha` → Anenii, Bălți, Cahul, Edineț, Orhei.
  - `stateText('down')` → `'✕ Indisponibilă'`; `fmtPct(99.954)` → `'99,95%'`; `fmtPct(null)` → `'—'`.
  - `presetRange('luna-trecuta', Date.UTC(2026,2,15,12))` → `from = zonedToUtc('2026-02-01','00:00')`, `to = zonedToUtc('2026-03-01','00:00')`; `presetRange('24h', n)` → `{from: n-86400000, to: n}`.
- [ ] **Step 2: Run** `npm test` — Expected: FAIL.
- [ ] **Step 3: Implement** cu `Intl.DateTimeFormat('ro-RO', {timeZone: TZ, …})`; `zonedToUtc` prin iterație de decalaj (ghicește UTC, corectează cu decalajul zonei de 2 ori, alege prima soluție validă, iar pentru gol mută înainte).
- [ ] **Step 4: Run** `npm test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): formatare timp Europe/Chisinau, stări și perioade`.

### Task 7: Store live, anunțuri și sunet

**Files:**
- Create: `web/src/lib/live.svelte.ts`, `web/src/lib/sound.ts`, `web/src/components/ConnectionBadge.svelte`, `web/src/components/Announcer.svelte`, `web/src/components/SoundToggle.svelte`
- Test: `web/src/lib/live.test.ts`, `web/src/lib/sound.test.ts`

**Interfaces:**
- Consumes: Task 1 (formatul SSE), Task 6.
- Produces:
  - `class LiveStore` (instanță unică `live`): `$state` `locations: LocationView[]`, `health`, `identity`, `intervalS`, `epoch`, `connected: boolean`, `stale: boolean`, `lastMessageAt`, `clockOffset` (ms server − ms local). Metode: `start()`, `stop()`, `serverNow(): number`, `location(id)`, `onChange(fn: (prev: LocationView|undefined, next: LocationView|undefined) => void): () => void`.
  - Pornire: `GET /api/state` (identitate, interval, `now` → `clockOffset`), apoi `EventSource('/api/events?last=…')`. Evenimente: `snapshot` (înlocuiește tot; locațiile absente dispar), `state` (upsert), `removed` (șterge), `health`, `ping` (`clockOffset` din `now`). Orice mesaj actualizează `lastMessageAt`. La `error` cu `readyState === CLOSED` → reconectare în 2 s, 4 s, 8 s … maxim 30 s, cu `?last=<ultimul id>`. Un temporizator la 1 s setează `stale = serverNow() - lastMessageAt > 30000`.
  - Funcția pură `applyEvent(state: {locations, epoch, lastId}, type, data, id): {…}` testată separat.
  - `healthWarning(health, intervalS, serverNow): string | ''` — `health.warning` dacă există; altfel, dacă `last_cycle` lipsește sau e mai vechi de `2 × intervalS`, „Monitorul nu a scris date recent.”.
  - `sound.ts`: `type SoundPrefs = {on: boolean; degraded: boolean}` în `localStorage["nw-sound"]`; funcția pură `soundFor(prev?: State, next?: State, prefs): 'alarm'|'recovery'|null` — `alarm` la trecerea în `down` dintr-o altă stare (și la `degraded` dacă `prefs.degraded`); `recovery` la trecerea din `down` în `up`; locație nouă direct `down` → `alarm`; `null` când `prefs.on` e fals. `class Beeper { play(kind); unlock(); blocked: boolean }` cu Web Audio (oscilator: alarmă 3 tonuri de 880 Hz, revenire 2 tonuri ascendente 523/784 Hz).
  - `SoundToggle.svelte`: buton „Sunet: pornit/oprit”; dacă `blocked` → textul „Click pentru a activa sunetul”; opțiunea „și la degradare”.
  - `ConnectionBadge.svelte`: „Conectat” / „Reconectare…” / „Date neactualizate” (`role="status"`).
  - `App.svelte` montează `ConnectionBadge` în `Header` (toate paginile, inclusiv administrarea) și un singur `Announcer`.
  - `Announcer.svelte`: regiune `aria-live="polite"` vizual ascunsă, cu anunțuri „Cahul: ✕ Indisponibilă” la schimbarea stării unei locații.
- Consumers: Task 9–14.

- [ ] **Step 1: Write the failing tests:**
  - `live.test.ts`: `applyEvent` cu `snapshot` care nu conține locația 3 → 3 dispare; `state` pentru id nou → adăugată; `removed` → ștearsă; snapshot cu epocă nouă → `epoch` actualizat și lista înlocuită; `lastId` = id-ul evenimentului. Un `FakeEventSource` injectat: după `error` cu `CLOSED`, reconectarea folosește URL-ul cu `?last=<epoch>-<seq>` al ultimului eveniment; fără mesaje 31 s (temporizatoare simulate) → `stale === true`; un `ping` → `stale === false`. `clockOffset`: `now` server = local + 60000 → `fmtSince(serverNow()-60000, serverNow())` = „de 1 min”. `healthWarning` cu `last_cycle` vechi de 3 intervale → mesajul; cu `warning: 'X'` → `'X'`.
  - `sound.test.ts`: `soundFor('up','down',on)` → `alarm`; `soundFor('down','up',on)` → `recovery`; `soundFor('up','degraded',{on:true,degraded:false})` → `null`, cu `degraded:true` → `alarm`; `soundFor(undefined,'down',on)` → `alarm`; `soundFor('down','down',on)` → `null`; `soundFor('unknown','up',on)` → `null`; `prefs.on=false` → `null`.
- [ ] **Step 2: Run** `npm test` — Expected: FAIL.
- [ ] **Step 3: Implement** (constructorul `LiveStore` primește `EventSource`, `fetch`, `setTimeout`, `now` injectabile; implicit cele globale).
- [ ] **Step 4: Run** `npm test && npm run check` — Expected: PASS, 0 erori.
- [ ] **Step 5: Commit** `feat(web): store live SSE cu reconectare, date neactualizate, sunet`.

### Task 8: Harnașament E2E: server cu pinger simulat și Playwright

**Files:**
- Create: `tools/e2eserver/main.go`, `web/playwright.config.ts`, `web/e2e/fixtures.ts`, `web/e2e/smoke.spec.ts`
- Test: `web/e2e/smoke.spec.ts`

**Interfaces:**
- Consumes: Task 3 (`Options.Wrap`, `Web`), Task 4 (`Options.TilesUpstream`).
- Produces:
  - `go run ./tools/e2eserver --listen 127.0.0.1:18501 --control 127.0.0.1:18502 [--no-password] [--web internal/webui/dist]`: director de date temporar nou la fiecare pornire; parola `parola-e2e-12345` (fără `--no-password`); `IntervalScale = 1.0/60`, `FlushEvery = 50ms`; upstream de dale `http://127.0.0.1:18502/osm` (controlabil); servește `--web` prin `os.DirFS`.
  - Date inițiale: „Chișinău Centru” (Router `10.0.0.1` ★, Switch `10.0.0.2`), „Cahul” (Router `10.0.1.1` ★), „Bălți” (Router `10.0.2.1` ★, AP `10.0.2.2`), toate cu coordonate reale; toate host-urile răspund cu 5 ms.
  - Port de control (JSON): `POST /host {"host":"10.0.1.1","result":"ok"|"timeout"|"permission","latency_ms":5}`; `POST /reset` (toate `ok`, SSE pornit, dale OK); `POST /sse {"on":false}` (închide fluxurile SSE active și răspunde 503 la cele noi, până la `on:true`); `POST /tiles {"ok":false}` (upstream-ul `/osm/...` răspunde 500); `GET /osm/{z}/{x}/{y}.png` (PNG 256×256 gri, generat).
  - `playwright.config.ts`: `workers: 1`, `webServer` = `[{command: 'go run ./tools/e2eserver --listen 127.0.0.1:18501 --control 127.0.0.1:18502', cwd: '..', url: 'http://127.0.0.1:18501/api/state'}, {… --no-password pe 18511/18512}]`, `use.baseURL` `http://127.0.0.1:18501`, Chromium; scriptul `e2e` rulează întâi `npm run build`.
  - `fixtures.ts`: fixture `page` extins care (a) colectează `console` de tip `error` și evenimentele `securitypolicyviolation` (prin `addInitScript`) și eșuează testul dacă există; (b) apelează `POST /reset` înainte de fiecare test; helperi `control(path, body)`, `setHost(host, result)`, `login(page)`, `adminApi(request)` (login + CSRF pentru crearea de date prin API), `waitState(page, locationName, stateText)`.
- Consumers: Task 9–18.

- [ ] **Step 1: Write the failing test** `smoke.spec.ts`: `/` se încarcă, titlul paginii conține „NetWatch”, antetul conține „Monitorizare”; fără erori de consolă/CSP (prin fixture); `/dispecerat` și `/administrare` răspund 200 cu aplicația montată.
- [ ] **Step 2: Run** `npm run e2e -- smoke` — Expected: FAIL (e2eserver inexistent).
- [ ] **Step 3: Implement** `tools/e2eserver` (folosește `icmp.Fake` cu o hartă protejată de mutex, `app.Run` cu `Options{Pinger, Lookup: rezolvare directă IP, Web: os.DirFS(...), Wrap: …, TilesUpstream: …}`); Wrap-ul urmărește contextele cererilor `/api/events` ca să le poată închide.
- [ ] **Step 4: Run** `npm run e2e -- smoke` — Expected: PASS. Apoi `go vet ./...` — Expected: OK.
- [ ] **Step 5: Commit** `test(e2e): server cu pinger simulat controlabil și Playwright`.

### Task 9: Pagina Monitorizare

**Files:**
- Create: `web/src/pages/Monitor.svelte`, `web/src/components/LocationCard.svelte`, `web/src/components/StatusBadge.svelte`, `web/src/components/MiniTimeline.svelte`, `web/src/components/TopBar.svelte`, `web/src/lib/timelines.svelte.ts`
- Test: `web/e2e/monitor.spec.ts`

**Interfaces:**
- Consumes: Task 2 (`/api/timelines`, `sort_order`), Task 6, Task 7.
- Produces:
  - `TopBar`: totaluri pe stări (butoane comutabile care filtrează grila, `aria-pressed`), căutare după nume locație/echipament/IP (cu `/` ca scurtătură de focus), avertismentul `healthWarning`, `SoundToggle`, `ConnectionBadge`.
  - Grilă de `LocationCard` (`<a href="/locatie/{id}">`), sortare `severity|alpha|manual` reținută în `localStorage["nw-sort"]`. Card: nume, `StatusBadge`, „de 12 min” (actualizat la 1 s pe `serverNow`), marcaj „date incomplete” dacă `incomplete`, echipamente (★ pentru primare, stare, `pendingText`, latență), `MiniTimeline` pe 24 h. Puls CSS (clasa `pulse`, 3 s) când locația trece într-o stare mai gravă. Când `live.stale`, cardurile primesc clasa `stale` (culoare gri + eticheta „neactualizat”).
  - `timelines.svelte.ts`: încarcă `/api/timelines` (24 h) la pornire, la 60 s și la 5 s după o schimbare de stare (debounce); `MiniTimeline` desenează segmentele ca `<div>`-uri poziționate prin `style:left`/`style:width` în procente, cu `title` „Funcțională · 09.11 14:05–14:30”.
  - Sunetul și anunțurile sunt legate prin `live.onChange` în `App.svelte` (o singură dată, pentru toate paginile care afișează `SoundToggle`).
- [ ] **Step 1: Write the failing tests** `monitor.spec.ts`:
  - `cardurile apar sortate după gravitate`: `setHost('10.0.1.1','timeout')` → în ≤ 15 s cardul „Cahul” afișează „✕ Indisponibilă” și este primul în grilă.
  - `confirmarea este afișată`: după primul eșec, înainte de confirmare, echipamentul afișează „cădere în confirmare” (se verifică textul apărut cel puțin o dată, prin `expect.poll`).
  - `secundar căzut → degradată`: `setHost('10.0.2.2','timeout')` → „Bălți” afișează „▲ Degradată”.
  - `eroare monitor`: `setHost('10.0.0.1','permission')` → „Chișinău Centru” are un echipament „! Eroare monitor”.
  - `filtrul pe stări`: click pe totalul „Indisponibile” → doar Cahul vizibil; al doilea click → toate.
  - `căutare după IP`: tastează `10.0.2.2` → doar Bălți vizibil.
  - `nume cu HTML afișat literal`: creează prin `adminApi` locația `<img src=x onerror=window.__xss=1>Ștefan Vodă` → textul apare literal în card; `window.__xss` nedefinit.
  - `anunț aria-live`: după căderea Cahul, regiunea `[aria-live=polite]` conține „Cahul: ✕ Indisponibilă”.
  - `navigare din tastatură`: `Tab` ajunge la un card; `Enter` deschide `/locatie/{id}`.
- [ ] **Step 2: Run** `npm run e2e -- monitor` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run e2e -- monitor && npm test && npm run check` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): pagina Monitorizare cu carduri live, filtre și cronologii`.

### Task 10: Panoul de detalii al locației

**Files:**
- Create: `web/src/pages/LocationPanel.svelte`, `web/src/components/Timeline.svelte`, `web/src/components/LatencyChart.svelte`, `web/src/components/IncidentList.svelte`
- Test: `web/e2e/panel.spec.ts`

**Interfaces:**
- Consumes: Task 2 (`/api/locations/{id}/timeline`), `/api/devices/{id}/timeline`, `/api/devices/{id}/latency`, `/api/stats?location=` (incidente), Task 7.
- Produces: panou lateral (`role="dialog"`, `aria-labelledby`, focus mutat în panou, `Esc` și butonul „Închide” revin la `/` cu focus pe cardul de origine) suprapus peste Monitorizare la ruta `/locatie/:id` (URL partajabil: deschiderea directă afișează Monitorizarea cu panoul deschis). Conținut: nume, adresă, observații, stare; echipamente cu stare, `pendingText`, cod de eroare tradus, avertisment DNS, latență și pierderi curente, ultima verificare; selector de perioadă `24 h | 7 z | 30 z` pentru cronologia locației și a fiecărui echipament; `LatencyChart` (uPlot: medie, bandă min/max, pierderi pe axă secundară, ora Chișinău pe axa X, temă din tokens) pentru echipamentul selectat; incidentele locației din perioadă (`IncidentList`: echipament, început observat, confirmat, sfârșit, durată, motiv `reason_text`). Actualizare live: stările vin din `live`; cronologia și incidentele se reîncarcă la o schimbare de stare a locației (debounce 5 s). Locație inexistentă → mesajul „Locația nu există sau a fost arhivată.”.
  - Codurile de eroare traduse: `timeout` „Fără răspuns”, `host_unreachable` „Echipament inaccesibil”, `net_unreachable` „Rețea/rută inaccesibilă”, `dns` „Eroare DNS”, `permission` „Lipsă permisiune ICMP”, `internal` „Eroare internă a monitorului” (în `status.ts`, cu test Vitest).
- [ ] **Step 1: Write the failing tests** `panel.spec.ts`:
  - `deschidere din card și URL`: click pe „Cahul” → URL `/locatie/<id>`, dialogul conține „Router” și „10.0.1.1”; reîncărcarea paginii păstrează panoul deschis.
  - `actualizare live`: cu panoul deschis, `setHost('10.0.1.1','timeout')` → panoul afișează „✕ Indisponibilă” fără reîncărcare; după revenire (`ok`), lista de incidente conține un rând cu motivul „Revenire”.
  - `perioadă 7 z`: click pe „7 z” → cererea de cronologie are `to - from = 7 zile` (verificat prin `page.waitForRequest`).
  - `grafic de latență`: există un `canvas` uPlot în panou.
  - `Esc închide`: `Escape` → URL `/`, focus pe cardul „Cahul”.
  - `locație inexistentă`: `/locatie/999999` → mesajul „Locația nu există sau a fost arhivată.”.
- [ ] **Step 2: Run** `npm run e2e -- panel` — Expected: FAIL.
- [ ] **Step 3: Implement** (uPlot importat leneș în `LatencyChart`).
- [ ] **Step 4: Run** `npm run e2e -- panel && npm test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): panou de detalii cu cronologii, latență și incidente`.

### Task 11: Dispecerat

**Files:**
- Create: `web/src/pages/Dispatch.svelte`, `web/src/lib/grid.ts`
- Test: `web/src/lib/grid.test.ts`, `web/e2e/dispatch.spec.ts`

**Interfaces:**
- Consumes: Task 7, Task 9 (`LocationCard` în varianta `size="large"`), Task 12 (`MapView` pentru rotație — Task 11 implementează rotația cu un placeholder dacă harta nu există încă; Task 12 o conectează).
- Produces:
  - `/dispecerat`: fără antet/meniu; ceas mare (ora Chișinău, secunde), `ConnectionBadge`, `SoundToggle` (doar dacă `?sunet=1`), bandă superioară cu locațiile cu probleme, sortate după gravitate, în format „Cahul · Router · de 12 min” (primul echipament primar căzut; pentru `degraded` primul secundar căzut). Grilă auto-dimensionată fără derulare: `computeGrid(n, width, height, minAspect = 1.6): {cols, rows, fontPx}` cu `style:--cols` / `style:--font`. La `live.stale`: chenar gri pe toată pagina și textul „DATE NEACTUALIZATE”. Opțiuni URL: `?rotatie=N` (N ≥ 10 s; alternează grilă/hartă), `?sunet=1`, `?tema=luminoasa`. Buton ecran complet (Fullscreen API) vizibil la focus/hover.
- [ ] **Step 1: Write the failing tests:**
  - `grid.test.ts`: `computeGrid(3,1920,1080)` → `cols*rows >= 3`; `computeGrid(60,1920,1080)` → `cols*rows >= 60`, `cols >= rows`; `computeGrid(0, …)` → `{cols:1, rows:1}`; `fontPx` scade când `n` crește și nu scade sub 12.
  - `dispatch.spec.ts`:
    - `bandă cu probleme`: `setHost('10.0.1.1','timeout')` → banda conține „Cahul · Router · de”.
    - `pierderea conexiunii`: `page.clock.install()`, deschide `/dispecerat`, așteaptă „Conectat”; `control('/sse',{on:false})`; `page.clock.fastForward(31000)` → vizibil „DATE NEACTUALIZATE”; `control('/sse',{on:true})`, `fastForward(35000)` → textul dispare.
    - `60 de locații încap fără derulare`: creează 57 de locații prin `adminApi` (plus cele 3 inițiale); viewport 1920×1080 → `scrollHeight <= innerHeight` și `scrollWidth <= innerWidth`; toate cele 60 de carduri vizibile (`toBeInViewport`). Locațiile create se arhivează la final.
    - `temă întunecată implicit`: `html[data-theme=dark]`; cu `?tema=luminoasa` → `light`.
    - `rotație`: `?rotatie=10` cu `page.clock` → după 10 s este vizibilă harta (`.leaflet-container`), după încă 10 s din nou grila. (Acest test se adaugă în Task 12, când harta există.)
- [ ] **Step 2: Run** `npm test && npm run e2e -- dispatch` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm test && npm run e2e -- dispatch` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): ecran de dispecerat cu grilă auto-dimensionată și detecția conexiunii pierdute`.

### Task 12: Hartă

**Files:**
- Create: `web/src/pages/MapPage.svelte`, `web/src/components/MapView.svelte`, `web/src/lib/tilehealth.ts`, `web/src/assets/contours.json`, `web/scripts/contours.mjs`
- Modify: `web/src/pages/Dispatch.svelte` (rotația folosește `MapView`)
- Test: `web/src/lib/tilehealth.test.ts`, `web/e2e/map.spec.ts`, `web/e2e/dispatch.spec.ts` (testul de rotație)

**Interfaces:**
- Consumes: Task 4 (`/tiles/{z}/{x}/{y}.png`), Task 7.
- Produces:
  - `MapView.svelte` (Leaflet încărcat leneș, CSS-ul Leaflet și markercluster importate ca fișiere): strat de dale `/tiles/{z}/{x}/{y}.png` (`minZoom 2`, `maxZoom 18`, atribuire „© OpenStreetMap”), marcaje `L.divIcon` cu clasă de stare și simbol (construite prin DOM, cu `textContent`; fără HTML cu atribute `style`), grupare `markerClusterGroup` la zoom mic (culoarea clusterului = cea mai gravă stare din grup), popup construit prin DOM (nume ca `textContent`, stare, echipamente căzute, legătura „Detalii” → `/locatie/{id}`), încadrare automată pe locațiile cu coordonate; locațiile fără coordonate sunt listate sub hartă („Fără coordonate: …”). Marcajele se actualizează din `live` fără a recrea harta.
  - `tilehealth.ts`: `class TileHealth { record(ok: boolean, now): void; shouldFallback(now): boolean; }` — fereastră ultimele 20 de evenimente în 30 s; fallback când > 50% eșuate și cel puțin 6 eșecuri. În fallback: stratul de dale ascuns, stratul GeoJSON `contours.json` afișat, reîncercare a stratului de dale la 60 s (o dală de probă); la succes revenire automată. O dală eșuată se reîncearcă o dată după 2 s (înainte de a fi numărată ca eșec).
  - `scripts/contours.mjs`: descarcă `ne_50m_admin_0_countries.geojson` (Natural Earth, domeniu public) și păstrează țările din caseta lon 20–35, lat 43–50 (Moldova și vecinii), cu coordonate rotunjite la 3 zecimale; rezultatul este comis în `src/assets/contours.json` (≤ 300 KB).
  - `/harta`: antet normal + `MapView` pe tot spațiul rămas.
- [ ] **Step 1: Write the failing tests:**
  - `tilehealth.test.ts`: 10 succese → `false`; 6 eșecuri din 10 → `true`; 3 eșecuri din 4 → `false` (sub minimul de 6); evenimentele mai vechi de 30 s nu se mai numără.
  - `map.spec.ts`:
    - `marcaje colorate`: `/harta` → 3 marcaje (sau clustere care le conțin); după `setHost('10.0.1.1','timeout')`, marcajul Cahul are clasa `state-down`.
    - `popup cu legătură`: click pe marcajul Cahul → popup cu „Cahul” și legătura „Detalii” spre `/locatie/<id>`.
    - `popup cu nume HTML literal`: locația `<img src=x onerror=window.__xss=1>Ștefan Vodă` cu coordonate → textul literal în popup, `window.__xss` nedefinit.
    - `fallback contururi`: `control('/tiles',{ok:false})` înainte de încărcare → în ≤ 10 s este vizibil stratul GeoJSON (`path.contour`); `control('/tiles',{ok:true})` + `page.clock.fastForward(61000)` → stratul de dale revine.
  - `dispatch.spec.ts` › `rotație` (vezi Task 11).
- [ ] **Step 2: Run** `npm test && npm run e2e -- map dispatch` — Expected: FAIL.
- [ ] **Step 3: Implement.** Rulează `node scripts/contours.mjs` o singură dată și comite rezultatul.
- [ ] **Step 4: Run** `npm test && npm run e2e -- map dispatch` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): hartă cu dale prin proxy, grupare și contururi locale de rezervă`.

### Task 13: Statistici

**Files:**
- Create: `web/src/pages/Stats.svelte`, `web/src/components/PeriodPicker.svelte`, `web/src/components/SortableTable.svelte`, `web/src/components/StateBar.svelte`, `web/src/lib/statsquery.ts`
- Test: `web/src/lib/statsquery.test.ts`, `web/e2e/stats.spec.ts`

**Interfaces:**
- Consumes: `/api/stats`, `/api/catalog`, Task 6.
- Produces:
  - `statsquery.ts`: `type StatsFilter = {preset: PresetKey|'custom'; from?: number; to?: number; location?: number; device?: number; kind?: string; incidents: 'all'|'active'|'closed'}`; `filterFromQuery(q: URLSearchParams, now): StatsFilter` (valori invalide → implicite: `24h`, `all`); `filterToQuery(f): URLSearchParams` (doar câmpurile nevide; `preset` sau `from`/`to`); `apiParams(f, now): Record<string,string>`.
  - Pagina: filtrele sincronizate cu URL-ul (`navigate(…, {replace:true})`); `PeriodPicker` cu perioadele rapide și „Personalizată” (dată + oră pentru început și sfârșit, interpretate în Chișinău prin `zonedToUtc`, validare locală „Sfârșitul trebuie să fie după început.”); filtre locație/echipament (din catalog, arhivatele marcate „(arhivat)”), tip, incidente. Indicatori: disponibilitate, acoperire, număr incidente, timp indisponibil cumulat, MTTR. Tabel sortabil pe echipamente (click/`Enter` pe antet, `aria-sort`): locație, echipament, IP, tip, ★, disponibilitate, acoperire, latență, pierderi, incidente, MTTR. Bare de stare pe locații (`StateBar`: proporții `up/degraded/down/unknown/error` prin `style:width`, text alternativ cu procentele). Listă incidente. Antet „Perioada: 01.11.2026 00:00 – 09.11.2026 14:05 · Ora: Europe/Chisinau”. Erorile API afișate lângă câmpul indicat de `field`.
- [ ] **Step 1: Write the failing tests:**
  - `statsquery.test.ts`: `filterFromQuery('preset=7z&location=3')` → `{preset:'7z', location:3, incidents:'all'}`; `filterFromQuery('preset=xyz&location=-1&incidents=abc')` → implicite; `filterToQuery(filterFromQuery(q))` este idempotent; `apiParams({preset:'custom', from:1, to:2, …})` → `{from:'1', to:'2', …}`.
  - `stats.spec.ts`:
    - `indicatori și tabel`: după o cădere și o revenire a Cahul, `/statistici?preset=24h` → tabelul conține Cahul cu incidente ≥ 1; numărul total de incidente ≥ 1; disponibilitatea afișată în format „xx,xx%”.
    - `filtre în URL`: selectează locația Cahul → URL conține `location=<id>`; reîncărcarea păstrează filtrul și tabelul are doar echipamentele Cahul.
    - `sortare`: click pe antetul „Disponibilitate” → `aria-sort="ascending"`, apoi „descending”.
    - `perioadă personalizată invalidă`: sfârșit înainte de început → mesajul „Sfârșitul trebuie să fie după început.” și nicio cerere trimisă.
    - `fus orar afișat`: textul „Ora: Europe/Chisinau” este vizibil.
- [ ] **Step 2: Run** `npm test && npm run e2e -- stats` — Expected: FAIL.
- [ ] **Step 3: Implement** (pagina importată leneș).
- [ ] **Step 4: Run** `npm test && npm run e2e -- stats` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): statistici cu filtre în URL, indicatori, tabel sortabil și incidente`.

### Task 14: Raport 24H

**Files:**
- Create: `web/src/pages/Report24.svelte`, `web/src/styles/print.css`
- Test: `web/e2e/report.spec.ts`

**Interfaces:**
- Consumes: `/api/report24`, Task 7, `IncidentList` (Task 10).
- Produces: titlul raportului din identitate (`report_title`), perioada și „Ora: Europe/Chisinau”, rezumat („Locații monitorizate: N · Afectate acum: M”), tabelul „Situația curentă” (locație, echipament primar, indisponibil de la, durată), tabelul întreruperilor din ultimele 24 h; reîncărcare automată la o schimbare de stare a oricărei locații (debounce 3 s) și la 60 s; `SoundToggle`; buton „Tipărește” (`window.print()`). `print.css` (`@media print`, `@page { size: A4; margin: 12mm }`): ascunde navigarea și butoanele, fără fundaluri închise, rândurile tabelelor nu se rup între pagini.
- [ ] **Step 1: Write the failing tests** `report.spec.ts`:
  - `situația curentă live`: `/raport-24h` → „Afectate acum: 0”; `setHost('10.0.1.1','timeout')` → în ≤ 20 s tabelul „Situația curentă” conține „Cahul” și „Afectate acum: 1”, fără reîncărcarea paginii.
  - `întreruperi`: după revenire, tabelul întreruperilor conține un rând Cahul cu motivul „Revenire”.
  - `tipărire`: `page.emulateMedia({media:'print'})` → antetul de navigare nu este vizibil; tabelul este vizibil.
- [ ] **Step 2: Run** `npm run e2e -- report` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run e2e -- report` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): raport 24H actualizat live, cu stil de tipărire A4`.

### Task 15: Administrare: sesiune, prima configurare, autentificare

**Files:**
- Create: `web/src/pages/admin/AdminShell.svelte`, `web/src/pages/admin/Login.svelte`, `web/src/pages/admin/Setup.svelte`, `web/src/lib/session.svelte.ts`
- Test: `web/e2e/auth.spec.ts`

**Interfaces:**
- Consumes: `GET /api/admin/session` (`authenticated`, `configured`, `setup_allowed`, `tls`, `csrf`), `POST /api/admin/setup|login|logout`.
- Produces:
  - `session.svelte.ts`: `session` (`$state` cu câmpurile de mai sus), `refresh()`, `login(password)`, `setup(password, repeat)`, `logout()`; la orice `ApiError` 401 dintr-o cerere admin → `session.authenticated = false` și formularul de login (cu mesajul „Sesiunea a expirat. Autentificați-vă din nou.”); `setCsrf` apelat la fiecare `csrf` primit.
  - `AdminShell`: meniu lateral (Locații, Setări monitorizare, Identitate, Backup-uri, Jurnal audit, Diagnostic, Parolă, Despre, Ieșire) și conținutul subpaginii; neautentificat → `Login`; neconfigurat și `setup_allowed` → `Setup` (parolă + repetare, 12–200 caractere, validare locală); neconfigurat și fără `setup_allowed` → mesajul „Parola nu este configurată. Rulați pe server: netwatch set-password”.
  - Fără TLS (`tls === false`), formularul de login afișează: „Conexiune necriptată: parola se transmite în clar în rețea. Porniți serverul cu --tls auto.”.
  - Login: 401 → „Parolă incorectă…” (mesajul serverului); 429/503 → mesajul serverului; butonul este dezactivat cât durează cererea.
- [ ] **Step 1: Write the failing tests** `auth.spec.ts`:
  - `login greșit apoi corect`: parola `gresit-gresit` → mesajul serverului vizibil; `parola-e2e-12345` → meniul „Locații” vizibil.
  - `avertisment fără TLS`: textul „Conexiune necriptată” vizibil pe formularul de login.
  - `ieșire`: click „Ieșire” → formularul de login; `GET /api/admin/locations` din pagină → 401.
  - `sesiune expirată`: după login, șterge cookie-ul `nw_session` din context și execută o acțiune → formularul de login cu „Sesiunea a expirat.”.
  - `prima configurare` (pe serverul `--no-password`, `baseURL` `http://127.0.0.1:18511`): `/administrare` afișează formularul de setare; parole diferite → „Parolele nu coincid.”; parolă de 5 caractere → mesaj de lungime; parolă validă → meniul admin.
- [ ] **Step 2: Run** `npm run e2e -- auth` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm run e2e -- auth` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): administrare — sesiune, prima configurare, autentificare`.

### Task 16: Administrare: locații și echipamente

**Files:**
- Create: `web/src/pages/admin/Locations.svelte`, `web/src/pages/admin/LocationEditor.svelte`, `web/src/components/DeviceRow.svelte`, `web/src/components/CoordPicker.svelte`, `web/src/lib/editor.ts`
- Test: `web/src/lib/editor.test.ts`, `web/e2e/admin-locations.spec.ts`

**Interfaces:**
- Consumes: `GET/POST /api/admin/locations`, `PUT/DELETE /api/admin/locations/{id}`, `POST /api/admin/devices/test`, `PUT /api/admin/locations/order` (Task 2), `GET /api/admin/settings` (valorile implicite pentru suprascrieri), `MapView`/Leaflet (Task 12).
- Produces:
  - `editor.ts`: `toInput(loc: AdminLocation): LocationInput`, `emptyLocation(): LocationInput` (un echipament gol, primar), `isDirty(a, b): boolean` (comparare structurală după normalizarea spațiilor), `moveDevice(list, from, to)`, `fieldPath(field: string): {device?: number; name: string}` (`devices.2.host` → `{device:2, name:'host'}`), `parseCoord(text): number|null` (acceptă virgulă zecimală).
  - `Locations.svelte` (`/administrare`): listă cu căutare, stare live, număr de echipamente, „Adaugă locație”; reordonare prin tragere și cu butoane ↑/↓ (accesibile din tastatură), salvată cu `PUT …/order`.
  - `LocationEditor.svelte` (`/administrare/locatie/:id|nou`): nume, adresă, observații; coordonate (două câmpuri + `CoordPicker`: hartă mică; click setează coordonatele, marcajul se poate trage); rânduri `DeviceRow` (nume, host, tip cu `datalist` din tipurile existente, ★ primar, suprascrieri opționale interval/eșecuri/reveniri cu valorile globale ca placeholder, „Testează acum” → rezultat lângă rând „✓ Răspunde · 4,1 ms” sau mesajul serverului, „Șterge rând”); reordonare prin tragere și ↑/↓; „Salvează”, „Renunță”, „Arhivează” (cu confirmare: „Locația și istoricul ei rămân în statistici, dar monitorizarea se oprește.”). Erorile de la server marchează câmpul indicat de `field` (`aria-invalid`, mesaj sub câmp) și focalizează primul câmp invalid. Modificări nesalvate: `beforeLeave` pentru navigarea internă (dialog „Aveți modificări nesalvate. Le abandonați?”) și `beforeunload`.
- [ ] **Step 1: Write the failing tests:**
  - `editor.test.ts`: `fieldPath('devices.2.host')` → `{device:2, name:'host'}`; `fieldPath('name')` → `{name:'name'}`; `moveDevice([a,b,c],0,2)` → `[b,c,a]`; `isDirty(x, structuredClone(x))` → `false`; după schimbarea unui host → `true`; `parseCoord('46,99')` → `46.99`; `parseCoord('abc')` → `null`.
  - `admin-locations.spec.ts`:
    - `creare cu Testează acum`: „Adaugă locație” → nume „Orhei E2E”, echipament „Router” `10.0.5.1` ★; „Testează acum” → „✓ Răspunde”; `setHost('10.0.5.2','timeout')`, al doilea rând `10.0.5.2` → „Testează acum” afișează „Fără răspuns în timpul stabilit.”; „Salvează” → locația apare pe `/` în ≤ 10 s.
    - `eroare pe câmp`: host `999.1.1.1` → „Salvează” → mesajul serverului sub câmpul host al rândului respectiv, `aria-invalid="true"`.
    - `fără primar`: debifează ★ → mesajul serverului afișat, nimic salvat.
    - `modificări nesalvate`: schimbă numele, click pe „Monitorizare” în meniu → dialogul „Aveți modificări nesalvate”; „Rămân” → URL neschimbat.
    - `reordonare echipamente`: ↓ pe primul rând, salvează, redeschide → ordinea inversată.
    - `arhivare`: arhivează „Orhei E2E” → dispare din `/` și din lista admin.
    - `coordonate din hartă`: click pe harta editorului → câmpurile lat/lon se completează.
- [ ] **Step 2: Run** `npm test && npm run e2e -- admin-locations` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm test && npm run e2e -- admin-locations` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): administrare locații și echipamente cu test imediat și reordonare`.

### Task 17: Administrare: setări, identitate, backup, audit, diagnostic, parolă, despre

**Files:**
- Create: `web/src/pages/admin/Settings.svelte`, `Identity.svelte`, `Backups.svelte`, `Audit.svelte`, `Diagnostic.svelte`, `Password.svelte`, `About.svelte` (toate în `web/src/pages/admin/`), `web/src/lib/format.ts` (dimensiuni: `fmtBytes`)
- Test: `web/src/lib/format.test.ts`, `web/e2e/admin-pages.spec.ts`

**Interfaces:**
- Consumes: `GET/PUT /api/admin/settings`, `GET/PUT /api/admin/identity`, `GET/POST /api/admin/backups`, `GET /api/admin/backups/{name}`, `GET /api/admin/audit?limit&before`, `GET /api/admin/diagnostic`, `PUT /api/admin/password`.
- Produces:
  - Setări monitorizare: toate câmpurile `Settings` cu etichete în română și unități („Interval între verificări (s)”, „Pachete per verificare”, „Timeout per pachet (s)”, „Eșecuri pentru confirmarea căderii”, „Reușite pentru confirmarea revenirii”, „Expirarea datelor (s)”, „Incidente pentru: toate echipamentele / doar primare”, „Preferă IPv4”, „Ora backup-ului (0–23)”, „Al doilea director de backup (opțional)”); erori pe câmp; mesaj „Setări salvate.”.
  - Identitate: denumire, subtitlu, titlul raportului (logo în Planul 3); antetul aplicației se actualizează după salvare.
  - Backup-uri: tabel (nume, tip `auto|manual|pre-migrate|pre-restore` tradus, data, dimensiune), „Creează backup acum”, descărcare prin legătură; ultima stare (`health.last_backup`, `backup_error`) afișată sus; instrucțiunea de restaurare: „Restaurarea se face pe server: netwatch restore FIȘIER”.
  - Jurnal audit: tabel (dată, acțiune tradusă, entitate, detalii JSON formatat pe rând expandabil), paginare „Mai vechi” cu `before = ultimul id`, 50 pe pagină.
  - Diagnostic: ultimul ciclu, verificări întârziate, durata medie, modul ICMP tradus (`windows` „Windows (IcmpSendEcho)”, `unprivileged` „Linux neprivilegiat”, `raw` „Linux raw socket”, `unavailable` „Indisponibil — lipsă permisiune”, `fake` „Simulat”), ultimele erori, dimensiunea bazei (`fmtBytes`), directorul de date, versiunea, starea ultimului backup, `healthWarning`; reîmprospătare la 10 s.
  - Parolă: actuală, nouă, repetare; după succes → mesaj „Parola a fost schimbată. Autentificați-vă din nou.” și formularul de login.
  - Despre: versiunea, licențele componentelor (Leaflet BSD-2, uPlot MIT, Svelte MIT, date © OpenStreetMap ODbL, Natural Earth domeniu public), legătura către `docs/operare.md` în text.
- [ ] **Step 1: Write the failing tests:**
  - `format.test.ts`: `fmtBytes(0)` → `'0 B'`; `fmtBytes(1536)` → `'1,5 KB'`; `fmtBytes(215*1024*1024)` → `'215 MB'`.
  - `admin-pages.spec.ts`:
    - `setări: eroare pe câmp`: interval 5 → mesajul serverului sub „Interval între verificări (s)”; interval 30 → „Setări salvate.”; reîncărcarea arată 30. La final se revine la 60.
    - `identitate`: denumirea „Dispecerat Test” → antetul aplicației afișează „Dispecerat Test”; revenire la „NetWatch”.
    - `backup`: „Creează backup acum” → un rând nou de tip „manual” cu dimensiune > 0; legătura de descărcare are `href` `/api/admin/backups/<nume>`.
    - `audit`: după acțiunile de mai sus, jurnalul conține „Setări modificate”.
    - `diagnostic`: afișează „Simulat” și versiunea.
    - `schimbare parolă`: parola actuală greșită → mesaj sub câmpul „Parola actuală”; corectă (nouă `parola-e2e-67890`) → formularul de login; login cu parola nouă reușește; apoi se revine la `parola-e2e-12345`.
- [ ] **Step 2: Run** `npm test && npm run e2e -- admin-pages` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm test && npm run e2e -- admin-pages` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(web): pagini de administrare — setări, identitate, backup, audit, diagnostic, parolă`.

### Task 18: Accesibilitate, teme, documentație și verificarea finală

**Files:**
- Create: `web/e2e/a11y.spec.ts`
- Modify: `README.md`, `docs/operare.md`, eventual tokens CSS pentru contrast
- Test: `web/e2e/a11y.spec.ts`

**Interfaces:**
- Consumes: toate paginile.
- Produces: documentație pentru interfață (paginile, URL-ul de kiosk `/dispecerat?rotatie=60&sunet=1`, Chrome `--kiosk`, permisiunea de sunet), comenzile de dezvoltare front-end (`npm ci`, `npm run build` înainte de `go build`, `npm run e2e`).
- [ ] **Step 1: Write the failing tests** `a11y.spec.ts`: pentru `/`, `/locatie/<Cahul>`, `/dispecerat`, `/harta`, `/statistici`, `/raport-24h`, `/administrare` (login și, autentificat, `/administrare/setari`), în ambele teme (`localStorage nw-theme`), `AxeBuilder` cu `withTags(['wcag2a','wcag2aa'])` → 0 încălcări (cu Cahul căzut, ca să fie prezente toate culorile de stare). Testul de tastatură: de pe `/`, `Tab` repetat parcurge navigarea, butoanele de filtru și cardurile, cu focus vizibil (`outline` calculat ≠ `none`).
- [ ] **Step 2: Run** `npm run e2e -- a11y` — Expected: FAIL sau PASS; orice încălcare se repară în tokens sau markup până la PASS. Dacă trece din prima, se notează în registru (testul a fost scris după pagini).
- [ ] **Step 3: Update docs** (README: secțiunea „Interfața web” și „Dezvoltare front-end”; operare: ecranul de dispecerat).
- [ ] **Step 4: Full verification:** `go vet ./...`, `go test ./...`, `GOOS=linux go vet ./...`, `npm run check`, `npm test`, `npm run e2e` (toate), `npm run build && go build -o netwatch.exe ./cmd/netwatch`, apoi pornire reală `./netwatch.exe run --data <tmp> --listen 127.0.0.1:18601` și `curl` pe `/`, `/dispecerat` (HTML-ul aplicației) și `/assets/...` (cache `immutable`). Expected: totul trece.
- [ ] **Step 5: Commit** `docs: interfața web, dispecerat și dezvoltare front-end; teste de accesibilitate`.
