// 24-hour location timelines for the cards, refreshed periodically and
// shortly after a state change.

import { get } from './api';
import { live } from './live.svelte';
import type { Segment } from './types';

interface TimelinesResponse {
  from: number;
  to: number;
  locations: Record<string, Segment[]>;
}

class Timelines {
  data = $state<Record<string, Segment[]>>({});
  from = $state(0);
  to = $state(0);
  private users = 0;
  private timer: ReturnType<typeof setInterval> | null = null;
  private debounce: ReturnType<typeof setTimeout> | null = null;
  private off: (() => void) | null = null;

  async load(): Promise<void> {
    try {
      const r = await get<TimelinesResponse>('/api/timelines');
      this.data = r.locations;
      this.from = r.from;
      this.to = r.to;
    } catch {
      /* keep the previous data; the connection badge reports the outage */
    }
  }

  /** Segments for a location, with the open end extended to "now". */
  for(id: number, now: number): { segments: Segment[]; from: number; to: number } {
    const segs = this.data[String(id)] ?? [];
    if (!segs.length) return { segments: segs, from: now - 86_400_000, to: now };
    const last = segs[segs.length - 1];
    const l = live.location(id);
    const tail: Segment[] = [...segs];
    if (now > last.end) tail.push({ start: last.end, end: now, state: l?.state ?? last.state });
    return { segments: tail, from: now - 86_400_000, to: now };
  }

  acquire(): () => void {
    if (this.users++ === 0) {
      this.load();
      this.timer = setInterval(() => this.load(), 60_000);
      this.off = live.onChange(() => {
        if (this.debounce) clearTimeout(this.debounce);
        this.debounce = setTimeout(() => this.load(), 5000);
      });
    }
    return () => {
      if (--this.users > 0) return;
      if (this.timer) clearInterval(this.timer);
      if (this.debounce) clearTimeout(this.debounce);
      this.off?.();
      this.timer = this.debounce = null;
      this.off = null;
    };
  }
}

export const timelines = new Timelines();
