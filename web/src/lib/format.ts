const num1 = new Intl.NumberFormat('ro-RO', { maximumFractionDigits: 1 });

export function fmtBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${i === 0 ? Math.round(v) : num1.format(v)} ${units[i]}`;
}

const AUDIT: Record<string, string> = {
  settings: 'Setări modificate',
  identity: 'Identitate modificată',
  backup: 'Backup manual',
  save_location: 'Locație salvată',
  archive_location: 'Locație arhivată',
  locations_order: 'Ordinea locațiilor modificată',
  setup_password: 'Parolă inițială setată',
  change_password: 'Parolă schimbată',
  set_password_cli: 'Parolă setată din linia de comandă',
  import: 'Import de locații',
  logo: 'Logo încărcat',
  logo_delete: 'Logo eliminat',
  enable_device_auto: 'Echipament reactivat automat',
};

export function auditAction(a: string): string {
  return AUDIT[a] ?? a;
}

const BACKUP: Record<string, string> = {
  auto: 'automat',
  manual: 'manual',
  'pre-migrate': 'înainte de migrare',
  'pre-restore': 'înainte de restaurare',
};

export function backupKind(k: string): string {
  return BACKUP[k] ?? k;
}

const ICMP: Record<string, string> = {
  windows: 'Windows (IcmpSendEcho)',
  'linux-unprivileged': 'Linux neprivilegiat (ping socket)',
  'linux-raw': 'Linux raw socket',
  unavailable: 'Indisponibil — lipsă permisiune',
  fake: 'Simulat',
};

export function icmpMode(m: string): string {
  return ICMP[m] ?? (m || '—');
}
