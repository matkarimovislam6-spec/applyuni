// Decorative ASCII drawings of Italy. Built from code (repeat/pad) so every
// line lines up exactly in a monospace font. Purely visual: hidden from screen readers.

const r = (s, n) => s.repeat(n);

// A character canvas: put() characters at (x, y), then read out trimmed lines.
function canvas(w, h) {
  const g = Array.from({ length: h }, () => Array(w).fill(' '));
  const put = (x, y, ch) => { if (y >= 0 && y < h && x >= 0 && x < w) g[y][x] = ch; };
  const text = (x, y, s) => [...s].forEach((ch, i) => ch !== ' ' && put(x + i, y, ch));
  const lines = () => g.map((row) => row.join('').replace(/\s+$/, ''));
  return { put, text, lines };
}

// Duomo di Milano: gothic facade with six buttresses topped by spires, a central
// gable crowned by the tall Madonnina spire, pointed windows and portals.
function duomo() {
  const W = 47, H = 24, C = 23;
  const { put, text, lines } = canvas(W, H);
  const buttresses = [[3, 6], [10, 4], [17, 2], [29, 2], [36, 4], [43, 6]]; // [x, top row]

  // Madonnina needle and central gable with small pinnacles along its slopes
  text(C, 0, '+');
  for (let y = 1; y <= 3; y++) put(C, y, '|');
  put(C, 4, 'A');
  for (let i = 1; i <= 5; i++) {
    put(C - i, 4 + i, '/');
    put(C + i, 4 + i, '\\');
    if (i % 2 === 0) { put(C - i, 3 + i, '^'); put(C + i, 3 + i, '^'); }
  }

  // Buttresses: a pinnacle on top, then a shaft down to the ground
  for (const [x, top] of buttresses) {
    put(x, top, '^');
    put(x, top + 1, 'A');
    for (let y = top + 2; y < H - 2; y++) put(x, y, '|');
  }

  // Lower roofs of the side bays
  for (let x = 11; x <= 16; x++) { put(x, 9, '_'); put(W - 1 - x, 9, '_'); }
  for (let x = 4; x <= 9; x++) { put(x, 11, '_'); put(W - 1 - x, 11, '_'); }

  // Side bays: pointed window above, pointed portal below
  for (const a of [3, 10, 29, 36]) {
    const m = a + 3;
    text(m, 13, '/\\'); text(m - 1, 14, '|::|'); text(m - 1, 15, '|::|');
    text(m, 18, '/\\'); text(m - 1, 19, '|  |'); text(m - 1, 20, '|  |'); text(m - 1, 21, '|  |');
  }

  // Central bay: large window, cornice, main portal
  text(C - 2, 11, '.-^-.');
  text(C - 3, 12, '/:::::\\');
  text(C - 3, 13, '|:::::|');
  text(C - 3, 14, '|:::::|');
  text(C - 3, 15, "'-----'");
  for (let x = 4; x <= 42; x++) if (![10, 17, 29, 36].includes(x)) put(x, 16, '-');
  put(C, 17, '^');
  text(C - 1, 18, '/ \\');
  text(C - 2, 19, '/   \\');
  text(C - 2, 20, '|   |');
  text(C - 2, 21, '|   |');

  // Steps / piazza
  text(1, H - 2, r('=', W - 2));
  text(0, H - 1, r('~', W));
  return lines();
}

// Pizza slice: scalloped crust, pepperoni, melted cheese texture.
function pizza() {
  const W = 35, R = 17; // crust width (5 scallops of 7) and rows of the triangle
  const { put, text, lines } = canvas(W + 2, R + 3);
  // crust
  text(1, 0, r('_.-~~-.', 5));
  put(0, 1, '('); put(W + 1, 1, ')');
  for (let x = 1; x <= W; x++) put(x, 1, x % 4 === 2 ? "'" : x % 4 === 0 ? '`' : ' ');
  // triangle edges and cheese texture
  const inside = (x, y) => x > y + 1 && x < W - y;
  for (let y = 0; y < R; y++) {
    const row = y + 2;
    put(y + 1, row, '\\');
    put(W - y, row, '/');
    for (let x = y + 2; x < W - y; x++) {
      const k = (x * 7 + y * 5) % 13;
      put(x, row, k === 0 ? '~' : k === 6 ? ',' : k === 9 ? '.' : ' ');
    }
  }
  put(R + 1, R + 2, 'V');
  // pepperoni (3x3 stamps), only where they fit inside the slice
  for (const [px, py] of [[4, 0], [14, 0], [25, 0], [10, 3], [20, 4], [14, 7], [16, 11]]) {
    if (!inside(px, py + 2) || !inside(px + 2, py + 2)) continue;
    text(px, py + 2, '.-.');
    text(px, py + 3, '(o)');
    text(px, py + 4, "'-'");
  }
  return lines();
}

function pisa() {
  // Upright tower first (every floor 11 wide), then lean it: each line
  // further up is shifted right a little more.
  const floors = 6;
  const tower = ['    .-.    ', '  _|___|_  ', ' |=======| ', ' |:|:|:|:| '];
  for (let f = 0; f < floors; f++) tower.push(' |=======| ', ' |:|:|:|:| ');
  tower.push('|=========|', '|_________|');
  const n = tower.length;
  const lean = 0.42;
  const lines = tower.map((line, i) => r(' ', Math.round((n - 1 - i) * lean)) + line);
  lines.push(r('~', 13));
  return lines;
}

function espresso() {
  return [
    '     (   )  (',
    '      ) (   )',
    '     (   ) (',
    '  .-----------.',
    '  |           |--.',
    '  |  ApplyUni |  |',
    '  |           |--\'',
    '   \\_________/',
    ' .-------------.',
    " '-------------'",
  ];
}

const ART = { duomo, pizza, pisa, espresso };

export default function Ascii({ art = 'duomo', className = '', style }) {
  const draw = ART[art] || duomo;
  return <pre aria-hidden="true" className={`ascii ${className}`} style={style}>{draw().join('\n')}</pre>;
}
