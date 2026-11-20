import { useCallback, useEffect, useState } from 'react';
import { Navigate } from 'react-router-dom';
import { api } from '../api.js';
import { useAuth } from '../auth.jsx';
import { useT } from '../i18n.jsx';

// ApplyUni staff: documents waiting for a person, from every applicant.
// Open each file, then accept it or reject it with a reason the student sees.
export default function AdminReview() {
  const { t, locale } = useT();
  const { user } = useAuth();
  const [items, setItems] = useState(null);
  const [error, setError] = useState('');
  const [notes, setNotes] = useState({});
  const [busy, setBusy] = useState('');

  const load = useCallback(() => {
    api.adminReviewQueue().then(setItems).catch((e) => setError(e.message));
  }, []);
  useEffect(() => { if (user?.is_admin) load(); }, [user, load]);

  if (user === undefined) return <main className="container section muted">{t('Loading…')}</main>;
  if (!user) return <Navigate to="/login" state={{ from: '/admin/review' }} replace />;
  if (!user.is_admin) return <Navigate to="/" replace />;

  const decide = async (id, accept) => {
    const note = (notes[id] || '').trim();
    if (!accept && !note) {
      setError(t('Write the reason for rejecting, so the student knows what to fix.'));
      return;
    }
    setBusy(id);
    setError('');
    try {
      await api.adminReview(id, accept, note);
      setItems((list) => list.filter((d) => d.id !== id));
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy('');
    }
  };

  // Group by student so one applicant's documents are reviewed together.
  const groups = [];
  for (const d of items || []) {
    let g = groups.find((x) => x.student_id === d.student_id);
    if (!g) groups.push(g = { student_id: d.student_id, name: d.student_name, programs: d.programs, docs: [] });
    g.docs.push(d);
  }

  return (
    <main className="container section stack">
      <div className="row between wrap">
        <div>
          <p className="mono-label">{t('ApplyUni staff')}</p>
          <h1 className="display-sm">{t('Document review')}</h1>
          <p className="muted small">{t('Check that each file is the right document, readable, valid and in the student\'s name.')}</p>
        </div>
        <button className="btn" onClick={load}>{t('Refresh')}</button>
      </div>
      {error && <div className="error">{error}</div>}
      {items && items.length === 0 && <p className="muted">{t('Nothing to review.')}</p>}

      {groups.map((g) => (
        <section key={g.student_id} className="card stack">
          <div>
            <h2>{g.name}</h2>
            {g.programs.map((p) => <p key={p} className="small muted">{p}</p>)}
          </div>
          <ol className="doclist">
            {g.docs.map((d) => {
              const notesFromChecks = [...(d.verdict?.issues || []), ...(d.verdict?.cross || []), ...(d.verdict?.notes || [])];
              return (
                <li key={d.id} className="stack">
                  <div className="row between wrap">
                    <strong>{d.doc_type.replace(/_/g, ' ')}</strong>
                    <span className="small muted">{new Date(d.created_at).toLocaleString(locale)} · {Math.round(d.size_bytes / 1024)} KB</span>
                  </div>
                  <a className="link small" href={api.adminFileUrl(d.id)} target="_blank" rel="noopener noreferrer">{t('Open {file}', { file: d.filename })}</a>
                  {notesFromChecks.map((x) => <p key={x} className="small warn-text">{x}</p>)}
                  <input
                    placeholder={t('Reason, if rejecting (the student sees this)')}
                    value={notes[d.id] || ''}
                    onChange={(e) => setNotes((n) => ({ ...n, [d.id]: e.target.value }))}
                  />
                  <div className="row wrap">
                    <button className="btn" disabled={busy === d.id} onClick={() => decide(d.id, true)}>{t('Accept')}</button>
                    <button className="btn" disabled={busy === d.id} onClick={() => decide(d.id, false)}>{t('Reject')}</button>
                  </div>
                </li>
              );
            })}
          </ol>
        </section>
      ))}
    </main>
  );
}
