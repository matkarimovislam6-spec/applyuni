import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api.js';
import Ascii from '../Ascii.jsx';
import { DeadlineBadge } from '../Deadline.jsx';
import { useT } from '../i18n.jsx';

export const COUNTRY = { IT: 'Italy', DE: 'Germany', US: 'USA' };
export const LANGUAGE = { en: 'English', it: 'Italian', de: 'German' };

export default function Programs() {
  const { t } = useT();
  const [programs, setPrograms] = useState(null);
  const [degree, setDegree] = useState('');
  const [country, setCountry] = useState('');
  const [openOnly, setOpenOnly] = useState(false);
  const [q, setQ] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    api.programs(country, degree).then(setPrograms).catch((e) => setError(e.message));
  }, [country, degree]);

  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    const rank = { open: 0, due: 0, upcoming: 1, not_published: 2, closed: 3 };
    return (programs || [])
      .filter((p) => !s || `${p.name} ${p.university} ${p.city}`.toLowerCase().includes(s))
      .filter((p) => !openOnly || ['open', 'due'].includes(p.deadline_status?.state))
      // most urgent first: open rounds by days left, then upcoming, unknown, closed
      .sort((a, b) => {
        const sa = a.deadline_status, sb = b.deadline_status;
        return (rank[sa?.state] ?? 9) - (rank[sb?.state] ?? 9) || (sa?.days_left ?? 1e9) - (sb?.days_left ?? 1e9);
      });
  }, [programs, q, openOnly]);

  return (
    <main>
      <section className="page-head">
        <Ascii art="duomo" className="float a2" />
        <div className="container">
          <p className="mono-label">{t('Catalogue')} · {t('Italy')} · {t('Germany')} · {t('USA')}</p>
          <h1 className="display-sm">{t('Find your')} <em>{t('programme')}</em></h1>
          <p className="lead">{t('Pick one to see every document you need for it, tailored to you, in under a minute.')}</p>
        </div>
      </section>
      <div className="container stack">
        <div className="chips country-chips">
          {[['', 'All countries'], ['IT', 'Italy'], ['DE', 'Germany'], ['US', 'USA']].map(([v, l]) => (
            <button key={l} className={`chip ${country === v ? 'on' : ''}`} onClick={() => setCountry(v)} aria-pressed={country === v}>{t(l)}</button>
          ))}
        </div>
        <div className="filters">
          <input className="grow" placeholder={t('Search programme, university or city…')} value={q} onChange={(e) => setQ(e.target.value)} />
          <div className="chips">
            {[['', 'All'], ['bachelor', 'Bachelor'], ['master', 'Master']].map(([v, l]) => (
              <button key={l} className={`chip ${degree === v ? 'on' : ''}`} onClick={() => setDegree(v)}>{t(l)}</button>
            ))}
            <button className={`chip ${openOnly ? 'on' : ''}`} onClick={() => setOpenOnly(!openOnly)} aria-pressed={openOnly}>{t('Open now')}</button>
          </div>
        </div>
        {error && <div className="error">{error}</div>}
        {!programs && !error && <p className="muted">{t('Loading…')}</p>}
        <div className="program-list">
          {shown.map((p) => (
            <Link key={p.id} to={`/programs/${p.id}`} className="program">
              <div>
                <span className="program-name">{p.name}</span>
                <span className="program-meta">{p.university} · {p.city}, {t(COUNTRY[p.country])}</span>
              </div>
              <div className="program-tags">
                <DeadlineBadge status={p.deadline_status} />
                <span className="tag">{t(p.degree === 'bachelor' ? 'Bachelor' : 'Master')}</span>
                <span className="tag hide-sm">{t(LANGUAGE[p.language] || 'English')}</span>
                <span className="arrow" aria-hidden="true">→</span>
              </div>
            </Link>
          ))}
          {programs && shown.length === 0 && <p className="muted">{openOnly ? t('No programme is open for applications right now.') : t('No programmes match your search.')}</p>}
        </div>
      </div>
    </main>
  );
}
