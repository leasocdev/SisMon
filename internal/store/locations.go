package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"sismon/internal/apperr"
	"sismon/internal/domain"
	"sismon/internal/settings"
	"sismon/internal/state"
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
	// Disabled devices are kept in the configuration but not monitored.
	Disabled      bool       `json:"disabled"`
	DisabledAt    *int64     `json:"disabled_at"`
	DisabledUntil *int64     `json:"disabled_until"`
	CreatedAt     time.Time  `json:"-"`
	ArchivedAt    *time.Time `json:"-"`
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
	Disabled    []int64
	Enabled     []int64
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

const deviceCols = `id,location_id,rev,name,host,kind,is_primary,sort_order,interval_s,failures,recoveries,created_at,archived_at,disabled_at,disabled_until`

func scanDevice(sc interface{ Scan(...any) error }) (Device, error) {
	var d Device
	var iv, f, r, arch, dis, until sql.NullInt64
	var created int64
	err := sc.Scan(&d.ID, &d.LocationID, &d.Rev, &d.Name, &d.Host, &d.Kind, &d.Primary, &d.SortOrder, &iv, &f, &r, &created, &arch, &dis, &until)
	d.Overrides = settings.Overrides{IntervalS: intPtr(iv), Failures: intPtr(f), Recoveries: intPtr(r)}
	d.CreatedAt, d.ArchivedAt = FromMs(created), timePtr(arch)
	d.DisabledAt, d.DisabledUntil = i64(dis), i64(until)
	d.Disabled = d.DisabledAt != nil
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

// disableDevice stops monitoring a device: its open incident and state
// interval end (the time it stays disabled is a gap, left out of statistics)
// and its status restarts from unknown when it is enabled again.
func disableDevice(ctx context.Context, tx *sql.Tx, id int64, until *int64, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE devices SET disabled_at=?,disabled_until=?,rev=rev+1 WHERE id=?`, Ms(now), until, id); err != nil {
		return wrap("disable device", err)
	}
	if err := endIncident(ctx, tx, id, now, "disabled"); err != nil {
		return err
	}
	if err := resetStatus(ctx, tx, id, now); err != nil {
		return err
	}
	return CloseOpen(ctx, tx, EntityDevice, id, now)
}

// enableDevice resumes monitoring a disabled device from the unknown state.
func enableDevice(ctx context.Context, tx *sql.Tx, id int64, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE devices SET disabled_at=NULL,disabled_until=NULL,rev=rev+1 WHERE id=?`, id); err != nil {
		return wrap("enable device", err)
	}
	if err := resetStatus(ctx, tx, id, now); err != nil {
		return err
	}
	return CloseAndOpen(ctx, tx, EntityDevice, id, state.Unknown, now)
}

func eqPtr(a, b *int64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

// EnableExpired re-enables the devices whose disabled_until has passed and
// returns how many there were.
func (s *Store) EnableExpired(ctx context.Context, now time.Time) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,location_id FROM devices WHERE archived_at IS NULL AND disabled_at IS NOT NULL AND disabled_until<=?`, Ms(now))
	if err != nil {
		return 0, wrap("expired disabled devices", err)
	}
	type dev struct{ id, loc int64 }
	var due []dev
	for rows.Next() {
		var d dev
		if err := rows.Scan(&d.id, &d.loc); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, d)
	}
	rows.Close()
	if len(due) == 0 {
		return 0, nil
	}
	locs := map[int64]bool{}
	for _, d := range due {
		if err := enableDevice(ctx, tx, d.id, now); err != nil {
			return 0, err
		}
		if err := s.Audit(ctx, tx, "enable_device_auto", "device", d.id, map[string]int64{"location_id": d.loc}, now); err != nil {
			return 0, err
		}
		locs[d.loc] = true
	}
	for l := range locs {
		if _, err := RecomputeLocation(ctx, tx, l, now); err != nil {
			return 0, err
		}
	}
	return len(due), tx.Commit()
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
		if d.Disabled && d.DisabledUntil != nil && *d.DisabledUntil <= Ms(now) {
			// An unchanged date that has just passed is left to EnableExpired.
			if d.ID == nil || old[*d.ID].DisabledAt == nil || !eqPtr(old[*d.ID].DisabledUntil, d.DisabledUntil) {
				return res, apperr.Invalid(fmt.Sprintf("devices.%d.disabled_until", pos), "Data reactivării trebuie să fie în viitor.")
			}
		}
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
			if d.Disabled {
				if err := disableDevice(ctx, tx, did, d.DisabledUntil, now); err != nil {
					return res, err
				}
				res.Disabled = append(res.Disabled, did)
			}
			continue
		}
		did := *d.ID
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET name=?,host=?,kind=?,is_primary=?,sort_order=?,interval_s=?,failures=?,recoveries=? WHERE id=?`,
			d.Name, d.Host, d.Kind, d.Primary, pos, nullInt(o.IntervalS), nullInt(o.Failures), nullInt(o.Recoveries), did); err != nil {
			return res, wrap("update device", err)
		}
		wasDisabled := old[did].DisabledAt != nil
		switch {
		case d.Disabled && !wasDisabled:
			if err := disableDevice(ctx, tx, did, d.DisabledUntil, now); err != nil {
				return res, err
			}
			res.Disabled = append(res.Disabled, did)
		case !d.Disabled && wasDisabled:
			if err := enableDevice(ctx, tx, did, now); err != nil {
				return res, err
			}
			res.Enabled = append(res.Enabled, did)
		case d.Disabled:
			if _, err := tx.ExecContext(ctx, `UPDATE devices SET disabled_until=? WHERE id=?`, d.DisabledUntil, did); err != nil {
				return res, wrap("update device", err)
			}
		}
		hostChanged := d.Host != old[did].Host
		if hostChanged && (d.Disabled || wasDisabled) {
			// Disabled: nothing is monitored, only drop in-flight results.
			// Just enabled: enableDevice already restarted it from unknown.
			if d.Disabled {
				if _, err := tx.ExecContext(ctx, `UPDATE devices SET rev=rev+1 WHERE id=?`, did); err != nil {
					return res, err
				}
			}
			res.HostChanged = append(res.HostChanged, did)
		} else if hostChanged {
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
			return apperr.Invalid("ids", "Lista nu corespunde locațiilor active; reîncărcați pagina.")
		}
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM locations WHERE archived_at IS NULL`).Scan(&active); err != nil {
		return err
	}
	if active != len(ids) {
		return apperr.Invalid("ids", "Lista nu corespunde locațiilor active; reîncărcați pagina.")
	}
	if err := s.Audit(ctx, tx, "locations_order", "location", 0, map[string]any{"ids": ids}, now); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteLocation permanently removes a location (active or archived), all its
// devices (archived ones included) and their whole history: samples, hourly
// aggregates, incidents, statuses and state intervals. It cannot be undone;
// only the audit entry remains. It returns the ids of the removed devices.
func (s *Store) DeleteLocation(ctx context.Context, id int64, now time.Time) ([]int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var removed []int64
	var name string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM locations WHERE id=?`, id).Scan(&name); errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("Locația nu există.")
	} else if err != nil {
		return nil, wrap("delete location", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM devices WHERE location_id=?`, id)
	if err != nil {
		return nil, wrap("delete location", err)
	}
	for rows.Next() {
		var did int64
		if err := rows.Scan(&did); err != nil {
			rows.Close()
			return nil, err
		}
		removed = append(removed, did)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	const devs = `(SELECT id FROM devices WHERE location_id=?)`
	for _, q := range []string{
		`DELETE FROM samples WHERE device_id IN ` + devs,
		`DELETE FROM samples_hourly WHERE device_id IN ` + devs,
		`DELETE FROM incidents WHERE device_id IN ` + devs,
		`DELETE FROM device_status WHERE device_id IN ` + devs,
		`DELETE FROM state_intervals WHERE entity='device' AND entity_id IN ` + devs,
		`DELETE FROM state_intervals WHERE entity='location' AND entity_id=?`,
		`DELETE FROM devices WHERE location_id=?`,
		`DELETE FROM locations WHERE id=?`,
	} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return nil, wrap("delete location", err)
		}
	}
	if err := s.Audit(ctx, tx, "delete_location", "location", id, map[string]any{"name": name, "devices": removed}, now); err != nil {
		return nil, err
	}
	return removed, tx.Commit()
}
