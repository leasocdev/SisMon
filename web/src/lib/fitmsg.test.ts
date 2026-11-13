import { describe, expect, it } from 'vitest';
import { fitMessage } from './fitmsg';

const base = { rowWidth: 300, nameEnd: 60, hostStart: 200, gap: 10, pad: 6, minWidth: 48 };

describe('fitMessage', () => {
  it('sits between the name and the IP when it fits there', () => {
    // free space: 70..190
    expect(fitMessage({ ...base, msgWidth: 120 })).toEqual({ mode: 'between', right: 110, maxWidth: null });
  });
  it('covers the IP when it does not fit before it', () => {
    expect(fitMessage({ ...base, msgWidth: 121 })).toEqual({ mode: 'cover', right: 6, maxWidth: null });
  });
  it('is cut after the name when the rest of the row is too short', () => {
    // room after the name: 300 - 6 - 70 = 224
    expect(fitMessage({ ...base, msgWidth: 400 })).toEqual({ mode: 'cover', right: 6, maxWidth: 224 });
  });
  it('goes to a second line when not even a few characters fit after the name', () => {
    expect(fitMessage({ ...base, nameEnd: 250, msgWidth: 120 })).toEqual({ mode: 'below', right: 0, maxWidth: null });
  });
  it('a short message that fits whole stays on the line', () => {
    // room 300 - 6 - 260 = 34: enough for a 30 px message
    expect(fitMessage({ ...base, nameEnd: 250, msgWidth: 30 })).toEqual({ mode: 'cover', right: 6, maxWidth: null });
  });
});
