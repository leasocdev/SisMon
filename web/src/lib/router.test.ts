import { describe, expect, it } from 'vitest';
import { match } from './router.svelte';

describe('match', () => {
  it('matches location panel', () => {
    expect(match('/locatie/12')).toMatchObject({ name: 'location', params: { id: '12' } });
  });
  it('matches admin location editor (new)', () => {
    expect(match('/administrare/locatie/nou')).toMatchObject({ name: 'adminLocation', params: { id: 'nou' } });
  });
  it('matches admin pages', () => {
    expect(match('/administrare/setari')).toMatchObject({ name: 'adminPage', params: { page: 'setari' } });
    expect(match('/administrare/necunoscut').name).toBe('notFound');
  });
  it('parses the query', () => {
    const r = match('/statistici?from=1&to=2');
    expect(r.name).toBe('stats');
    expect(r.query.from).toBe('1');
    expect(r.query.to).toBe('2');
  });
  it('fixed routes and unknown paths', () => {
    expect(match('/').name).toBe('monitor');
    expect(match('/dispecerat').name).toBe('dispatch');
    expect(match('/harta').name).toBe('map');
    expect(match('/raport-24h').name).toBe('report');
    expect(match('/administrare').name).toBe('admin');
    expect(match('/x/y').name).toBe('notFound');
    expect(match('/locatie/abc').name).toBe('notFound');
  });
});
