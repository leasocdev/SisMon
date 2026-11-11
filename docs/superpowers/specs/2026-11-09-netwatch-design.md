# NetWatch 1.0 — Specificație de design

Data: 2026-11-09 · Stare: aprobat conversațional, în revizuire scrisă

## 1. Scop și context

NetWatch monitorizează prin ICMP disponibilitatea echipamentelor de rețea (routere, PC-uri, radiolinkuri etc.) grupate pe locații, afișează starea în timp real (inclusiv pe un ecran mare de dispecerat), păstrează istoricul și produce statistici și rapoarte. Este o reconstrucție de la zero a NetWatch 0.4 (Python/Flask), care elimină neajunsurile identificate la analiza acesteia. **Nu există migrare din 0.4** — instalarea pornește pe o bază nouă.

### Cerințe stabilite

| Subiect | Decizie |
|---|---|
| Rețea | Serverul are acces la internet; monitorizarea ICMP internă trebuie să continue și fără internet. |
| Distribuție | Un singur executabil pentru Windows și Linux (amd64; Linux și arm64), fără dependențe externe. |
| Volum | ~20 locații / ~60 echipamente; proiectat să funcționeze confortabil până la 500 echipamente. |
| Istoric | Stări și incidente păstrate integral; măsurători brute 90 de zile; agregate orare nelimitat. |
| Acces | Vizualizare liberă în LAN; un singur administrator cu parolă. |
| Alerte | Doar sunet în browser (fără notificări externe). |
| Utilizare UI | Ecran mare permanent (dispecerat) și calculatoarele operatorilor. |
| Limbă / fus | Interfață în română; toate orele în `Europe/Chisinau`, afișat explicit. |

### Criterii de succes

1. Starea „indisponibil” este detectată corect, inclusiv rețea/rută inaccesibilă (cădere VPN); erorile locale ale monitorului sunt distinse de căderi.
2. Raportul pe toată perioada, după un an de date pentru 60 de echipamente, se generează în sub 500 ms; baza rămâne sub 300 MB.
3. Backup-ul, agregarea și curățarea nu întârzie ciclul de monitorizare.
4. Instalarea ca serviciu se face cu o singură comandă; actualizarea prin înlocuirea executabilului.
5. Nicio intrare invalidă nu produce eroare 500; toate mesajele pentru utilizator sunt în română.

### În afara scopului (YAGNI)

Notificări externe (e-mail, Telegram, webhook); conturi multiple/roluri; verificare automată de versiuni noi; monitorizare TCP/HTTP/SNMP; migrare din 0.4; aplicație mobilă (UI-ul rămâne utilizabil pe ecrane mici, fără optimizare dedicată).

## 2. Arhitectură

```
                    ┌──────────────── netwatch (un proces) ────────────────┐
 browser ──HTTP/SSE─┤ api (handlers, auth, SSE hub) ── webui (go:embed)    │
                    │        │ citiri                                       │
                    │        ▼                                              │
                    │ live state (memorie) ◄── monitor (scheduler+pinger)   │
                    │        │                         │ rezultate          │
                    │        ▼                         ▼                    │
                    │ store (SQLite WAL) ◄── writer (un singur fir)         │
                    │        ▲                                              │
                    │ jobs: backup · agregare · retenție · optimize         │
                    │ tiles (proxy OSM, cache LRU, pool separat)            │
                    └───────────────────────────────────────────────────────┘
```

### Tehnologii

- **Go** (versiunea stabilă curentă), `CGO_ENABLED=0`, cross-compilare din orice OS.
- **SQLite** prin `modernc.org/sqlite` (Go pur), mod WAL, `busy_timeout=5000`.
- **ICMP**: `golang.org/x/net/icmp` pe Linux; `IcmpSendEcho2`/`Icmp6SendEcho2` din `iphlpapi.dll` prin `golang.org/x/sys/windows` pe Windows.
- **Serviciu**: `github.com/kardianos/service` (Windows Service, systemd).
- **Parolă**: Argon2id (`golang.org/x/crypto/argon2`).
- **Exporturi**: `github.com/xuri/excelize/v2` (XLSX), `github.com/go-pdf/fpdf` cu font DejaVu Sans încorporat (PDF), `encoding/csv`.
- **Fus orar**: `time/tzdata` încorporat; toate formatările folosesc `Europe/Chisinau`.
- **Frontend**: Svelte 5 + TypeScript + Vite; Leaflet + Leaflet.markercluster (hartă); uPlot (grafice). Compilat în `internal/webui/dist` și încorporat prin `go:embed`. Node este necesar doar la dezvoltare.

### Structura depozitului

```
cmd/netwatch/            main, subcomenzi CLI
internal/config/         netwatch.toml, flag-uri, directoare implicite
internal/store/          conexiune, migrații versionate, interogări tipizate
internal/state/          reguli pure: histerezis, starea locației, intervale
internal/monitor/        scheduler, pool, interfața Pinger
internal/monitor/icmp/   pinger_linux.go, pinger_windows.go, pinger_fake.go
internal/live/           starea curentă în memorie + abonați (SSE)
internal/stats/          durate, disponibilitate, incidente, serii pentru grafice
internal/api/            rutare, handlers publici și admin, validare, erori
internal/auth/           parole, sesiuni, CSRF, rate-limit, proxy de încredere
internal/tiles/          proxy OSM cu cache LRU
internal/jobs/           backup, rotație, agregare orară, retenție, optimize
internal/export/         csv, xlsx, pdf
internal/importer/       CSV/XLSX: detectare separator/codificare/zecimale
internal/demo/           scenarii simulate pentru pinger_fake
internal/webui/          go:embed pentru dist/
web/                     sursa Svelte
docs/
```

Fiecare pachet are o responsabilitate clară; `state` și `stats` sunt funcții pure testabile fără bază sau rețea.

## 3. Model de date

Toate momentele de timp sunt stocate ca milisecunde Unix UTC (`INTEGER`).

```sql
meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)          -- versiune schemă, identitate, setări globale (JSON)
locations(id, name TEXT NOT NULL UNIQUE COLLATE NOCASE, address, lat, lon, notes,
          sort_order, created_at, archived_at)
devices(id, location_id → locations, name, host, kind, is_primary INTEGER,
        sort_order, interval_s NULL, failures NULL, recoveries NULL,   -- suprascrieri opționale
        created_at, archived_at,
        UNIQUE(location_id, host) WHERE archived_at IS NULL)
state_intervals(id, entity TEXT CHECK(entity IN ('device','location')), entity_id,
                state TEXT CHECK(state IN ('up','down','degraded','unknown','error')),
                start_at, end_at NULL)
  INDEX (entity, entity_id, start_at); INDEX parțial (entity, entity_id) WHERE end_at IS NULL
samples(device_id, ts, ok INTEGER, latency_ms REAL NULL, loss_pct REAL, error_code TEXT NULL)
  INDEX (device_id, ts)                                    -- retenție 90 de zile
samples_hourly(device_id, hour_ts, checks, ok_checks, lat_avg, lat_min, lat_max, loss_avg,
               PRIMARY KEY(device_id, hour_ts))            -- nelimitat
incidents(id, device_id, observed_at, confirmed_at, ended_at NULL, end_reason NULL)
  INDEX (device_id, observed_at); INDEX parțial WHERE ended_at IS NULL
audit(id, ts, action, entity, entity_id, details_json)
sessions(token_hash BLOB PRIMARY KEY, created_at, last_seen_at, csrf TEXT)
```

- `end_reason` ∈ `recovered`, `monitoring_gap`, `monitor_error`, `configuration`, `archived`; traducerea în română se face într-un singur loc și este folosită de UI și de toate exporturile.
- Arhivarea setează `archived_at`; nu există ștergere definitivă. Elementele arhivate rămân în statistici și filtre.
- Modificarea `host` la un echipament închide intervalul și incidentul curent (`configuration`) și pornește un interval `unknown`.

## 4. Reguli de stare

### Stările echipamentului

| Stare | Semnificație |
|---|---|
| `up` | Răspunde la ICMP (confirmat). |
| `down` | Nu răspunde: timeout, host/rețea inaccesibilă, rută lipsă, răspuns ICMP Destination Unreachable (confirmat). |
| `unknown` | Nu există date recente (echipament nou, monitor oprit, pauză peste pragul de expirare). |
| `error` | Eroare locală a monitorului (lipsă permisiune socket, eroare internă). Afișat distinct: „Eroare monitor”. |

### Histerezis

- Parametri globali (cu suprascriere pe echipament): `failures` = 3, `recoveries` = 2.
- `up`/`unknown` → `down` după `failures` verificări eșuate consecutive; `down`/`unknown` → `up` după `recoveries` reușite consecutive.
- Orice `error` resetează contoarele și setează starea `error`; un incident deschis se închide cu `monitor_error`.
- În timpul confirmării, UI afișează „cădere în confirmare” / „revenire în confirmare”.

### Atribuire retroactivă

- La confirmarea căderii, intervalul anterior se închide la momentul **primei verificări eșuate** din serie (`observed_at`), iar intervalul `down` începe de acolo. Incidentul are `observed_at` și `confirmed_at`.
- La confirmarea revenirii, intervalul `down` se închide la momentul **primei verificări reușite** din serie; incidentul se închide cu `recovered` la același moment.
- Astfel, duratele din cronologie, statistici, raportul 24H și exporturi sunt identice.

### Pauze de monitorizare

- `stale` (implicit 300 s) = vârsta maximă a ultimei verificări pentru ca starea să fie considerată valabilă.
- Dacă trec mai mult de `stale` secunde fără verificare (monitor oprit, server oprit), intervalul curent se închide la `ultima_verificare + stale`, se deschide `unknown`, iar un incident deschis se închide cu `monitoring_gap` la același moment. Verificarea se face la pornire și periodic de un job de supraveghere (la 30 s).

### Starea locației

Calculată din stările echipamentelor active; `error` se tratează ca `unknown` pentru calcul.

1. Oricare primar `down` → `down` (roșu).
2. Altfel, oricare primar nu este `up` → `unknown` (gri).
3. Altfel, oricare secundar `down` → `degraded` (portocaliu), cu marcaj „date incomplete” dacă alt secundar nu este `up`.
4. Altfel, oricare secundar nu este `up` → `unknown`.
5. Altfel → `up` (verde).

O locație fără echipamente primare nu poate fi salvată. Starea locației este înregistrată în `state_intervals` doar la schimbare.

### Indicatori

- **Disponibilitate** = `up` / (`up` + `down`) pentru echipamente; (`up` + `degraded`) / (`up` + `degraded` + `down`) pentru locații (`degraded` = doar secundare căzute, locația funcționează). „—” dacă numitorul e zero.
- **Acoperire** = timp cu stare cunoscută (`up`, `down`, `degraded`) / durata perioadei eligibile.
- **Perioada eligibilă** = intersecția perioadei selectate cu [`created_at`, `archived_at` sau acum].
- **MTTR** = media duratelor incidentelor încheiate cu `recovered` în perioadă.
- Duratele incidentelor sunt limitate la perioada selectată.
- Incidentele se deschid pentru toate echipamentele (setare globală: „toate” / „doar primare”, implicit „toate”).

## 5. Motorul de monitorizare

### Planificare

- Fiecare echipament activ are propriul termen următor; prima verificare este eșalonată uniform în intervalul global (fără rafale).
- Un pool de până la 64 de verificări simultane. Un echipament nu are niciodată două verificări simultane; dacă o verificare depășește intervalul, următoarea pornește imediat după, iar întârzierea este raportată în diagnostic.
- Modificările de configurație se aplică imediat (echipament nou, host schimbat, arhivare).

### Verificare

- N pachete (implicit 3, 1–10), la 200 ms distanță, timeout pe pachet (implicit 2 s, 1–10 s). Reușită = cel puțin un răspuns. Latența = media răspunsurilor; pierderi = pachete fără răspuns / N.
- DNS: rezolvare cu timeout de 2 s; ultima adresă reușită este păstrată în memorie. La eșec DNS se folosește ultima adresă cunoscută și se afișează avertismentul „DNS indisponibil”; fără adresă cunoscută, verificarea este `down` cu cod `dns`.
- IPv4 și IPv6; pentru nume cu ambele, se preferă IPv4 (configurabil global).

### Clasificarea erorilor

| Situație | Rezultat | Cod |
|---|---|---|
| Fără răspuns în timeout | eșec | `timeout` |
| ICMP Destination Unreachable / `IP_DEST_HOST_UNREACHABLE` / `EHOSTUNREACH` | eșec | `host_unreachable` |
| `IP_DEST_NET_UNREACHABLE` / `ENETUNREACH` / rută lipsă | eșec | `net_unreachable` |
| DNS fără adresă cunoscută | eșec | `dns` |
| `EPERM`/`EACCES`, socket indisponibil, eroare API | `error` | `permission` / `internal` |

### Implementări Pinger

- **Linux**: socket ICMP neprivilegiat (`udp4`/`udp6`), dacă `net.ipv4.ping_group_range` permite; altfel socket raw cu `CAP_NET_RAW` (setat de `netwatch install` prin `AmbientCapabilities` în unitatea systemd). Dacă niciuna nu este disponibilă: toate echipamentele `error/permission` și mesaj clar în UI.
- **Windows**: `IcmpSendEcho2` / `Icmp6SendEcho2`; nu necesită drepturi de administrator.
- **Fake**: scenarii deterministe pentru teste și modul demo.

Interfața: `Ping(ctx, addr netip.Addr, packets int, timeout time.Duration) Result`.

### Scriere și timp real

- Rezultatele intră pe un canal către **un singur fir de scriere**, care le aplică în loturi (cel mult o dată pe secundă) într-o tranzacție: `samples`, intervale, incidente.
- `live` ține starea curentă a tuturor echipamentelor și locațiilor în memorie; API-ul de stare o citește fără interogări.
- La fiecare schimbare de stare (sau de stare în confirmare), `live` publică un eveniment către abonații SSE.

### Validarea setărilor globale

| Parametru | Limite | Implicit |
|---|---|---|
| `interval_s` | 10–3600 | 60 |
| `packets` | 1–10 | 3 |
| `timeout_s` | 1–10 | 2 |
| `failures` | 1–20 | 3 |
| `recoveries` | 1–20 | 2 |
| `stale_s` | 30–86400 | 300 |

Reguli suplimentare (aceleași pentru suprascrierile pe echipament): `packets × timeout + (packets−1) × 0,2 s + 2 s (DNS) < interval`; `stale ≥ interval × (max(failures, recoveries) + 1)`. Mesajele de eroare explică valorile minime acceptate.

### Diagnostic

Publicat în `/api/state` și în pagina Diagnostic: ultimul ciclu, verificări întârziate, durata medie a unei verificări, modul ICMP (neprivilegiat/raw/Windows), ultimele 50 de erori, dimensiunea bazei, starea ultimului backup. Dacă monitorul rămâne în urmă sau nu a scris nimic în ultimele 2 × interval, UI afișează avertisment.

## 6. API HTTP

Toate răspunsurile sunt JSON (cu excepția exporturilor/dalelor), `Cache-Control: no-store` pentru `/api`. Erorile au forma `{"error": "mesaj în română", "field": "opțional"}`. Corpurile JSON se decodează strict (câmpuri necunoscute → 400). Dimensiune maximă cerere: 1 MB (5 MB pentru import și logo).

### Public (fără autentificare)

| Metodă și rută | Descriere |
|---|---|
| `GET /api/state` | Locații, echipamente, stări curente, în confirmare, latență, identitate, sănătatea monitorului. |
| `GET /api/events` | SSE: `state` (schimbări), `health`, `ping` la 15 s. Reconectare automată cu `Last-Event-ID`. |
| `GET /api/catalog` | Locații și echipamente, inclusiv arhivate, pentru filtre. |
| `GET /api/stats` | Parametri: `from`, `to` (ms; maxim 400 de zile), `location`, `device`, `kind`, `incidents=all\|active\|closed`. |
| `GET /api/devices/{id}/timeline?from&to` | Intervale de stare. |
| `GET /api/devices/{id}/latency?from&to` | Serie: brut până la 48 h, agregat pe 15 min până la 7 zile, orar peste. Maxim ~500 de puncte. |
| `GET /api/report24` | Raport pe ultimele 24 h. |
| `GET /api/export/{csv\|xlsx\|pdf}` | Aceiași parametri ca `/api/stats`. |
| `GET /tiles/{z}/{x}/{y}.png` | Proxy OSM (vezi mai jos). |
| `GET /branding/logo` | Logo-ul încărcat. |

### Administrare (`/api/admin/*`, sesiune + CSRF)

Login/logout/sesiune; CRUD locații și echipamente (cu arhivare); `POST /devices/test` (ping imediat pentru un host, fără salvare); setări globale; identitate și logo; import (previzualizare, aplicare); export locații (CSV/XLSX); backup-uri (listă, creare, descărcare); audit (paginat, filtrabil); diagnostic; schimbare parolă.

### Proxy de dale OSM

- Validare `z` 2–18, `x`,`y` în domeniu. Cache pe disc în `data/tiles/`, limită 500 MB, eliminare LRU; dale valabile 7 zile, servite și expirate dacă OSM nu răspunde.
- Maxim 4 descărcări simultane spre OSM, cereri identice deduplicate, timeout total 5 s (inclusiv DNS și conectare). Handler-ul rulează independent de restul API-ului; când pool-ul e plin răspunde imediat 503.
- `User-Agent` cu numele aplicației și versiunea, conform politicii OSM; fără preîncărcare.

## 7. Securitate

- **Sesiuni pe server**: token aleator de 32 de octeți în cookie `nw_session` (`HttpOnly`, `SameSite=Strict`, `Secure` când cererea e HTTPS); în bază se păstrează doar SHA-256 al tokenului. Expiră după 8 h de inactivitate și 24 h absolut. Logout, schimbarea parolei și restaurarea șterg sesiunile.
- **CSRF**: token per sesiune, trimis în antetul `X-CSRF-Token` pentru orice metodă care modifică date; plus verificarea `Origin`/`Referer` față de schema și host-ul efectiv.
- **Proxy de încredere**: `--trusted-proxy <IP/CIDR>` (repetabil). Doar pentru aceste surse se folosesc `X-Forwarded-For`, `X-Forwarded-Proto`, `X-Forwarded-Host` (pentru IP-ul clientului, rate-limit, `Origin`, cookie `Secure`).
- **HTTPS opțional**: `--tls auto` (certificat autosemnat generat în directorul de date, valabil 5 ani) sau `--tls-cert`/`--tls-key`. Fără TLS, pagina de login afișează avertisment.
- **Parolă**: Argon2id (m=64 MiB, t=3, p=2), 12–200 caractere. Rate-limit: 5 încercări eșuate / 5 min / IP, cu întârziere progresivă; tabelă limitată cu eliminarea celor mai vechi intrări (fără golire totală).
- **Prima configurare**: dacă nu există parolă, `/administrare` afișează formularul de setare doar pentru conexiuni loopback fără antete de proxy; altfel indică comanda `netwatch set-password`.
- **Antete**: CSP strict (`default-src 'self'`; `img-src 'self' data:`; fără `unsafe-inline`), `X-Content-Type-Options`, `Referrer-Policy`, `frame-ancestors 'none'`.
- **Logo**: PNG validat (structură, CRC, maximum 1024 × 1024, 500 KB) și reencodat de server înainte de salvare.
- **Exporturi**: protecție împotriva injectării de formule doar în CSV/XLSX statistice; exportul locațiilor (destinat reimportului) scrie valorile neschimbate, iar în XLSX ca text explicit.
- Backup-urile nu conțin tabela `sessions`.

## 8. Interfața

### Principii

- Stil sobru, densitate informațională ridicată, fără decorațiuni.
- Starea se exprimă prin culoare **și** simbol/text: ✓ Funcțională, ▲ Degradată, ✕ Indisponibilă, ? Fără date, ! Eroare monitor.
- Teme luminoasă și întunecată (urmează sistemul, comutare manuală reținută local); dispeceratul folosește implicit tema întunecată.
- Toate orele în `Europe/Chisinau`, cu mențiunea fusului în antetul paginilor de raport.
- Navigare completă din tastatură; contrast WCAG AA; regiune `aria-live` pentru schimbări de stare.
- Indicator de conexiune pe toate paginile: dacă SSE este întrerupt peste 30 s, stările sunt marcate „neactualizate” (nu se păstrează verde fals).

### Pagini

1. **Monitorizare `/`** — bara de sus: totaluri pe stări (filtrabile la click), căutare după nume/IP, sănătatea monitorului, buton sunet. Grilă de carduri sortată implicit după gravitate (roșu, portocaliu, gri/eroare, verde), cu opțiuni alfabetic/manual. Card: nume, stare, durata stării curente, echipamente cu ★ pentru primare, latență curentă, mini-cronologie pe 24 h; puls scurt la o problemă nouă. Click → panou lateral de detalii (`/locatie/{id}`, URL partajabil), actualizat în timp real: echipamente cu stare/în confirmare/eroare, cronologie 24 h/7 z/30 z, grafic latență cu min/max și pierderi, incidentele locației.
2. **Dispecerat `/dispecerat`** — ecran complet, fără meniuri, potrivit pentru modul kiosk. Grilă auto-dimensionată fără derulare, text mare. Bandă superioară cu locațiile cu probleme („Cahul · Router · de 12 min”). Ceas mare și indicator de conexiune; la pierderea conexiunii, chenar gri „DATE NEACTUALIZATE”. Opțiuni în URL: rotație grilă/hartă la N secunde, sunet.
3. **Hartă `/harta`** — Leaflet cu dale prin proxy, marcaje colorate cu simbol, grupare la zoom mic, popup cu detalii și legătură spre panou. O dală eșuată se reîncearcă; trecerea la contururi locale (GeoJSON Natural Earth încorporat) doar dacă eșuează majoritatea dalelor vizibile, cu revenire automată.
4. **Statistici `/statistici`** — filtre sincronizate cu URL-ul; perioade rapide (24 h, 7 z, 30 z, luna curentă, luna trecută) sau personalizată. Indicatori: disponibilitate, acoperire, număr incidente, timp indisponibil cumulat, MTTR. Tabel sortabil pe echipamente, bare de stare pe locații, listă de incidente. Exporturi CSV/Excel/PDF cu exact datele afișate.
5. **Raport 24H `/raport-24h`** — actualizat automat (SSE); situația curentă pe primare, întreruperile primarelor din ultimele 24 h și o secțiune separată „Incidente minore” pentru secundare; stil de tipărire pe A4.
6. **Administrare `/administrare`** — listă locații cu căutare; editor: date generale, coordonate pe hartă, rânduri de echipamente cu reordonare prin tragere, ★ primar, suprascrieri opționale ale pragurilor, buton „Testează acum” pe rând; avertisment la modificări nesalvate. Import: încărcare, previzualizare tabelară cu erori pe rând și celulă, aplicare atomică. Pagini: Setări monitorizare, Identitate și logo, Backup-uri, Jurnal audit, Diagnostic, Parolă, Despre.

### Sunet

Comutator pe paginile Monitorizare, Dispecerat și Raport 24H; preferință reținută local. Semnal la apariția unei noi locații `down` (și opțional la `degraded`); semnal diferit la revenire. Dacă browserul blochează audio, butonul indică „Click pentru a activa sunetul”.

## 9. Import și export

### Import locații (CSV/XLSX)

- Coloane (antete insensibile la majuscule, cu sinonime în română): `location`, `address`, `lat`, `lon`, `device`, `host`, `kind`, `is_primary`, `notes`.
- CSV: detectare automată a separatorului (`,`, `;`, tab), codificării (UTF-8 cu/fără BOM, Windows-1250, Windows-1252) și a zecimalei cu virgulă la coordonate.
- Rânduri complet goale ignorate; maxim 1000 de rânduri și 200 de locații per fișier.
- `is_primary`: 1/0, da/nu, true/false, x/gol; fără coloană → primul echipament este primar.
- Locațiile existente (după nume) nu sunt modificate: previzualizarea le marchează ca eroare. Aplicarea se face într-o singură tranzacție.

### Exporturi

- **Statistici** CSV (UTF-8 cu BOM, separator `;` pentru compatibilitate cu Excel în setările regionale românești), XLSX (foi: Echipamente, Locații, Incidente, Perioadă și filtre), PDF A4 landscape (rezumat, tabel echipamente, bare de stare locații, incidente; font DejaVu Sans; număr de pagină și fus orar în subsol).
- **Locații** CSV/XLSX în formatul de import (reimportabil fără pierderi).

## 10. Operare

### CLI

| Comandă | Efect |
|---|---|
| `netwatch run` | Rulare în prim-plan. Flag-uri: `--data`, `--listen` (implicit `:8501`), `--tls`, `--tls-cert`, `--tls-key`, `--trusted-proxy`. |
| `netwatch install` / `uninstall` | Instalează/elimină serviciul (Windows Service; systemd cu utilizator dedicat `netwatch`, `AmbientCapabilities=CAP_NET_RAW`). |
| `netwatch start` / `stop` / `status` | Control serviciu. |
| `netwatch set-password` | Parolă administrator (interactiv, fără ecou). |
| `netwatch backup` | Backup imediat. |
| `netwatch restore FIȘIER` | Oprește serviciul dacă rulează, verifică integritatea și versiunea, salvează baza curentă, restaurează, șterge sesiunile, repornește serviciul. |
| `netwatch demo` | Rulează cu pinger simulat într-un director separat (`demo-data`), cu scenarii care evoluează în timp; banner „MOD DEMONSTRATIV”. |
| `netwatch version` | Versiunea. |

- Ieșirea consolei este UTF-8 (pe Windows se setează pagina de cod a consolei; la redirecționare se scrie UTF-8).
- Un lock exclusiv pe directorul de date împiedică două instanțe; fișierul de lock nu crește.

### Configurare și directoare

- Implicit: Windows `C:\ProgramData\NetWatch`, Linux `/var/lib/netwatch`; în modul `run` fără serviciu se poate folosi `--data`.
- `netwatch.toml` în directorul de date pentru opțiunile de pornire; flag-urile au prioritate. Setările de monitorizare sunt în bază și se modifică din UI.

### Joburi de fundal (fiecare pe firul propriu)

| Job | Program | Detalii |
|---|---|---|
| Backup | zilnic, ora configurabilă (implicit 03:00) | `VACUUM INTO` în fișier temporar, eliminare `sessions`, verificare `integrity_check`, redenumire atomică. Rotație 7 zilnice / 4 săptămânale / 12 lunare. Director creat complet dacă lipsește. La eșec: avertisment în UI, reîncercare după 1 h. Copie opțională într-un al doilea director. |
| Agregare orară | la fiecare oră, pentru ora încheiată | Scrie `samples_hourly`; idempotent. |
| Retenție | zilnic | Șterge `samples` mai vechi de 90 de zile (în loturi), după confirmarea agregării. |
| Optimizare | zilnic, după retenție | `PRAGMA optimize`; `wal_checkpoint(TRUNCATE)`. |
| Supraveghere pauze | la 30 s | Aplică regula de pauză (secțiunea 4). |
| Curățare sesiuni | la 10 min | Șterge sesiunile expirate. |

### Jurnale

Fișier rotit `logs/netwatch.log` (5 × 5 MB, JSON pe linie) și ieșirea standard (journald pe Linux; Event Log prin `kardianos/service` pe Windows).

### Actualizare

Se înlocuiește executabilul și se repornește serviciul. Migrațiile de schemă sunt versionate, rulează automat la pornire, precedate de un backup automat; o schemă mai nouă decât executabilul blochează pornirea cu mesaj clar.

## 11. Testare

- **Unitare** (`go test`): regulile de stare și histerezis, atribuirea retroactivă, pauzele; duratele/disponibilitatea/acoperirea/MTTR pe cazuri limită (perioade tăiate, arhivare, schimbare primare, host schimbat); clasificarea erorilor ICMP (pinger fake și mapare coduri OS); validarea setărilor; importul (separatori, codificări, zecimale, sinonime antete, atomicitate); exporturi (conținut, reimport locații fără pierderi); auth (sesiuni, CSRF, rate-limit, proxy de încredere, prima configurare doar loopback).
- **Integrare**: server real pe port aleator cu pinger fake — SSE, fluxuri admin complete, restaurare, joburi (backup cu rotație, retenție, agregare).
- **Performanță**: un an simulat pentru 60 de echipamente — raport pe toată perioada < 500 ms, bază < 300 MB; ciclu cu 500 de echipamente fără întârzieri.
- **ICMP real**: ping la `127.0.0.1`/`::1` (reușit) și la o adresă TEST-NET (eșec) pe Windows și Linux în CI.
- **End-to-end UI** (Playwright): monitorizare și panou detalii, dispecerat (inclusiv pierderea conexiunii), editare locație cu „Testează acum”, import cu erori, statistici și export.
- **Build**: `go run ./tools/release` (fără dependență de `make`) / GitHub Actions produce `netwatch-windows-amd64.exe`, `netwatch-linux-amd64`, `netwatch-linux-arm64`, cu sume de control.
