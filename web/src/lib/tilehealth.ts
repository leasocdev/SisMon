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

/** Retries for one tile before it counts as failed. */
export const TILE_RETRIES = 3;

/** Delay before retry number `attempt` (1-based): exponential with jitter, so
 * a burst of failed tiles is not retried all at once. The proxy answers 503
 * right away when its download pool is full, and a synchronized retry burst
 * would just fill it again. `rand` is in [0, 1). */
export function retryDelay(attempt: number, rand: number): number {
  const base = 1000 * 2 ** (attempt - 1);
  return Math.round(base * (0.75 + rand));
}
