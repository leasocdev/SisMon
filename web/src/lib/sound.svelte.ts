// Browser sound alerts (Web Audio, no files).

import type { State } from './types';

export interface SoundPrefs {
  on: boolean;
  degraded: boolean;
}

export type SoundKind = 'alarm' | 'recovery';

const KEY = 'nw-sound';

export function loadPrefs(): SoundPrefs {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) || 'null');
    if (v && typeof v.on === 'boolean') return { on: v.on, degraded: !!v.degraded };
  } catch {
    /* ignore */
  }
  return { on: false, degraded: false };
}

export function savePrefs(p: SoundPrefs): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(p));
  } catch {
    /* ignore */
  }
}

/** Which sound a location state transition deserves. */
export function soundFor(prev: State | undefined, next: State | undefined, prefs: SoundPrefs): SoundKind | null {
  if (!prefs.on || !next || prev === next) return null;
  if (next === 'down') return 'alarm';
  if (next === 'degraded' && prefs.degraded && prev !== 'down') return 'alarm';
  if (next === 'up' && prev === 'down') return 'recovery';
  return null;
}

const PATTERNS: Record<SoundKind, [number, number][]> = {
  alarm: [[880, 0], [880, 0.28], [880, 0.56]],
  recovery: [[523, 0], [784, 0.22]],
};

export class Beeper {
  blocked = $state(false);
  private ctx: AudioContext | null = null;

  private context(): AudioContext | null {
    if (this.ctx) return this.ctx;
    const AC = globalThis.AudioContext ?? (globalThis as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!AC) return null;
    this.ctx = new AC();
    return this.ctx;
  }

  /** Resumes audio from a user gesture; returns whether sound can play. */
  async unlock(): Promise<boolean> {
    const ctx = this.context();
    if (!ctx) return false;
    try {
      await ctx.resume();
    } catch {
      /* ignore */
    }
    this.blocked = ctx.state !== 'running';
    return !this.blocked;
  }

  async play(kind: SoundKind): Promise<void> {
    const ctx = this.context();
    if (!ctx) return;
    if (ctx.state !== 'running') {
      try {
        await Promise.race([ctx.resume(), new Promise((r) => setTimeout(r, 300))]);
      } catch {
        /* ignore */
      }
    }
    if (ctx.state !== 'running') {
      this.blocked = true;
      return;
    }
    this.blocked = false;
    const t0 = ctx.currentTime + 0.02;
    for (const [freq, at] of PATTERNS[kind]) {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = 'sine';
      osc.frequency.value = freq;
      gain.gain.setValueAtTime(0.0001, t0 + at);
      gain.gain.exponentialRampToValueAtTime(0.3, t0 + at + 0.02);
      gain.gain.exponentialRampToValueAtTime(0.0001, t0 + at + 0.2);
      osc.connect(gain).connect(ctx.destination);
      osc.start(t0 + at);
      osc.stop(t0 + at + 0.22);
    }
  }
}

/** Shared preferences and player for every page that shows the sound toggle. */
export const soundPrefs = $state<SoundPrefs>(loadPrefs());
export const beeper = new Beeper();

/** Alerts play on every page except administration. */
export function soundAllowed(routeName: string): boolean {
  return !routeName.startsWith('admin');
}
