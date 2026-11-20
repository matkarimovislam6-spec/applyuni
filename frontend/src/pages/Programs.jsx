import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api.js';
import Ascii from '../Ascii.jsx';

export default function Programs() {
  const [programs, setPrograms] = useState(null);
  const [degree, setDegree] = useState('');
  const [q, setQ] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    api.programs('IT', degree).then(setPrograms).catch((e) => setError(e.message));
  }, [degree]);

  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return (programs || []).filter((p) => !s || `${p.name} ${p.university} ${p.city}`.toLowerCase().includes(s));
  }, [programs, q]);

  return (
    <main>
      <section className="page-head">
        <Ascii size={9} seed={5} className="float a2" />
        <div className="container">
          <p className="mono-label">Catalogue · Italy</p>
          <h1 className="display-sm">Find your <em>programme</em></h1>
          <p className="lead">Pick one to see every document you need for it — tailored to you, in under a minute.</p>
        </div>
      </section>
      <div className="container stack">
        <div className="filters">
          <input className="grow" placeholder="Search programme, university or city…" value={q} onChange={(e) => setQ(e.target.value)} />
          <div className="chips">
            {[['', 'All'], ['bachelor', 'Bachelor'], ['master', 'Master']].map(([v, l]) => (
              <button key={l} className={`chip ${degree === v ? 'on' : ''}`} onClick={() => setDegree(v)}>{l}</button>
            ))}
          </div>
        </div>
        {error && <div className="error">{error}</div>}
        {!programs && !error && <p className="muted">Loading…</p>}
        <div className="program-list">
          {shown.map((p) => (
            <Link key={p.id} to={`/programs/${p.id}`} className="program">
              <div>
                <span className="program-name">{p.name}</span>
                <span className="program-meta">{p.university} · {p.city}</span>
              </div>
              <div className="program-tags">
                <span className="tag">{p.degree}</span>
                <span className="tag hide-sm">{p.language === 'en' ? 'English' : 'Italian'}</span>
                <span className="arrow" aria-hidden="true">→</span>
              </div>
            </Link>
          ))}
          {programs && shown.length === 0 && <p className="muted">No programmes match your search.</p>}
        </div>
      </div>
    </main>
  );
}
