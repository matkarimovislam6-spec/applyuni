import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api.js';

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
    <main className="container section stack">
      <div>
        <h1>Universities in Italy</h1>
        <p className="muted">Pick a programme to see every document you need for it.</p>
      </div>
      <div className="row wrap">
        <input className="grow" placeholder="Search programme, university or city" value={q} onChange={(e) => setQ(e.target.value)} />
        <select value={degree} onChange={(e) => setDegree(e.target.value)}>
          <option value="">Bachelor & master</option>
          <option value="bachelor">Bachelor</option>
          <option value="master">Master</option>
        </select>
      </div>
      {error && <div className="error">{error}</div>}
      {!programs && !error && <p className="muted">Loading…</p>}
      <div className="grid">
        {shown.map((p) => (
          <Link key={p.id} to={`/programs/${p.id}`} className="program">
            <div>
              <strong>{p.name}</strong>
              <div className="small muted">{p.university} · {p.city}</div>
            </div>
            <div className="row">
              <span className="badge">{p.degree}</span>
              <span className="badge hide-sm">{p.language === 'en' ? 'English' : 'Italian'}</span>
              <span className="chev">›</span>
            </div>
          </Link>
        ))}
        {programs && shown.length === 0 && <p className="muted">No programmes match your search.</p>}
      </div>
    </main>
  );
}
