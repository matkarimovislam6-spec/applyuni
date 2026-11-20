// Decorative ASCII drawings of Italy. Built from code (repeat/pad) so every
// line lines up exactly in a monospace font. Purely visual: hidden from screen readers.

const r = (s, n) => s.repeat(n);

function colosseum() {
  const W = 40; // inner width
  const arches = '| ' + r('.-.  ', 7) + '.-. |';
  const pillars = '| ' + r('| |  ', 7) + '| | |';
  const band = '|' + r('=', W) + '|';
  return [
    ' ' + r('_', 25),
    '|  []   []   []   []   [] |' + r('_', 14),       // broken upper ring on the right
    '|  []   []   []   []   [] |  []   []     |',
    band, arches, pillars,
    band, arches, pillars,
    band, arches, pillars,
    "'" + r('-', W) + "'",
  ];
}

function pizza() {
  const toppings = 'o  .  o   .  o .  o  .  o   .  o  .';
  const rows = 11;
  const top = 23; // width of the crust
  const lines = [
    ' ' + r('_', top),
    '(' + toppings.slice(0, top) + ')',
    ' \\' + r('~', top - 2) + '/',
  ];
  for (let i = 1; i <= rows; i++) {
    const inner = top - 2 - i * 2;
    if (inner < 1) break;
    const fill = toppings.slice(i * 2, i * 2 + inner).padEnd(inner, ' ');
    lines.push(' ' + r(' ', i) + '\\' + fill + '/');
  }
  lines.push(r(' ', Math.floor(top / 2) + 1) + 'V');
  return lines;
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

const ART = { colosseum, pizza, pisa, espresso };

export default function Ascii({ art = 'colosseum', className = '', style }) {
  const draw = ART[art] || colosseum;
  return <pre aria-hidden="true" className={`ascii ${className}`} style={style}>{draw().join('\n')}</pre>;
}
