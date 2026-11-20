// Decorative ASCII "planets" drawn from characters, generated (not hand-drawn)
// so each one looks slightly different. Purely visual; hidden from screen readers.

const RAMP = ' .:-o#';

function rng(seed) {
  let s = seed;
  return () => ((s = (s * 9301 + 49297) % 233280) / 233280);
}

function blob(size, seed) {
  const r = rng(seed);
  const rows = [];
  const cx = size, cy = size / 2;
  for (let y = 0; y < size; y++) {
    let line = '';
    for (let x = 0; x < size * 2; x++) {
      const dx = (x - cx) / size, dy = (y - cy) / (size / 2);
      const d = Math.sqrt(dx * dx + dy * dy);
      if (d > 1) { line += ' '; continue; }
      // edge is dense, centre is airy, with noise
      const edge = d > 0.86 ? 5 : Math.floor((d * 3.4 + r() * 1.6));
      line += RAMP[Math.max(0, Math.min(5, edge))];
    }
    rows.push(line.replace(/\s+$/, ''));
  }
  return rows.join('\n');
}

export default function Ascii({ size = 9, seed = 1, className = '', style }) {
  return <pre aria-hidden="true" className={`ascii ${className}`} style={style}>{blob(size, seed)}</pre>;
}
