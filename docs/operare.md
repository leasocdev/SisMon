# NetWatch 1.0 — Ghid de operare

NetWatch este un singur fișier executabil (`netwatch.exe` pe Windows, `netwatch` pe Linux). Nu are nevoie de Python, Node.js sau alte programe. Pingul ICMP este trimis direct de aplicație, fără comanda `ping` a sistemului.

## 1. Instalare

### Windows

1. Copiați `netwatch.exe` într-un director permanent, de exemplu `C:\Program Files\NetWatch\`.
2. Deschideți **PowerShell ca administrator** în acel director.
3. Setați parola administratorului:
   ```powershell
   .\netwatch.exe set-password
   ```
4. Instalați și porniți serviciul:
   ```powershell
   .\netwatch.exe install
   .\netwatch.exe start
   .\netwatch.exe status
   ```
5. Deschideți în browser `http://localhost:8501/`.

Datele sunt păstrate implicit în `C:\ProgramData\NetWatch`. Serviciul pornește automat la boot și este repornit la eroare. Pe Windows, ICMP nu necesită drepturi suplimentare.

### Linux (systemd)

1. Creați contul de serviciu și instalați executabilul:
   ```bash
   sudo useradd --system --home /var/lib/netwatch --shell /usr/sbin/nologin netwatch
   sudo install -m 0755 netwatch /usr/local/bin/netwatch
   sudo install -d -o netwatch -g netwatch -m 0750 /var/lib/netwatch
   ```
2. Setați parola (ca utilizatorul serviciului, ca fișierele să aibă proprietarul corect):
   ```bash
   sudo -u netwatch netwatch set-password
   ```
3. Instalați și porniți serviciul:
   ```bash
   sudo netwatch install
   sudo netwatch start
   sudo netwatch status
   ```

Unitatea systemd rulează ca `netwatch`, cu `AmbientCapabilities=CAP_NET_RAW`, deci ICMP funcționează fără root. Pentru rularea manuală (`netwatch run`) cu un utilizator obișnuit, activați socketurile ICMP neprivilegiate:

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"
echo 'net.ipv4.ping_group_range = 0 2147483647' | sudo tee /etc/sysctl.d/60-netwatch.conf
```

Dacă ICMP nu este permis, pagina Diagnostic afișează „Monitorul nu are permisiunea de a trimite pachete ICMP”, iar echipamentele apar cu starea **Eroare monitor**, nu ca indisponibile.

## 2. Prima configurare

Parola poate fi setată în două feluri:

- din consolă: `netwatch set-password`;
- din browser, **doar de pe serverul însuși** (`http://localhost:8501/administrare`), cât timp nu există încă o parolă.

Parola are 12–200 caractere. Orice schimbare a parolei închide toate sesiunile deschise.

## 3. Opțiuni de pornire

| Opțiune | Implicit | Descriere |
|---|---|---|
| `--data DIR` | Windows `C:\ProgramData\NetWatch`, Linux `/var/lib/netwatch` | Directorul de date |
| `--listen ADR:PORT` | `:8501` | Adresa și portul HTTP(S) |
| `--tls auto` | — | HTTPS cu certificat autosemnat, generat în `DIR/tls/` (valabil 5 ani) |
| `--tls files --tls-cert C --tls-key K` | — | HTTPS cu certificatul propriu |
| `--trusted-proxy IP/CIDR` | — | Reverse proxy de încredere (repetabil) |

Aceleași opțiuni pot fi puse în `DIR/netwatch.toml`. Opțiunile din linia de comandă au prioritate:

```toml
listen = ":8501"
tls = "auto"
# tls_cert = "/etc/ssl/netwatch.crt"
# tls_key  = "/etc/ssl/netwatch.key"
trusted_proxy = ["10.0.0.5"]
```

`netwatch install` salvează în serviciu opțiunile primite la instalare. Pentru a le schimba ulterior, editați `netwatch.toml` și reporniți serviciul (`netwatch stop`, apoi `netwatch start`).

Setările de monitorizare (interval, pachete, praguri, expirare, ora backup-ului, rotația și schema de culori ale Dispeceratului) se modifică din **Administrare → Setări monitorizare** și se aplică imediat.

## 4. Reverse proxy (opțional)

Exemplu nginx:

```nginx
server {
    listen 443 ssl;
    server_name netwatch.exemplu.md;
    # ssl_certificate ...; ssl_certificate_key ...;

    location / {
        proxy_pass http://10.0.0.10:8501;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
    }
    location /api/events {
        proxy_pass http://10.0.0.10:8501;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_buffering off;
        proxy_read_timeout 1h;
    }
}
```

Porniți NetWatch cu `--trusted-proxy <IP-ul proxy-ului>`. Fără această opțiune, antetele `X-Forwarded-*` sunt ignorate, iar autentificarea prin proxy-ul HTTPS este respinsă.

## 5. Backup și restaurare

- **Automat:** zilnic, după ora configurată (implicit 03:00, ora Chișinăului), în `DIR/backups/`. Se păstrează 7 backup-uri zilnice, 4 săptămânale și 12 lunare. Backup-urile manuale și cele dinaintea migrărilor/restaurărilor nu se șterg automat.
- **Copie suplimentară:** setați „Director backup secundar” în Setări, de exemplu un disc de rețea.
- **Manual:** din Administrare → Backup-uri, sau `netwatch backup` (cu serviciul oprit).
- Backup-urile nu conțin sesiunile de autentificare.

Restaurare:

```bash
netwatch restore DIR/backups/netwatch-20261109-030000-auto.db
```

Comanda oprește serviciul dacă rulează, verifică fișierul, salvează baza curentă (`…-pre-restore.db`), restaurează, închide toate sesiunile și repornește serviciul.

## 6. Actualizare

1. `netwatch stop`
2. Înlocuiți executabilul.
3. `netwatch start`

La prima pornire după actualizare, migrațiile bazei rulează automat, după un backup `…-pre-migrate.db`. Dacă baza este mai nouă decât executabilul, pornirea este refuzată cu mesaj clar.

## 7. Jurnale și diagnostic

- Jurnale: `DIR/logs/netwatch.log` (JSON, rotit la 5 MB, 5 fișiere). Pe Linux, și în `journalctl -u NetWatch`.
- **Administrare → Diagnostic:** modul ICMP, verificările întârziate, durata medie a unei verificări, erorile recente, dimensiunea bazei și ultimul backup.

## 8. Rețea și firewall

- Permiteți TCP 8501 (sau portul ales) către server din subrețelele operatorilor.
- Echipamentele monitorizate trebuie să permită ICMP Echo de la IP-ul serverului.
- Pentru hartă, serverul descarcă dale de la `https://tile.openstreetmap.org` (HTTPS ieșire). Dalele se păstrează 7 zile în `DIR/tiles/` (maximum 500 MB). Fără internet, harta afișează contururi locale; monitorizarea nu este afectată.

## 9. Retenția datelor

| Date | Păstrare |
|---|---|
| Stări (intervale), incidente, audit | nelimitat |
| Măsurători brute (fiecare verificare) | 90 de zile |
| Agregate orare de latență/pierderi | nelimitat |

La aproximativ 60 de echipamente, baza crește cu sub 300 MB pe an.

## 10. API

Toate datele au format JSON. Timpii sunt în milisecunde Unix, iar duratele în secunde.

| Rută | Acces |
|---|---|
| `GET /api/state`, `/api/events` (SSE), `/api/catalog`, `/api/stats`, `/api/report24`, `/api/timelines`, `/api/locations/{id}/timeline`, `/api/devices/{id}/timeline`, `/api/devices/{id}/latency`, `/tiles/{z}/{x}/{y}.png` | public (LAN) |
| `/api/admin/session`, `setup`, `login`, `logout` | autentificare |
| `/api/admin/locations/order` | sesiune + `X-CSRF-Token` |
| `GET /api/export/csv`, `/api/export/xlsx`, `/api/export/pdf`, `GET /branding/logo` | public (LAN) |
| `POST /api/admin/import/preview`, `/api/admin/import/apply` (multipart, câmpul `file`), `POST/DELETE /api/admin/identity/logo`, `GET /api/admin/export/locations.csv`, `locations.xlsx` | sesiune (+ `X-CSRF-Token` pentru modificări) |
| `/api/admin/locations[/{id}]`, `devices/test`, `settings`, `identity`, `audit`, `diagnostic`, `backups[/{name}]`, `password` | sesiune de administrator + `X-CSRF-Token` pentru modificări |

Parametri pentru statistici: `from`, `to` (ms; implicit ultimele 24 h; maximum 400 de zile), `location`, `device`, `kind`, `incidents=all|active|closed`.

Fluxul SSE (`/api/events`) trimite `snapshot`, `state`, `removed`, `health` și `ping` (la 15 s). Identificatorii au forma `epocă-număr`; după o repornire a serverului, clientul primește automat un `snapshot` nou. Reluarea se face cu antetul `Last-Event-ID` sau cu `?last=`.

## 11. Ecranul de dispecerat

Adresa: `http://SERVER:8501/dispecerat`. Antetul arată logo-ul, denumirea și subtitlul din Identitate, iar în dreapta starea conexiunii, ora și data. Locațiile cu probleme se văd pe plăcile din grilă.

Setări permanente, în **Administrare → Setări monitorizare** (se aplică imediat, și pe ecranele deja deschise):

| Setare | Efect |
|---|---|
| Rotația din Dispecerat (s) | `0` = oprită (implicit); 10–3600 alternează grila cu harta la acest interval |
| Schema de culori a Dispeceratului | întunecată (implicit), luminoasă sau ca sistemul de operare |

Opțiuni în URL, combinabile, care au prioritate față de setări (utile pentru un ecran anume):

| Opțiune | Efect |
|---|---|
| `?rotatie=60` | alternează grila cu harta la fiecare 60 s (minimum 10) |
| `?tema=luminoasa` / `?tema=intunecata` | forțează tema |

Exemplu de mod kiosk (Chrome/Edge):

```
chrome --kiosk "http://SERVER:8501/dispecerat?rotatie=60"
```

- Dacă legătura cu serverul se pierde peste 30 de secunde, ecranul se încadrează în gri și afișează „DATE NEACTUALIZATE”. Revenirea este automată.
- Grila se dimensionează singură pentru a încăpea fără derulare (testat cu 60 de locații la 1920×1080).

## 12. Import și export

**Import locații** (Administrare → Import / export): fișier CSV sau Excel (.xlsx), cel mult 5 MB, 1000 de rânduri și 200 de locații.

- Coloane (antetul poate fi în română sau engleză, fără diacritice): `Locație`/`location`, `Adresă`, `Latitudine`, `Longitudine`, `Echipament`/`device`, `Host`/`IP`, `Tip`, `Primar`, `Observații`. Obligatorii: locație, echipament, host.
- Un rând = un echipament. Rândurile cu aceeași locație formează o locație; adresa, coordonatele și observațiile se completează o singură dată (sau identic).
- `Primar`: `da`/`nu`, `1`/`0`, `x`/gol. Fără coloana `Primar`, primul echipament al fiecărei locații este primar.
- CSV: separatorul (`,`, `;`, tab), codificarea (UTF-8, Windows-1250, Windows-1252) și virgula zecimală la coordonate sunt detectate automat. Un fișier salvat din Excel cu setări românești („CSV (delimitat prin punct și virgulă)”) funcționează direct; literele ş/ţ cu sedilă devin ș/ț.
- **Previzualizează** arată fiecare rând cu erorile pe celulă; **Importă** salvează totul într-o singură tranzacție (ori tot, ori nimic). Locațiile existente (după nume) nu sunt modificate.

**Export locații**: CSV sau Excel în același format, reimportabil fără pierderi (pragurile proprii ale echipamentelor nu fac parte din format).

**Export statistici** (pagina Statistici → CSV / Excel / PDF): aceleași date ca pe ecran, pentru perioada și filtrele alese.

- CSV: UTF-8 cu BOM, separator `;`, zecimale cu virgulă — se deschide corect în Excel cu setări românești. Blocuri: perioadă și filtre, echipamente, locații, incidente.
- Excel: foile Echipamente, Locații, Incidente, Perioadă și filtre (numerele ca numere).
- PDF: A4 orientat peisaj, cu logo, rezumat, tabel echipamente, bare de stare pe locații și incidente; fontul DejaVu Sans este încorporat.
- Valorile care încep cu `=`, `+`, `-`, `@` primesc prefixul `'` în exporturile statistice, ca Excel să nu le execute ca formule.

## 13. Logo

Administrare → Identitate → Logo: PNG de cel mult 1024 × 1024 pixeli și 500 KB. Serverul verifică și reencodează imaginea. Logo-ul apare în antet, pe ecranul de dispecerat, în Raportul 24H și în PDF. Se păstrează în `DIR/branding/logo.png` — nu face parte din backup-ul bazei; după mutarea pe alt server, încărcați-l din nou.

## 14. Mod demonstrativ

`netwatch demo [--data demo-data] [--listen :8502]` pornește o instanță separată cu 12 locații simulate și scenarii care se repetă (de exemplu Cahul cade 6 minute din 30, Orhei este degradat periodic, Soroca are latență mare). Verificările rulează de 6 ori mai des decât în mod normal, ca statisticile să se umple repede. Parola de administrare: `netwatch-demo`. Bannerul „MOD DEMONSTRATIV” apare pe toate paginile.

## 15. Versiuni noi

Binarele se produc cu `go run ./tools/release --version X.Y.Z` (sau automat de CI la eticheta `vX.Y.Z`), împreună cu `SHA256SUMS`. Verificați suma de control înainte de instalare (`sha256sum -c SHA256SUMS` pe Linux, `Get-FileHash` pe Windows), apoi urmați secțiunea 6.
