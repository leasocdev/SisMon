CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);

CREATE TABLE locations(
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  address TEXT NOT NULL DEFAULT '',
  lat REAL, lon REAL,
  notes TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  archived_at INTEGER
);
CREATE UNIQUE INDEX locations_name_active ON locations(name COLLATE NOCASE) WHERE archived_at IS NULL;

CREATE TABLE devices(
  id INTEGER PRIMARY KEY,
  location_id INTEGER NOT NULL REFERENCES locations(id),
  name TEXT NOT NULL,
  host TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'Altele',
  is_primary INTEGER NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL DEFAULT 0,
  interval_s INTEGER, failures INTEGER, recoveries INTEGER,
  rev INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  archived_at INTEGER
);
CREATE INDEX devices_location ON devices(location_id);
CREATE UNIQUE INDEX devices_host_active ON devices(location_id, lower(host)) WHERE archived_at IS NULL;

CREATE TABLE device_status(
  device_id INTEGER PRIMARY KEY REFERENCES devices(id),
  state TEXT NOT NULL DEFAULT 'unknown',
  pending TEXT NOT NULL DEFAULT '',
  streak INTEGER NOT NULL DEFAULT 0,
  streak_start INTEGER,
  since INTEGER,
  last_check_at INTEGER,
  latency_ms REAL,
  loss_pct REAL,
  error_code TEXT NOT NULL DEFAULT '',
  dns_warning INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE state_intervals(
  id INTEGER PRIMARY KEY,
  entity TEXT NOT NULL CHECK(entity IN ('device','location')),
  entity_id INTEGER NOT NULL,
  state TEXT NOT NULL CHECK(state IN ('up','down','degraded','unknown','error')),
  start_at INTEGER NOT NULL,
  end_at INTEGER,
  CHECK(end_at IS NULL OR end_at >= start_at)
);
CREATE INDEX state_intervals_lookup ON state_intervals(entity, entity_id, start_at);
CREATE UNIQUE INDEX state_intervals_open ON state_intervals(entity, entity_id) WHERE end_at IS NULL;

CREATE TABLE samples(
  device_id INTEGER NOT NULL,
  ts INTEGER NOT NULL,
  ok INTEGER NOT NULL,
  latency_ms REAL,
  loss_pct REAL NOT NULL,
  error_code TEXT NOT NULL DEFAULT ''
);
CREATE INDEX samples_device_ts ON samples(device_id, ts);

CREATE TABLE samples_hourly(
  device_id INTEGER NOT NULL,
  hour_ts INTEGER NOT NULL,
  checks INTEGER NOT NULL,
  ok_checks INTEGER NOT NULL,
  lat_avg REAL, lat_min REAL, lat_max REAL,
  loss_avg REAL NOT NULL,
  PRIMARY KEY(device_id, hour_ts)
) WITHOUT ROWID;

CREATE TABLE incidents(
  id INTEGER PRIMARY KEY,
  device_id INTEGER NOT NULL REFERENCES devices(id),
  observed_at INTEGER NOT NULL,
  confirmed_at INTEGER NOT NULL,
  ended_at INTEGER,
  end_reason TEXT
);
CREATE INDEX incidents_device ON incidents(device_id, observed_at);
CREATE INDEX incidents_observed ON incidents(observed_at);
CREATE UNIQUE INDEX incidents_open ON incidents(device_id) WHERE ended_at IS NULL;

CREATE TABLE audit(
  id INTEGER PRIMARY KEY,
  ts INTEGER NOT NULL,
  action TEXT NOT NULL,
  entity TEXT NOT NULL DEFAULT '',
  entity_id INTEGER,
  details_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_ts ON audit(ts);

CREATE TABLE sessions(
  token_hash BLOB PRIMARY KEY,
  created_at INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL,
  csrf TEXT NOT NULL
) WITHOUT ROWID;
