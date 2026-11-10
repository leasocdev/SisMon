package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"netwatch/internal/apperr"
	"netwatch/internal/domain"
	"netwatch/internal/settings"
	"netwatch/internal/state"
)

// Device is a configured device.
type Device struct {
	ID         int64              `json:"id"`
	LocationID int64              `json:"location_id"`
	Rev        int64              `json:"rev"`
	Name       string             `json:"name"`
	Host       string             `json:"host"`
	Kind       string             `json:"kind"`
	Primary    bool               `json:"is_primary"`
	SortOrder  int                `json:"sort_order"`
	Overrides  settings.Overrides `json:"overrides"`
	CreatedAt  time.Time          `json:"-"`
	ArchivedAt *time.Time         `json:"-"`
}

// Location is a configured location with its devices.
type Location struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Address    string     `json:"address"`
	Notes      string     `json:"notes"`
	Lat        *float64   `json:"lat"`
	Lon        *float64   `json:"lon"`
	SortOrder  int        `json:"sort_order"`
	CreatedAt  time.Time  `json:"-"`
	ArchivedAt *time.Time `json:"-"`
	Devices    []Device   `json:"devices"`
}

// SaveResult reports what SaveLocation changed.
type SaveResult struct {
	LocationID  int64
	Added       []int64
	HostChanged []int64
	Archived    []int64
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func intPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	i := int(v.Int64)
	return &i
}

const deviceCols = `id,location_id,rev,name,host,kind,is_primary,sort_order,interval_s,failures,recoveries,created_at,archived_at`

func scanDevice(sc interface{ Scan(...any) error }) (Device, error) {
	var d Device
	var iv, f, r, arch sql.NullInt64
	var created int64
	err := sc.Scan(&d.ID, &d.LocationID, &d.Rev, &d.Name, &d.Host, &d.Kind, &d.Primary, &d.SortOrder, &iv, &f, &r, &created, &arch)
	d.Overrides = settings.Overrides{IntervalS: intPtr(iv), Failures: intPtr(f), Recoveries: intPtr(r)}
	d.CreatedAt, d.ArchivedAt = FromMs(created), timePtr(arch)
	return d, err
}

// ListDevices returns devices, optionally including archived ones.
func ListDevices(ctx context.Context, q Querier, includeArchived bool) ([]Device, error) {
	where := ` WHERE archived_at IS NULL`
	if includeArchived {
		where = ``
	}
	rows, err := q.QueryContext(ctx, `SELECT `+deviceCols+` FROM devices`+where+` ORDER BY location_id,sort_order,id`)
	if err != nil {
		return nil, wrap("list devices", err)
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListLocations returns locations with their devices.
func (s *Store) ListLocations(ctx context.Context, includeArchived bool) ([]Location, error) {
	where := ` WHERE archived_at IS NULL`
	if includeArchived {
		where = ``
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,address,notes,lat,lon,sort_order,created_at,archived_at FROM locations`+where+` ORDER BY sort_order,name COLLATE NOCASE,id`)
	if err != nil {
		return nil, wrap("list locations", err)
	}
	var out []Location
	idx := map[int64]int{}
	for rows.Next() {
		var l Location
		var lat, lon sql.NullFloat64
		var created int64
		var arch sql.NullInt64
		if err := rows.Scan(&l.ID, &l.Name, &l.Address, &l.Notes, &lat, &lon, &l.SortOrder, &created, &arch); err != nil {
			rows.Close()
			return nil, err
		}
		if lat.Valid && lon.Valid {
			l.Lat, l.Lon = &lat.Float64, &lon.Float64
		}
		l.CreatedAt, l.ArchivedAt = FromMs(created), timePtr(arch)
		l.Devices = []Device{}
		idx[l.ID] = len(out)
		out = append(out, l)
	}
	rows.Close()
	devs, err := ListDevices(ctx, s.DB, includeArchived)
	if err != nil {
		return nil, err
	}
	for _, d := range devs {
		if i, ok := idx[d.LocationID]; ok {
			out[i].Devices = append(out[i].Devices, d)
		}
	}
	return out, nil
}

// GetLocation returns one location (archived included).
func (s *Store) GetLocation(ctx context.Context, id int64) (Location, error) {
	all, err := s.ListLocations(ctx, true)
	if err != nil {
		return Location{}, err
	}
	for _, l := range all {
		if l.ID == id {
			return l, nil
		}
	}
	return Location{}, apperr.NotFound("Locația nu există.")
}

// Audit records an administrative action.
func (s *Store) Audit(ctx context.Context, q Querier, action, entity string, id int64, details any, now time.Time) error {
	b, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO audit(ts,action,entity,entity_id,details_json) VALUES(?,?,?,?,?)`, Ms(now), action, entity, id, string(b))
	return wrap("audit", err)
}

func endIncident(ctx context.Context, tx *sql.Tx, deviceID int64, at time.Time, reason string) error {
	_, err := tx.ExecContext(ctx, `UPDATE incidents SET ended_at=MAX(observed_at,?),end_reason=? WHERE device_id=? AND ended_at IS NULL`, Ms(at), reason, deviceID)
	return wrap("end incident", err)
}

func resetStatus(ctx context.Context, tx *sql.Tx, deviceID int64, at time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO device_status(device_id,state,since) VALUES(?,?,?)`, deviceID, state.Unknown, Ms(at))
	return wrap("device status", err)
}

func archiveDevice(ctx context.Context, tx *sql.Tx, id int64, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE devices SET archived_at=?,rev=rev+1 WHERE id=?`, Ms(now), id); err != nil {
		return wrap("archive device", err)
	}
	if err := endIncident(ctx, tx, id, now, "archived"); err != nil {
		return err
	}
	return CloseOpen(ctx, tx, EntityDevice, id, now)
}

// SaveLocation creates (id==nil) or updates a location. in must be normalized.
func (s *Store) SaveLocation(ctx context.Context, id *int64, in domain.LocationInput, now time.Time) (SaveResult, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return SaveResult{}, err
	}
	defer tx.Rollback()
	res, err := s.saveLocationTx(ctx, tx, id, in, now)
	if err != nil {
		return res, err
	}
	return res, tx.Commit()
}

// SaveLocations creates several new locations in one transaction (import):
// any error leaves the database unchanged. Each input must be normalized.
func (s *Store) SaveLocations(ctx context.Context, ins []domain.LocationInput, now time.Time) ([]SaveResult, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := make([]SaveResult, 0, len(ins))
	devices := 0
	for _, in := range ins {
		res, err := s.saveLocationTx(ctx, tx, nil, in, now)
		if err != nil {
			return nil, err
		}
		out = append(out, res)
		devices += len(in.Devices)
	}
	if err := s.Audit(ctx, tx, "import", "location", 0, map[string]int{"locations": len(ins), "devices": devices}, now); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

func (s *Store) saveLocationTx(ctx context.Context, tx *sql.Tx, id *int64, in domain.LocationInput, now time.Time) (SaveResult, error) {
	var res SaveResult
	var clash int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM locations WHERE archived_at IS NULL AND name=? COLLATE NOCASE AND id<>?`, in.Name, derefID(id)).Scan(&clash)
	if err == nil {
		return res, apperr.Conflict("name", "Există deja o locație cu această denumire.")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return res, err
	}
	if id == nil {
		r, err := tx.ExecContext(ctx, `INSERT INTO locations(name,address,notes,lat,lon,created_at,sort_order) VALUES(?,?,?,?,?,?,(SELECT COALESCE(MAX(sort_order),-1)+1 FROM locations WHERE archived_at IS NULL))`, in.Name, in.Address, in.Notes, in.Lat, in.Lon, Ms(now))
		if err != nil {
			return res, wrap("insert location", err)
		}
		res.LocationID, _ = r.LastInsertId()
	} else {
		r, err := tx.ExecContext(ctx, `UPDATE locations SET name=?,address=?,notes=?,lat=?,lon=? WHERE id=? AND archived_at IS NULL`, in.Name, in.Address, in.Notes, in.Lat, in.Lon, *id)
		if err != nil {
			return res, wrap("update location", err)
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return res, apperr.NotFound("Locația nu există.")
		}
		res.LocationID = *id
	}
	old := map[int64]Device{}
	rows, err := tx.QueryContext(ctx, `SELECT `+deviceCols+` FROM devices WHERE location_id=? AND archived_at IS NULL`, res.LocationID)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			rows.Close()
			return res, err
		}
		old[d.ID] = d
	}
	rows.Close()
	kept := map[int64]bool{}
	// Archive removed devices first so their hosts can be reused by new rows.
	for _, d := range in.Devices {
		if d.ID != nil {
			if _, ok := old[*d.ID]; !ok {
				return res, apperr.Invalid("devices", "Echipamentul nu aparține acestei locații.")
			}
			kept[*d.ID] = true
		}
	}
	for oid := range old {
		if !kept[oid] {
			if err := archiveDevice(ctx, tx, oid, now); err != nil {
				return res, err
			}
			res.Archived = append(res.Archived, oid)
		}
	}
	// Park changed hosts on placeholders so swaps don't trip the unique index.
	for _, d := range in.Devices {
		if d.ID != nil && d.Host != old[*d.ID].Host {
			if _, err := tx.ExecContext(ctx, `UPDATE devices SET host='~'||id WHERE id=?`, *d.ID); err != nil {
				return res, err
			}
		}
	}
	for pos, d := range in.Devices {
		o := d.Overrides
		if d.ID == nil {
			r, err := tx.ExecContext(ctx, `INSERT INTO devices(location_id,name,host,kind,is_primary,sort_order,interval_s,failures,recoveries,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
				res.LocationID, d.Name, d.Host, d.Kind, d.Primary, pos, nullInt(o.IntervalS), nullInt(o.Failures), nullInt(o.Recoveries), Ms(now))
			if err != nil {
				return res, wrap("insert device", err)
			}
			did, _ := r.LastInsertId()
			if err := resetStatus(ctx, tx, did, now); err != nil {
				return res, err
			}
			if err := CloseAndOpen(ctx, tx, EntityDevice, did, state.Unknown, now); err != nil {
				return res, err
			}
			res.Added = append(res.Added, did)
			continue
		}
		did := *d.ID
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET name=?,host=?,kind=?,is_primary=?,sort_order=?,interval_s=?,failures=?,recoveries=? WHERE id=?`,
			d.Name, d.Host, d.Kind, d.Primary, pos, nullInt(o.IntervalS), nullInt(o.Failures), nullInt(o.Recoveries), did); err != nil {
			return res, wrap("update device", err)
		}
		if d.Host != old[did].Host {
			if _, err := tx.ExecContext(ctx, `UPDATE devices SET rev=rev+1 WHERE id=?`, did); err != nil {
				return res, err
			}
			if err := endIncident(ctx, tx, did, now, "configuration"); err != nil {
				return res, err
			}
			if err := resetStatus(ctx, tx, did, now); err != nil {
				return res, err
			}
			if err := CloseAndOpen(ctx, tx, EntityDevice, did, state.Unknown, now); err != nil {
				return res, err
			}
			res.HostChanged = append(res.HostChanged, did)
		}
	}
	if _, err := RecomputeLocation(ctx, tx, res.LocationID, now); err != nil {
		return res, err
	}
	if err := s.Audit(ctx, tx, "save_location", "location", res.LocationID, in, now); err != nil {
		return res, err
	}
	return res, nil
}

func derefID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// ArchiveLocation archives a location and all its devices.
func (s *Store) ArchiveLocation(ctx context.Context, id int64, now time.Time) (SaveResult, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return SaveResult{}, err
	}
	defer tx.Rollback()
	res := SaveResult{LocationID: id}
	r, err := tx.ExecContext(ctx, `UPDATE locations SET archived_at=? WHERE id=? AND archived_at IS NULL`, Ms(now), id)
	if err != nil {
		return res, err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return res, apperr.NotFound("Locația nu există sau este deja arhivată.")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM devices WHERE location_id=? AND archived_at IS NULL`, id)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var did int64
		rows.Scan(&did)
		res.Archived = append(res.Archived, did)
	}
	rows.Close()
	for _, did := range res.Archived {
		if err := archiveDevice(ctx, tx, did, now); err != nil {
			return res, err
		}
	}
	if err := CloseOpen(ctx, tx, EntityLocation, id, now); err != nil {
		return res, err
	}
	if err := s.Audit(ctx, tx, "archive_location", "location", id, map[string]any{"devices": res.Archived}, now); err != nil {
		return res, err
	}
	return res, tx.Commit()
}

// SetLocationOrder sets the manual order of active locations: sort_order is
// the index in ids. Unknown or archived ids are rejected.
func (s *Store) SetLocationOrder(ctx context.Context, ids []int64, now time.Time) error {
	if len(ids) == 0 || len(ids) > 10000 {
		return apperr.Invalid("ids", "Lista locațiilor este goală sau prea lungă.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	seen := map[int64]bool{}
	for i, id := range ids {
		if seen[id] {
			return apperr.Invalid("ids", "Locație repetată în listă.")
		}
		seen[id] = true
		r, err := tx.ExecContext(ctx, `UPDATE locations SET sort_order=? WHERE id=? AND archived_at IS NULL`, i, id)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return apperr.Invalid("ids", "Locație inexistentă sau arhivată în listă.")
		}
	}
	if err := s.Audit(ctx, tx, "locations_order", "location", 0, map[string]any{"ids": ids}, now); err != nil {
		return err
	}
	return tx.Commit()
}
