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

Setările de monitorizare (interval, pachete, praguri, expirare, ora backup-ului) se modifică din **Administrare → Setări monitorizare** și se aplică imediat.

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
| `GET /api/state`, `/api/events` (SSE), `/api/catalog`, `/api/stats`, `/api/report24`, `/api/devices/{id}/timeline`, `/api/devices/{id}/latency` | public (LAN) |
| `/api/admin/session`, `setup`, `login`, `logout` | autentificare |
| `/api/admin/locations[/{id}]`, `devices/test`, `settings`, `identity`, `audit`, `diagnostic`, `backups[/{name}]`, `password` | sesiune de administrator + `X-CSRF-Token` pentru modificări |

Parametri pentru statistici: `from`, `to` (ms; implicit ultimele 24 h; maximum 400 de zile), `location`, `device`, `kind`, `incidents=all|active|closed`.
