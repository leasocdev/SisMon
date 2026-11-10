package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the embedded UI: existing files as-is, extension-less
// unknown paths as index.html (client-side routes), everything else 404.
func spaHandler(web fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if fi, err := fs.Stat(web, name); err == nil && !fi.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				http.ServeFileFS(w, r, web, name)
				return
			}
			if path.Ext(name) != "" || name == "assets" || strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r)
				return
			}
		}
		b, err := fs.ReadFile(web, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(b)
	})
}
