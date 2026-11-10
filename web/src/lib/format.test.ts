import { describe, expect, it } from 'vitest';
import { auditAction, backupKind, fmtBytes, icmpMode } from './format';

describe('format', () => {
  it('fmtBytes', () => {
    expect(fmtBytes(0)).toBe('0 B');
    expect(fmtBytes(1536)).toBe('1,5 KB');
    expect(fmtBytes(215 * 1024 * 1024)).toBe('215 MB');
    expect(fmtBytes(3.2 * 1024 ** 3)).toBe('3,2 GB');
  });
  it('labels', () => {
    expect(auditAction('settings')).toBe('Setări modificate');
    expect(auditAction('save_location')).toBe('Locație salvată');
    expect(auditAction('necunoscut')).toBe('necunoscut');
    expect(backupKind('auto')).toBe('automat');
    expect(backupKind('pre-migrate')).toBe('înainte de migrare');
    expect(icmpMode('fake')).toBe('Simulat');
    expect(icmpMode('unavailable')).toBe('Indisponibil — lipsă permisiune');
  });
});
