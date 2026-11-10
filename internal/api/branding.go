package api

import (
	"net/http"

	"netwatch/internal/branding"
)

func (a *admin) uploadLogo(w http.ResponseWriter, r *http.Request) {
	_, data, err := readUpload(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	img, err := branding.Prepare(data)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := branding.Save(a.d.DataDir, img); err != nil {
		writeError(w, err)
		return
	}
	a.setLogoVersion(w, r, a.d.Now().UnixMilli(), "logo")
}

func (a *admin) deleteLogo(w http.ResponseWriter, r *http.Request) {
	if err := branding.Delete(a.d.DataDir); err != nil {
		writeError(w, err)
		return
	}
	a.setLogoVersion(w, r, 0, "logo_delete")
}

func (a *admin) setLogoVersion(w http.ResponseWriter, r *http.Request, v int64, action string) {
	id, err := LoadIdentity(r.Context(), a.d.Store)
	if err == nil {
		id.LogoVersion = v
		err = a.d.Store.SetMeta(r.Context(), "identity", id)
	}
	if err == nil {
		err = a.d.Store.Audit(r.Context(), a.d.Store.DB, action, "identity", 0, map[string]int64{"logo_version": v}, a.d.Now())
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, id)
}

// logo serves the uploaded logo (public: it appears on every page).
func (p *public) logo(w http.ResponseWriter, r *http.Request) {
	b, err := branding.Load(p.d.DataDir)
	if err != nil {
		writeError(w, err)
		return
	}
	if b == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(b)
}
