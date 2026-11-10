package api

import (
	"errors"
	"io"
	"net/http"

	"netwatch/internal/apperr"
	"netwatch/internal/domain"
	"netwatch/internal/importer"
)

// MaxUpload bounds multipart uploads (import files and the logo).
const MaxUpload = 5 << 20

// readUpload returns the "file" field of a multipart request.
func readUpload(w http.ResponseWriter, r *http.Request) (string, []byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxUpload+64<<10)
	if err := r.ParseMultipartForm(MaxUpload); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			return "", nil, &apperr.Error{Status: http.StatusRequestEntityTooLarge, Message: "Fișierul depășește 5 MB."}
		}
		return "", nil, apperr.Invalid("file", "Cererea nu conține un fișier valid.")
	}
	defer r.MultipartForm.RemoveAll()
	f, hdr, err := r.FormFile("file")
	if err != nil {
		return "", nil, apperr.Invalid("file", "Alegeți un fișier.")
	}
	defer f.Close()
	if hdr.Size > MaxUpload {
		return "", nil, &apperr.Error{Status: http.StatusRequestEntityTooLarge, Message: "Fișierul depășește 5 MB."}
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxUpload+1))
	if err != nil {
		return "", nil, apperr.Invalid("file", "Fișierul nu a putut fi citit.")
	}
	if len(data) > MaxUpload {
		return "", nil, &apperr.Error{Status: http.StatusRequestEntityTooLarge, Message: "Fișierul depășește 5 MB."}
	}
	return hdr.Filename, data, nil
}

// importPreview validates an uploaded file against the current locations.
func (a *admin) importPreview(w http.ResponseWriter, r *http.Request) {
	p, _, err := a.buildImport(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, p)
}

func (a *admin) buildImport(w http.ResponseWriter, r *http.Request) (importer.Preview, []domain.LocationInput, error) {
	name, data, err := readUpload(w, r)
	if err != nil {
		return importer.Preview{}, nil, err
	}
	t, err := importer.Parse(name, data)
	if err != nil {
		return importer.Preview{}, nil, err
	}
	locs, err := a.d.Store.ListLocations(r.Context(), false)
	if err != nil {
		return importer.Preview{}, nil, err
	}
	existing := make(map[string]bool, len(locs))
	for _, l := range locs {
		existing[importer.Fold(l.Name)] = true
	}
	p, ins := importer.Build(t, existing, a.d.Settings.Get())
	return p, ins, nil
}

// importApply re-validates the same file and saves it in one transaction.
func (a *admin) importApply(w http.ResponseWriter, r *http.Request) {
	p, ins, err := a.buildImport(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	if !p.Valid {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Fișierul conține erori; verificați previzualizarea.", "field": "file", "preview": p})
		return
	}
	if _, err := a.d.Store.SaveLocations(r.Context(), ins, a.d.Now()); err != nil {
		writeError(w, err)
		return
	}
	if err := a.d.Monitor.Reload(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	devices := 0
	for _, in := range ins {
		devices += len(in.Devices)
	}
	writeJSON(w, 200, map[string]int{"locations": len(ins), "devices": devices})
}
