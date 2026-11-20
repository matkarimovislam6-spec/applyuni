import { useId } from 'react';
import { SHAPES } from './countryShapes.js';

// The country's outline, filled with its flag.
function FlagFill({ code, box: [x0, y0, x1, y1] }) {
  const w = x1 - x0, h = y1 - y0;
  if (code === 'IT') {
    return ['#009246', '#ffffff', '#ce2b37'].map((c, i) => <rect key={c} x={x0 + (w / 3) * i} y={y0} width={w / 3 + 0.3} height={h} fill={c} />);
  }
  if (code === 'DE') {
    return ['#111', '#dd0000', '#ffce00'].map((c, i) => <rect key={c} x={x0} y={y0 + (h / 3) * i} width={w} height={h / 3 + 0.3} fill={c} />);
  }
  // US: 13 stripes, blue canton with stars
  const sh = h / 13, cw = w * 0.4, ch = sh * 7;
  const stars = [];
  for (let r = 0; r < 5; r++) for (let c = 0; c < 6; c++) stars.push([x0 + cw * (c + 0.5) / 6, y0 + ch * (r + 0.5) / 5]);
  return (
    <>
      {Array.from({ length: 13 }, (_, i) => <rect key={i} x={x0} y={y0 + sh * i} width={w} height={sh + 0.2} fill={i % 2 ? '#ffffff' : '#b22234'} />)}
      <rect x={x0} y={y0} width={cw} height={ch} fill="#3c3b6e" />
      {stars.map(([x, y]) => <circle key={`${x}-${y}`} cx={x} cy={y} r="0.9" fill="#ffffff" />)}
    </>
  );
}

export default function CountryMap({ code, size = 72 }) {
  const id = useId().replace(/:/g, '');
  const shape = SHAPES[code];
  if (!shape) return null;
  return (
    <svg className="country-map" viewBox="0 0 100 100" width={size} height={size} aria-hidden="true">
      <defs><clipPath id={`clip-${id}`}><path d={shape.d} /></clipPath></defs>
      {/* outline first: the flag on top covers its inner half, so only the outside edge shows */}
      <path className="country-outline" d={shape.d} fill="none" strokeWidth="2.4" strokeLinejoin="round" />
      <g clipPath={`url(#clip-${id})`}><FlagFill code={code} box={shape.box} /></g>
    </svg>
  );
}
