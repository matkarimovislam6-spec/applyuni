import { useEffect, useState } from 'react';
import { Link, Navigate, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth.jsx';

const EMPTY = {
  degree_target: 'bachelor', school_years: 11, completed_university_years: 0, gpa: '',
  english_test: '', english_score: '', documents_language: 'uz', wants_scholarship: true,
};

export default function Account() {
  const { user, profile, saveProfile, logout } = useAuth();
  const nav = useNavigate();
  const [form, setForm] = useState(null);
  const [status, setStatus] = useState(''); // '', saving, saved, error message

  useEffect(() => {
    if (profile) setForm({ ...EMPTY, ...profile });
  }, [profile]);

  if (user === undefined) return <main className="container section muted">Loading…</main>;
  if (!user) return <Navigate to="/login" state={{ from: '/account' }} replace />;

  const set = (k, v) => {
    setForm({ ...form, [k]: v });
    setStatus('');
  };
  const save = async (e) => {
    e.preventDefault();
    setStatus('saving');
    try {
      await saveProfile(form);
      setStatus('saved');
    } catch (err) {
      setStatus(err.message);
    }
  };

  return (
    <main className="container section stack account">
      <div className="row between wrap">
        <div>
          <p className="mono-label">My account</p>
          <h1 className="display-sm">Hi, <em>{user.username}</em></h1>
          <p className="muted small">Member since {new Date(user.created_at).toLocaleDateString()}</p>
        </div>
        <button className="btn" onClick={async () => { await logout(); nav('/'); }}>Log out</button>
      </div>

      {!form ? (
        <p className="muted">Loading your details…</p>
      ) : (
        <form className="card stack" onSubmit={save}>
          <div>
            <h2>About you</h2>
            <p className="small muted">Saved to your account. Every programme checklist uses these answers.</p>
          </div>
          <div className="form-grid">
            <label>I want to study
              <select value={form.degree_target} onChange={(e) => set('degree_target', e.target.value)}>
                <option value="bachelor">Bachelor</option>
                <option value="master">Master</option>
              </select>
            </label>
            <label>Years of school completed
              <select value={form.school_years} onChange={(e) => set('school_years', Number(e.target.value))}>
                <option value={11}>11 years</option>
                <option value={12}>12 years</option>
              </select>
            </label>
            <label>University years completed
              <select value={form.completed_university_years} onChange={(e) => set('completed_university_years', Number(e.target.value))}>
                <option value={0}>None</option>
                <option value={1}>1 year</option>
                <option value={2}>2 years</option>
                <option value={3}>3 years or more</option>
              </select>
            </label>
            <label>GPA / average grade
              <input value={form.gpa} onChange={(e) => set('gpa', e.target.value)} placeholder="e.g. 4.6 / 5" />
            </label>
            <label>English certificate
              <select value={form.english_test} onChange={(e) => set('english_test', e.target.value)}>
                <option value="">Not yet</option>
                <option value="IELTS">IELTS</option>
                <option value="TOEFL">TOEFL</option>
                <option value="Duolingo">Duolingo</option>
              </select>
            </label>
            <label>English score
              <input value={form.english_score} onChange={(e) => set('english_score', e.target.value)} placeholder="e.g. 6.5" disabled={!form.english_test} />
            </label>
            <label>My documents are in
              <select value={form.documents_language} onChange={(e) => set('documents_language', e.target.value)}>
                <option value="uz">Uzbek</option>
                <option value="ru">Russian</option>
                <option value="en">English</option>
                <option value="mixed">Mixed (some English, some Uzbek or Russian)</option>
              </select>
            </label>
            <label className="check">
              <input type="checkbox" checked={!!form.wants_scholarship} onChange={(e) => set('wants_scholarship', e.target.checked)} />
              I want a regional (DSU) scholarship
            </label>
          </div>
          <div className="row">
            <button className="btn primary" disabled={status === 'saving'}>{status === 'saving' ? 'Saving…' : 'Save'}</button>
            {status === 'saved' && <span className="saved">✓ Saved</span>}
            {status && !['saving', 'saved'].includes(status) && <span className="error small">{status}</span>}
          </div>
        </form>
      )}

      <div className="card row between wrap">
        <div>
          <h3>Next: pick your programmes</h3>
          <p className="small muted">Your answers are already filled in on every programme page.</p>
        </div>
        <Link to="/programs" className="btn primary">Browse universities →</Link>
      </div>
    </main>
  );
}
