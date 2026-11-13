package api

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"

	"sismon/internal/apperr"
	"sismon/internal/auth"
)

const cookieName = "nw_session"

type admin struct{ d Deps }

type sessionKey struct{}

type sessionInfo struct{ token string }

func registerAdmin(mux *http.ServeMux, d Deps) {
	a := &admin{d}
	mux.HandleFunc("GET /api/admin/session", a.session)
	mux.HandleFunc("POST /api/admin/setup", a.setup)
	mux.HandleFunc("POST /api/admin/login", a.login)
	mux.HandleFunc("POST /api/admin/logout", a.guard(a.logout))
	mux.HandleFunc("GET /api/admin/locations", a.guard(a.listLocations))
	mux.HandleFunc("GET /api/admin/export/{file}", a.guard(a.exportLocationsFile))
	mux.HandleFunc("POST /api/admin/import/preview", a.guard(a.importPreview))
	mux.HandleFunc("POST /api/admin/import/apply", a.guard(a.importApply))
	mux.HandleFunc("POST /api/admin/locations", a.guard(a.createLocation))
	mux.HandleFunc("GET /api/admin/locations/archived", a.guard(a.listArchivedLocations))
	mux.HandleFunc("PUT /api/admin/locations/order", a.guard(a.orderLocations))
	mux.HandleFunc("PUT /api/admin/locations/{id}", a.guard(a.updateLocation))
	mux.HandleFunc("DELETE /api/admin/locations/{id}", a.guard(a.archiveLocation))
	mux.HandleFunc("DELETE /api/admin/locations/{id}/permanent", a.guard(a.deleteLocation))
	mux.HandleFunc("POST /api/admin/devices/test", a.guard(a.testDevice))
	mux.HandleFunc("GET /api/admin/settings", a.guard(a.getSettings))
	mux.HandleFunc("PUT /api/admin/settings", a.guard(a.putSettings))
	mux.HandleFunc("GET /api/admin/identity", a.guard(a.getIdentity))
	mux.HandleFunc("PUT /api/admin/identity", a.guard(a.putIdentity))
	mux.HandleFunc("POST /api/admin/identity/logo", a.guard(a.uploadLogo))
	mux.HandleFunc("DELETE /api/admin/identity/logo", a.guard(a.deleteLogo))
	mux.HandleFunc("GET /api/admin/audit", a.guard(a.audit))
	mux.HandleFunc("GET /api/admin/diagnostic", a.guard(a.diagnostic))
	mux.HandleFunc("GET /api/admin/backups", a.guard(a.listBackups))
	mux.HandleFunc("POST /api/admin/backups", a.guard(a.createBackup))
	mux.HandleFunc("GET /api/admin/backups/{name}", a.guard(a.downloadBackup))
	mux.HandleFunc("PUT /api/admin/password", a.guard(a.changePassword))
}

func (a *admin) passwordHash(ctx context.Context) (string, error) {
	var h string
	_, err := a.d.Store.GetMeta(ctx, "password_hash", &h)
	return h, err
}

func (a *admin) setCookie(w http.ResponseWriter, ci auth.ClientInfo, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: ci.Scheme == "https", MaxAge: maxAge})
}

// current returns the session token and its CSRF token, if the request is authenticated.
func (a *admin) current(r *http.Request) (string, string, bool, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", false, nil
	}
	csrf, ok, err := a.d.Sessions.Validate(r.Context(), c.Value, a.d.Now())
	return c.Value, csrf, ok, err
}

// guard requires a valid session, and for mutating methods a matching
// X-CSRF-Token header and a same-origin request.
func (a *admin) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, csrf, ok, err := a.current(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if !ok {
			writeError(w, &apperr.Error{Status: http.StatusUnauthorized, Message: "Autentificare necesară."})
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			got := r.Header.Get("X-CSRF-Token")
			if subtle.ConstantTimeCompare([]byte(got), []byte(csrf)) != 1 || !auth.SameOrigin(r, a.d.Proxy.Client(r)) {
				writeError(w, apperr.Forbidden("Cerere respinsă (protecție CSRF)."))
				return
			}
		}
		h(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, sessionInfo{tok})))
	}
}

func (a *admin) session(w http.ResponseWriter, r *http.Request) {
	_, csrf, ok, err := a.current(r)
	if err != nil {
		writeError(w, err)
		return
	}
	hash, err := a.passwordHash(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	ci := a.d.Proxy.Client(r)
	out := map[string]any{"authenticated": ok, "configured": hash != "", "setup_allowed": hash == "" && setupAllowed(r, ci), "tls": ci.Scheme == "https"}
	if ok {
		out["csrf"] = csrf
	}
	writeJSON(w, 200, out)
}

// setupAllowed: a direct loopback client that also addressed the server by a
// loopback name — a DNS-rebinding page (foreign Host) is refused.
func setupAllowed(r *http.Request, ci auth.ClientInfo) bool {
	if !ci.DirectLoopback(r) {
		return false
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

type passwordBody struct {
	Password string `json:"password"`
}

func (a *admin) startSession(w http.ResponseWriter, r *http.Request) {
	tok, csrf, err := a.d.Sessions.Create(r.Context(), a.d.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	a.setCookie(w, a.d.Proxy.Client(r), tok, 0)
	writeJSON(w, 200, map[string]string{"csrf": csrf})
}

func (a *admin) setup(w http.ResponseWriter, r *http.Request) {
	ci := a.d.Proxy.Client(r)
	hash, err := a.passwordHash(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if hash != "" || !setupAllowed(r, ci) || !auth.SameOrigin(r, ci) {
		writeError(w, apperr.Forbidden("Configurarea inițială este permisă doar de pe acest server (localhost), cât timp nu există parolă."))
		return
	}
	var b passwordBody
	if err := decodeJSON(w, r, &b); err != nil {
		writeError(w, err)
		return
	}
	if err := auth.ValidatePassword(b.Password); err != nil {
		writeError(w, err)
		return
	}
	h, err := auth.HashPassword(b.Password)
	if err == nil {
		err = a.d.Store.SetMeta(r.Context(), "password_hash", h)
	}
	if err == nil {
		err = a.d.Store.Audit(r.Context(), a.d.Store.DB, "setup_password", "admin", 0, map[string]any{}, a.d.Now())
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.startSession(w, r)
}

func (a *admin) login(w http.ResponseWriter, r *http.Request) {
	ci := a.d.Proxy.Client(r)
	if !auth.SameOrigin(r, ci) {
		writeError(w, apperr.Forbidden("Cerere respinsă: origine necunoscută."))
		return
	}
	now := a.d.Now()
	if retry, ok := a.d.Limiter.Check(ci.IP, now); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
		writeError(w, &apperr.Error{Status: http.StatusTooManyRequests, Message: "Prea multe încercări. Reîncercați peste câteva minute."})
		return
	}
	var b passwordBody
	if err := decodeJSON(w, r, &b); err != nil {
		writeError(w, err)
		return
	}
	hash, err := a.passwordHash(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	select {
	case a.d.loginSem <- struct{}{}:
	default:
		w.Header().Set("Retry-After", "1")
		writeError(w, &apperr.Error{Status: http.StatusServiceUnavailable, Message: "Serverul verifică alte autentificări. Reîncercați peste o secundă."})
		return
	}
	// The slot covers the argon2id check only, not the failure delay below:
	// otherwise wrong guesses could keep the administrator from logging in.
	ok := func() bool {
		defer func() { <-a.d.loginSem }()
		return hash != "" && auth.VerifyPassword(hash, b.Password)
	}()
	if !ok {
		slog.Warn("autentificare eșuată", "ip", ci.IP.String())
		a.d.Sleep(r.Context(), a.d.Limiter.Fail(ci.IP, now))
		writeError(w, &apperr.Error{Status: http.StatusUnauthorized, Message: "Parolă incorectă sau administrator neconfigurat."})
		return
	}
	a.d.Limiter.Reset(ci.IP)
	a.startSession(w, r)
}

func (a *admin) logout(w http.ResponseWriter, r *http.Request) {
	si := r.Context().Value(sessionKey{}).(sessionInfo)
	if err := a.d.Sessions.Delete(r.Context(), si.token); err != nil {
		writeError(w, err)
		return
	}
	a.setCookie(w, a.d.Proxy.Client(r), "", -1)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type passwordChange struct {
	Old string `json:"old"`
	New string `json:"new"`
}

func (a *admin) changePassword(w http.ResponseWriter, r *http.Request) {
	var b passwordChange
	if err := decodeJSON(w, r, &b); err != nil {
		writeError(w, err)
		return
	}
	hash, err := a.passwordHash(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if !auth.VerifyPassword(hash, b.Old) {
		writeError(w, apperr.Invalid("old", "Parola actuală este incorectă."))
		return
	}
	if err := auth.ValidatePassword(b.New); err != nil {
		writeError(w, apperr.Invalid("new", err.(*apperr.Error).Message))
		return
	}
	h, err := auth.HashPassword(b.New)
	if err == nil {
		err = a.d.Store.SetMeta(r.Context(), "password_hash", h)
	}
	if err == nil {
		err = a.d.Sessions.DeleteAll(r.Context())
	}
	if err == nil {
		err = a.d.Store.Audit(r.Context(), a.d.Store.DB, "change_password", "admin", 0, map[string]any{}, a.d.Now())
	}
	if err != nil {
		writeError(w, err)
		return
	}
	a.setCookie(w, a.d.Proxy.Client(r), "", -1)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
