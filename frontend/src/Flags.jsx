// Small SVG flags (flag emoji show as letters on Windows).

export function UzFlag({ width = 27, height = 14 }) {
  // 12 stars in rows of 3, 4 and 5 on the blue stripe, right-aligned beside the crescent
  const stars = [];
  [[3, 1.3], [4, 2.9], [5, 4.5]].forEach(([n, y]) => {
    for (let i = 0; i < n; i++) stars.push([16.5 - i * 1.7, y]);
  });
  return (
    <svg className="flag" viewBox="0 0 36 18" width={width} height={height} role="img" aria-label="Uzbekistan">
      <rect width="36" height="6" fill="#0099b5" />
      <rect y="6" width="36" height="6" fill="#fff" />
      <rect y="12" width="36" height="6" fill="#1eb53a" />
      <rect y="5.6" width="36" height="0.8" fill="#ce1126" />
      <rect y="11.6" width="36" height="0.8" fill="#ce1126" />
      <circle cx="4.6" cy="2.9" r="2.2" fill="#fff" />
      <circle cx="5.5" cy="2.9" r="1.9" fill="#0099b5" />
      {stars.map(([x, y]) => <circle key={`${x}-${y}`} cx={x} cy={y} r="0.42" fill="#fff" />)}
    </svg>
  );
}

export function UsFlag({ width = 27, height = 14 }) {
  const stripes = Array.from({ length: 13 }, (_, i) => i);
  const stars = [];
  for (let r = 0; r < 5; r++) for (let c = 0; c < 6; c++) stars.push([1.4 + c * 2.35, 1.1 + r * 1.75]);
  return (
    <svg className="flag" viewBox="0 0 38 20" width={width} height={height} role="img" aria-label="United States">
      {stripes.map((i) => <rect key={i} y={(i * 20) / 13} width="38" height={20 / 13 + 0.05} fill={i % 2 ? '#fff' : '#b22234'} />)}
      <rect width="15.2" height={(20 / 13) * 7} fill="#3c3b6e" />
      {stars.map(([x, y]) => <circle key={`${x}-${y}`} cx={x} cy={y} r="0.45" fill="#fff" />)}
    </svg>
  );
}

export function RuFlag({ width = 27, height = 14 }) {
  return (
    <svg className="flag" viewBox="0 0 30 18" width={width} height={height} role="img" aria-label="Russia">
      <rect width="30" height="6" fill="#fff" />
      <rect y="6" width="30" height="6" fill="#0039a6" />
      <rect y="12" width="30" height="6" fill="#d52b1e" />
    </svg>
  );
}

export const FLAG = { en: UsFlag, uz: UzFlag, ru: RuFlag };
