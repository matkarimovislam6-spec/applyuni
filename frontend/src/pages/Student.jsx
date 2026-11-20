import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { api } from '../api.js';
import { useMe } from '../App.jsx';
import { StatusBadge } from '../components.jsx';

export default function Student() {
  const { id } = useParams();
  const { me, refresh } = useMe();
  const nav = useNavigate();
  const [student, setStudent] = useState(null);
  const [apps, setApps] = useState([]);
  const [error, setError] = useState('');

  const load = async () => {
    try {
      const [s, a] = await Promise.all([api.student(id), api.applications(id)]);
      setStudent(s);
      setApps(a);
    } catch (e) {
      setError(e.message);
    }
  };
  useEffect(() => {
    load();
  }, [id]);

  if (error) return <div className="error">{error}</div>;
  if (!student) return <p className="muted">Loading…</p>;

  const remove = async () => {
    if (!confirm(`Permanently delete ${student.full_name} and all their documents? This cannot be undone.`)) return;
    await api.deleteStudent(id);
    await refresh();
    nav(me.student_id ? '/login' : '/students');
  };

  return (
    <div className="stack">
      <div className="row between">
        <div>
          <h1>{student.full_name}</h1>
          <p className="muted small">Agent inbox for portal emails: <span className="mono">{student.email_alias}</span></p>
        </div>
      </div>
      <Applications studentId={id} apps={apps} onChange={load} />
      <Profile student={student} onSaved={setStudent} />
      <div className="card danger-zone">
        <h3>Delete all data</h3>
        <p className="small muted">Removes the profile, every application and every encrypted file. Encryption keys are destroyed, so backups become unreadable.</p>
        <button className="btn danger" onClick={remove}>Delete everything</button>
      </div>
    </div>
  );
}

function Applications({ studentId, apps, onChange }) {
  const [programs, setPrograms] = useState([]);
  const [degree, setDegree] = useState('');
  const [picking, setPicking] = useState(apps.length === 0);
  const [error, setError] = useState('');

  useEffect(() => {
    if (picking) api.programs('IT', degree).then(setPrograms);
  }, [picking, degree]);

  const chosen = new Set(apps.map((a) => a.program.id));
  const add = async (pid) => {
    setError('');
    try {
      await api.createApplication(studentId, pid);
      onChange();
    } catch (e) {
      setError(e.message);
    }
  };

  return (
    <section className="card stack">
      <div className="row between">
        <h2>Applications</h2>
        <button className="btn" onClick={() => setPicking(!picking)}>{picking ? 'Close' : '+ Add university'}</button>
      </div>
      {apps.length === 0 && !picking && <p className="muted">No applications yet.</p>}
      {apps.map((a) => (
        <Link key={a.id} to={`/applications/${a.id}`} className="app-row">
          <div>
            <strong>{a.program.university}</strong>
            <div className="small muted">{a.program.name} · {a.program.degree}</div>
          </div>
          <StatusBadge status={a.status} />
        </Link>
      ))}
      {picking && (
        <div className="stack">
          <div className="row">
            <span className="muted small">Italy 🇮🇹</span>
            <select value={degree} onChange={(e) => setDegree(e.target.value)}>
              <option value="">All degrees</option>
              <option value="bachelor">Bachelor</option>
              <option value="master">Master</option>
            </select>
          </div>
          {error && <div className="error">{error}</div>}
          <div className="grid">
            {programs.map((p) => (
              <div key={p.id} className="program">
                <div>
                  <strong>{p.name}</strong>
                  <div className="small muted">{p.university} · {p.city} · {p.degree} · {p.language === 'en' ? 'English' : 'Italian'}</div>
                  <div className="small">
                    Deadline: {p.deadline ? new Date(p.deadline).toLocaleDateString() : <span className="warn-text">not verified yet</span>}
                    {' · '}
                    <a href={p.website} target="_blank" rel="noreferrer">official site</a>
                  </div>
                </div>
                <button className="btn" disabled={chosen.has(p.id)} onClick={() => add(p.id)}>{chosen.has(p.id) ? 'Added' : 'Apply'}</button>
              </div>
            ))}
          </div>
        </div>
      )}
    </section>
  );
}

const FIELDS = [
  ['school_years', 'Years of school completed', 'number'],
  ['completed_university_years', 'University years completed', 'number'],
  ['gpa', 'GPA / average grade', 'text'],
  ['english_test', 'English test (IELTS, TOEFL…)', 'text'],
  ['english_score', 'English score', 'text'],
  ['date_of_birth', 'Date of birth', 'date'],
  ['citizenship', 'Citizenship', 'text'],
  ['phone', 'Phone', 'tel'],
];

function Profile({ student, onSaved }) {
  const [p, setP] = useState({ documents_language: 'uz', wants_scholarship: true, citizenship: 'Uzbekistan', ...student.profile });
  const [msg, setMsg] = useState('');
  const set = (k, type) => (e) =>
    setP({ ...p, [k]: type === 'number' ? (e.target.value === '' ? '' : Number(e.target.value)) : type === 'checkbox' ? e.target.checked : e.target.value });

  const save = async (e) => {
    e.preventDefault();
    setMsg('');
    try {
      onSaved(await api.saveProfile(student.id, p));
      setMsg('Saved. Your document checklists were updated.');
    } catch (err) {
      setMsg(err.message);
    }
  };

  return (
    <form className="card stack" onSubmit={save}>
      <h2>Profile</h2>
      <p className="small muted">This decides which documents each university needs from you.</p>
      <div className="grid two">
        {FIELDS.map(([k, label, type]) => (
          <label key={k}>{label}<input type={type} value={p[k] ?? ''} onChange={set(k, type)} /></label>
        ))}
        <label>
          Language of your documents
          <select value={p.documents_language} onChange={set('documents_language')}>
            <option value="uz">Uzbek</option>
            <option value="ru">Russian</option>
            <option value="en">English</option>
          </select>
        </label>
        <label className="check">
          <input type="checkbox" checked={!!p.wants_scholarship} onChange={set('wants_scholarship', 'checkbox')} />
          I want to apply for a regional (DSU) scholarship
        </label>
      </div>
      <div className="row">
        <button className="btn primary">Save profile</button>
        {msg && <span className="small muted">{msg}</span>}
      </div>
    </form>
  );
}
