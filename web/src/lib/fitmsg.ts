// Places a device's status message on the same line as its name, on a
// location card. The name always stays whole: the message sits between the
// name and the IP when it fits there; otherwise it covers the IP and the
// latency entirely (never half of them; the name then gets their room too)
// and is cut with "…" if needed. Only when not even a few characters fit
// after the name does the message go to a second line.

export interface FitInput {
  /** Width of the row (px). */
  rowWidth: number;
  /** Right end of the name on one line, from the row's left edge (px). */
  nameEnd: number;
  /** Left edge of the IP, from the row's left edge (px). */
  hostStart: number;
  /** Natural width of the message (px). */
  msgWidth: number;
  /** Space kept on each side of the message (px). */
  gap: number;
  /** Right padding of the row (px). */
  pad: number;
  /** Narrowest useful message (px); below it the message goes to a second line. */
  minWidth: number;
}

export interface Fit {
  /** between: before the IP; cover: over the IP and latency; below: second line. */
  mode: 'between' | 'cover' | 'below';
  /** Distance of the message's right edge from the row's right edge (px). */
  right: number;
  /** Width limit of the message, or null when it fits whole. */
  maxWidth: number | null;
}

export function fitMessage(i: FitInput): Fit {
  const start = i.nameEnd + i.gap;
  const beforeHost = i.hostStart - i.gap;
  if (start + i.msgWidth <= beforeHost) return { mode: 'between', right: i.rowWidth - beforeHost, maxWidth: null };
  const room = i.rowWidth - i.pad - start;
  if (room < Math.min(i.minWidth, i.msgWidth)) return { mode: 'below', right: 0, maxWidth: null };
  return { mode: 'cover', right: i.pad, maxWidth: i.msgWidth <= room ? null : room };
}

const MODES = ['between', 'cover', 'below'];

/** Svelte action for a device row (`li`) with `.dname`, `.host` and an
 * optional `.msg`; re-run whenever `key` (texts, card size) changes. */
export function fitMsg(li: HTMLElement, _key: string) {
  const run = () => {
    const msg = li.querySelector<HTMLElement>('.msg');
    const name = li.querySelector<HTMLElement>('.dname');
    const host = li.querySelector<HTMLElement>('.host');
    li.classList.remove(...MODES);
    if (!msg || !name || !host) return;
    msg.style.removeProperty('max-width');
    msg.style.removeProperty('right');
    const row = li.getBoundingClientRect();
    if (row.width === 0) return; // not laid out (hidden)
    // the name's length on one line (measured unwrapped, in the plain layout)
    li.classList.add('measure');
    const range = document.createRange();
    range.selectNodeContents(name);
    const nameEnd = range.getBoundingClientRect().right - row.left;
    const msgWidth = msg.scrollWidth;
    li.classList.remove('measure');
    const fit = fitMessage({
      rowWidth: row.width,
      nameEnd,
      hostStart: host.getBoundingClientRect().left - row.left,
      msgWidth,
      gap: 10,
      pad: parseFloat(getComputedStyle(li).paddingRight) || 0,
      minWidth: 48,
    });
    li.classList.add(fit.mode);
    if (fit.mode === 'below') return;
    msg.style.setProperty('right', `${fit.right}px`);
    if (fit.maxWidth != null) msg.style.setProperty('max-width', `${fit.maxWidth}px`);
  };
  // The columns are shared by every row of the card, so the IP can move
  // without this row changing size: watch the row's cells as well.
  const ro = new ResizeObserver(run);
  const watch = () => {
    ro.disconnect();
    ro.observe(li);
    li.querySelectorAll('.dname, .host, .info, .msg').forEach((el) => ro.observe(el));
  };
  watch();
  document.fonts?.ready.then(run);
  run();
  return {
    update() {
      watch();
      run();
    },
    destroy() {
      ro.disconnect();
    },
  };
}
