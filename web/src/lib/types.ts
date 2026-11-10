// JSON shapes of the NetWatch API (field names mirror the Go json tags).

export type State = 'up' | 'down' | 'degraded' | 'unknown' | 'error';

export interface DeviceView {
  id: number;
  location_id: number;
  name: string;
  host: string;
  kind: string;
  is_primary: boolean;
  state: State;
  pending: State | '';
  since: number;
  last_check: number | null;
  latency_ms: number | null;
  loss_pct: number | null;
  error_code: string;
  dns_warning: boolean;
}

export interface LocationView {
  id: number;
  name: string;
  address: string;
  notes: string;
  lat: number | null;
  lon: number | null;
  sort_order: number;
  state: State;
  incomplete: boolean;
  since: number;
  devices: DeviceView[];
}

export interface Health {
  last_cycle: number | null;
  late_checks: number;
  avg_check_ms: number;
  icmp_mode: string;
  errors: string[] | null;
  db_size_bytes: number;
  last_backup: number | null;
  backup_error: string;
  warning: string;
}

export interface Snapshot {
  epoch: number;
  seq: number;
  locations: LocationView[];
  health: Health;
}

export interface Identity {
  name: string;
  subtitle: string;
  report_title: string;
  logo_version: number;
}

export interface StateResponse extends Snapshot {
  identity: Identity;
  interval_s: number;
  version: string;
  now: number;
}

export interface Segment {
  start: number;
  end: number;
  state: State;
}

export interface Point {
  t: number;
  avg: number | null;
  min: number | null;
  max: number | null;
  loss_pct: number | null;
}

export interface Durations {
  up: number;
  down: number;
  degraded: number;
  unknown: number;
  error: number;
}

export interface DeviceRow {
  id: number;
  location_id: number;
  location: string;
  name: string;
  host: string;
  kind: string;
  is_primary: boolean;
  archived: boolean;
  durations: Durations;
  availability: number | null;
  coverage: number | null;
  latency_ms: number | null;
  loss_pct: number | null;
  incidents: number;
  mttr_s: number | null;
}

export interface LocationRow {
  id: number;
  name: string;
  archived: boolean;
  durations: Durations;
  availability: number | null;
  coverage: number | null;
}

export interface IncidentRow {
  id: number;
  device_id: number;
  location_id: number;
  location: string;
  device: string;
  observed_at: number;
  confirmed_at: number;
  ended_at: number | null;
  reason: string;
  reason_text: string;
  duration_s: number;
}

export interface Report {
  from: number;
  to: number;
  summary: {
    availability: number | null;
    coverage: number | null;
    incidents: number;
    down_s: number;
    mttr_s: number | null;
  };
  devices: DeviceRow[];
  locations: LocationRow[];
  incidents: IncidentRow[];
}

export interface Report24 {
  from: number;
  to: number;
  locations: number;
  affected_now: number;
  current: { location_id: number; location: string; device_id: number; device: string; since: number }[];
  incidents: IncidentRow[];
}

export interface CatalogLocation {
  id: number;
  name: string;
  archived: boolean;
  devices: { id: number; name: string; host: string; kind: string; is_primary: boolean; archived: boolean }[];
}

export interface Settings {
  interval_s: number;
  packets: number;
  timeout_s: number;
  failures: number;
  recoveries: number;
  stale_s: number;
  incidents_for: 'all' | 'primary';
  prefer_ipv4: boolean;
  backup_hour: number;
  backup_secondary_dir: string;
}

export interface Overrides {
  interval_s?: number;
  failures?: number;
  recoveries?: number;
}

export interface AdminDevice {
  id: number;
  location_id: number;
  rev: number;
  name: string;
  host: string;
  kind: string;
  is_primary: boolean;
  sort_order: number;
  overrides: Overrides;
}

export interface AdminLocation {
  id: number;
  name: string;
  address: string;
  notes: string;
  lat: number | null;
  lon: number | null;
  sort_order: number;
  devices: AdminDevice[];
}

export interface DeviceInput {
  id?: number;
  name: string;
  host: string;
  kind: string;
  is_primary: boolean;
  overrides: Overrides;
}

export interface LocationInput {
  name: string;
  address: string;
  notes: string;
  lat: number | null;
  lon: number | null;
  devices: DeviceInput[];
}

export interface BackupInfo {
  name: string;
  kind: string;
  created: number;
  size_bytes: number;
}

export interface AuditEntry {
  id: number;
  ts: number;
  action: string;
  entity: string;
  entity_id: number | null;
  details: string;
}

export interface SessionInfo {
  authenticated: boolean;
  configured: boolean;
  setup_allowed: boolean;
  tls: boolean;
  csrf?: string;
}

export interface TestResult {
  ok: boolean;
  sent: number;
  received: number;
  latency_ms: number | null;
  code: string;
  message: string;
}
