import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useT } from './i18n.jsx';

export const VERDICTS = {
  strong: { label: 'Strong match', tone: 'ok' },
  possible: { label: 'Possible', tone: 'info' },
  long_shot: { label: 'Long shot', tone: 'warn' },
  not_eligible: { label: 'Not eligible yet', tone: 'bad' },
  closed: { label: 'Closed this year', tone: 'muted' },
};

const ICON = { met: '✓', action: '→', warning: '!', blocker: '✕', unknown: '?' };
const ORDER = { blocker: 0, action: 1, warning: 2, unknown: 3, met: 4 };

export function ScoreRing({ score, tone }) {
  const { t } = useT();
  const r = 22, c = 2 * Math.PI * r;
  return (
    <svg className={`ring ${tone}`} viewBox="0 0 56 56" width="56" height="56" role="img" aria-label={t('Match score {score} out of 100', { score })}>
      <circle cx="28" cy="28" r={r} className="ring-bg" />
      <circle cx="28" cy="28" r={r} className="ring-fg" strokeDasharray={c} strokeDashoffset={c * (1 - score / 100)} transform="rotate(-90 28 28)" />
      <text x="28" y="33" textAnchor="middle">{score}</text>
    </svg>
  );
}

export function CheckList({ checks, limit }) {
  const sorted = [...checks].sort((a, b) => ORDER[a.status] - ORDER[b.status]);
  const shown = limit ? sorted.filter((c) => c.status !== 'met').slice(0, limit) : sorted;
  return (
    <ul className="checklist">
      {shown.map((c) => (
        <li key={c.key} className={`chk ${c.status}`}>
          <span className="chk-icon" aria-hidden="true">{ICON[c.status]}</span>
          <div>
            <strong>{c.label}</strong> <span>{c.detail}</span>
            {c.fix && <div className="chk-fix">{c.fix}</div>}
          </div>
        </li>
      ))}
    </ul>
  );
}

export function MatchCard({ match }) {
  const { t } = useT();
  const [open, setOpen] = useState(false);
  const v = VERDICTS[match.verdict] || VERDICTS.possible;
  const p = match.program;
  const issues = match.checks.filter((c) => c.status !== 'met').length;
  return (
    <article className={`match ${v.tone}`}>
      <div className="match-head">
        <ScoreRing score={match.score} tone={v.tone} />
        <div className="match-title">
          <Link to={`/programs/${p.id}`} className="program-name">{p.name}</Link>
          <span className="program-meta">{p.university} · {p.city}</span>
        </div>
        <span className={`verdict ${v.tone}`}>{t(v.label)}</span>
      </div>
      {issues > 0 ? <CheckList checks={match.checks} limit={open ? 0 : 3} /> : <p className="small ok-text">{t('Nothing to fix. You meet everything we can check.')}</p>}
      {open && <CheckList checks={match.checks.filter((c) => c.status === 'met')} />}
      <div className="match-foot">
        <button className="link small" onClick={() => setOpen(!open)}>{open ? t('Show less') : t('All {n} checks', { n: match.checks.length })}</button>
        {match.confidence < 100 && <span className="small muted">{t('Based on {n}% of the checks: complete your profile for a sharper result.', { n: match.confidence })}</span>}
      </div>
    </article>
  );
}
