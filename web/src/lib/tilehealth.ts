// Decides when the map should switch from OSM tiles to the local contours.

const WINDOW_MS = 30_000;
const MAX_EVENTS = 20;
const MIN_FAILURES = 6;

export class TileHealth {
  private events: { ok: boolean; at: number }[] = [];

  record(ok: boolean, now: number): void {
    this.events.push({ ok, at: now });
    if (this.events.length > MAX_EVENTS) this.events.shift();
  }

  shouldFallback(now: number): boolean {
    const recent = this.events.filter((e) => now - e.at <= WINDOW_MS);
    const failed = recent.filter((e) => !e.ok).length;
    return failed >= MIN_FAILURES && failed > recent.length / 2;
  }

  reset(): void {
    this.events = [];
  }
}
