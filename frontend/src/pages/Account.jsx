import { useEffect, useState } from 'react';
import { Link, Navigate, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth.jsx';

// Keys must match backend internal/matching fields.
export const FIELDS = [
  ['computer_science', 'Computer science / IT'],
  ['electrical_engineering', 'Electrical & electronic engineering'],
  ['engineering_other', 'Other engineering'],
  ['mathematics', 'Mathematics'],
  ['statistics_data', 'Statistics / data'],
  ['physics', 'Physics'],
  ['natural_sciences', 'Biology / chemistry'],
  ['economics_business', 'Economics / business'],
  ['law', 'Law'],
  ['political_science', 'Politics / international relations'],
  ['psychology', 'Psychology'],
  ['social_sciences', 'Other social sciences'],
  ['architecture_design', 'Architecture / design'],
  ['humanities', 'Languages / humanities'],
  ['health', 'Medicine / health'],
];

const EMPTY = {
  degree_target: 'bachelor', education_level: '', school_years: 11, completed_university_years: 0, school_grade: '',
  university: '', major: '', major_name: '', gpa: '', graduation_year: '',
  english_test: '', english_score: '', english_date: '', italian_level: '', german_level: '', other_languages: [],
  sat_score: '', gre_score: '', gmat_score: '', work_years: 0, has_portfolio: false,
  interests: [], budget_eur_year: '', documents_language: 'uz', wants_scholarship: true,
};

// "3.33/4" <-> {value: "3.33", scale: "4"}
const splitGrade = (g, fallback) => {
  const m = /^\s*([\d.,]+)\s*\/\s*([\d.,]+)\s*$/.exec(g || '');
  return m ? { value: m[1], scale: m[2] } : { value: g || '', scale: fallback };
};

// What we still need for good matching
function missing(f) {
  const list = [];
  if (!f.education_level) list.push('highest education');
  if (f.degree_target === 'master') {
    if (!f.major) list.push('bachelor subject');
    if (!f.gpa) list.push('GPA');
  } else {
    if (!f.school_grade) list.push('school grades');
    if (!f.interests.length) list.push('interests');
  }
  if (!f.english_test) list.push('English test');
  return list;
}

export default function Account() {
  const { user, profile, saveProfile, logout } = useAuth();
  const nav = useNavigate();
  const [form, setForm] = useState(null);
  const [status, setStatus] = useState('');

  useEffect(() => {
    if (profile) setForm({ ...EMPTY, ...profile });
  }, [profile]);

  if (user === undefined) return <main className="container section muted">Loading…</main>;
  if (!user) return <Navigate to="/login" state={{ from: '/account' }} replace />;

  const set = (k, v) => {
    setForm((f) => ({ ...f, [k]: v }));
    setStatus('');
  };
  const num = (v) => (v === '' ? '' : Number(v));
  const save = async (e) => {
    e.preventDefault();
    setStatus('saving');
    try {
      const clean = { ...form };
      for (const k of ['sat_score', 'gre_score', 'gmat_score', 'budget_eur_year', 'graduation_year']) clean[k] = clean[k] === '' ? 0 : Number(clean[k]);
      clean.other_languages = (clean.other_languages || []).filter((l) => l.language.trim());
      await saveProfile(clean);
      setStatus('saved');
    } catch (err) {
      setStatus(err.message);
    }
  };

  const todo = form ? missing(form) : [];
  const done = form ? Math.round(((5 - todo.length) / 5) * 100) : 0;
  const master = form?.degree_target === 'master';
  const gpa = splitGrade(form?.gpa, '4');
  const school = splitGrade(form?.school_grade, '5');

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

      {/* ---------- Profile ---------- */}
      {!form ? <p className="muted">Loading your details…</p> : (
        <form className="stack" onSubmit={save}>
          <div className="row between wrap">
            <div>
              <h2 className="display-xs">About you</h2>
              <p className="small muted">Each programme page uses these answers to show how well you match and what to fix.</p>
            </div>
            <div className="completeness" title={todo.length ? `Missing: ${todo.join(', ')}` : 'Complete'}>
              <span className="small">Profile {done}% complete</span>
              <div className="progress"><div style={{ width: `${done}%` }} /></div>
            </div>
          </div>
          {todo.length > 0 && <div className="notice small">Add your {todo.join(', ')} for accurate match results on each programme.</div>}

          <fieldset className="card form-section">
            <legend>Goal</legend>
            <div className="form-grid">
              <label>I want to study
                <select value={form.degree_target} onChange={(e) => set('degree_target', e.target.value)}>
                  <option value="bachelor">Bachelor's</option>
                  <option value="master">Master's</option>
                </select>
              </label>
              <label>Max budget per year (EUR)
                <input type="number" min="0" step="500" value={form.budget_eur_year || ''} onChange={(e) => set('budget_eur_year', num(e.target.value))} placeholder="e.g. 4000" />
              </label>
              <label className="check">
                <input type="checkbox" checked={!!form.wants_scholarship} onChange={(e) => set('wants_scholarship', e.target.checked)} />
                I want a regional (DSU) scholarship (Italy)
              </label>
            </div>
            <div className="interests">
              <span className="small muted">Subjects that interest you</span>
              <div className="chips">
                {FIELDS.map(([k, l]) => (
                  <button type="button" key={k} className={`chip ${form.interests.includes(k) ? 'on' : ''}`} aria-pressed={form.interests.includes(k)}
                    onClick={() => set('interests', form.interests.includes(k) ? form.interests.filter((x) => x !== k) : [...form.interests, k])}>{l}</button>
                ))}
              </div>
            </div>
          </fieldset>

          <fieldset className="card form-section">
            <legend>Education so far</legend>
            <div className="form-grid">
              <label>Highest education
                <select value={form.education_level} onChange={(e) => set('education_level', e.target.value)}>
                  <option value="">Choose…</option>
                  <option value="high_school">High school</option>
                  <option value="bachelor_in_progress">Bachelor's, still studying</option>
                  <option value="bachelor_done">Bachelor's, finished</option>
                  <option value="master_done">Master's, finished</option>
                </select>
              </label>
              <label>Years of school completed
                <select value={form.school_years} onChange={(e) => set('school_years', Number(e.target.value))}>
                  <option value={11}>11 years</option>
                  <option value={12}>12 years</option>
                </select>
              </label>
              <label>School grade average
                <div className="grade">
                  <input value={school.value} onChange={(e) => set('school_grade', e.target.value ? `${e.target.value}/${school.scale}` : '')} placeholder="e.g. 4.6" inputMode="decimal" />
                  <span>/</span>
                  <select value={school.scale} onChange={(e) => school.value && set('school_grade', `${school.value}/${e.target.value}`)}>
                    {['5', '10', '100'].map((s) => <option key={s}>{s}</option>)}
                  </select>
                </div>
              </label>
              {form.education_level && form.education_level !== 'high_school' && (
                <>
                  <label>University
                    <input value={form.university} onChange={(e) => set('university', e.target.value)} placeholder="e.g. Westminster International University in Tashkent" />
                  </label>
                  <label>Subject area
                    <select value={form.major} onChange={(e) => set('major', e.target.value)}>
                      <option value="">Choose…</option>
                      {FIELDS.map(([k, l]) => <option key={k} value={k}>{l}</option>)}
                    </select>
                  </label>
                  <label>Exact major
                    <input value={form.major_name} onChange={(e) => set('major_name', e.target.value)} placeholder="e.g. Software Engineering" />
                  </label>
                  <label>University GPA
                    <div className="grade">
                      <input value={gpa.value} onChange={(e) => set('gpa', e.target.value ? `${e.target.value}/${gpa.scale}` : '')} placeholder="e.g. 3.33" inputMode="decimal" />
                      <span>/</span>
                      <select value={gpa.scale} onChange={(e) => gpa.value && set('gpa', `${gpa.value}/${e.target.value}`)}>
                        {['4', '5', '10', '100'].map((s) => <option key={s}>{s}</option>)}
                      </select>
                    </div>
                  </label>
                  <label>University years completed
                    <select value={form.completed_university_years} onChange={(e) => set('completed_university_years', Number(e.target.value))}>
                      {[0, 1, 2, 3, 4].map((n) => <option key={n} value={n}>{n === 4 ? '4 or more' : n}</option>)}
                    </select>
                  </label>
                  <label>{form.education_level === 'bachelor_in_progress' ? 'Expected graduation year' : 'Graduation year'}
                    <input type="number" min="1990" max="2035" value={form.graduation_year || ''} onChange={(e) => set('graduation_year', num(e.target.value))} placeholder="e.g. 2026" />
                  </label>
                </>
              )}
            </div>
          </fieldset>

          <fieldset className="card form-section">
            <legend>Languages</legend>
            <div className="form-grid">
              <label>English certificate
                <select value={form.english_test} onChange={(e) => set('english_test', e.target.value)}>
                  <option value="">Not yet</option>
                  <option value="IELTS">IELTS Academic</option>
                  <option value="TOEFL">TOEFL iBT</option>
                  <option value="Duolingo">Duolingo English Test</option>
                  <option value="Cambridge">Cambridge (B2 First / C1 / C2)</option>
                </select>
              </label>
              <label>{form.english_test === 'Cambridge' ? 'Level' : 'Overall score'}
                {form.english_test === 'Cambridge' ? (
                  <select value={form.english_score} onChange={(e) => set('english_score', e.target.value)}>
                    <option value="">Choose…</option>
                    {['B2', 'C1', 'C2'].map((l) => <option key={l}>{l}</option>)}
                  </select>
                ) : (
                  <input value={form.english_score} onChange={(e) => set('english_score', e.target.value)} disabled={!form.english_test}
                    placeholder={{ IELTS: 'e.g. 6.5', TOEFL: 'e.g. 90', Duolingo: 'e.g. 115' }[form.english_test] || ''} inputMode="decimal" />
                )}
              </label>
              <label>Test date
                <input type="month" value={form.english_date} onChange={(e) => set('english_date', e.target.value)} disabled={!form.english_test} />
              </label>
              <label>Italian level
                <select value={form.italian_level} onChange={(e) => set('italian_level', e.target.value)}>
                  <option value="">None</option>
                  {['A1', 'A2', 'B1', 'B2', 'C1', 'C2'].map((l) => <option key={l}>{l}</option>)}
                </select>
              </label>
              <label>German level
                <select value={form.german_level} onChange={(e) => set('german_level', e.target.value)}>
                  <option value="">None</option>
                  {['A1', 'A2', 'B1', 'B2', 'C1', 'C2'].map((l) => <option key={l}>{l}</option>)}
                </select>
              </label>
              <label>My documents are in
                <select value={form.documents_language} onChange={(e) => set('documents_language', e.target.value)}>
                  <option value="uz">Uzbek</option>
                  <option value="ru">Russian</option>
                  <option value="en">English</option>
                  <option value="mixed">Mixed (some English, some Uzbek or Russian)</option>
                </select>
              </label>
            </div>
            <div className="other-langs">
              <span className="small muted">Other languages and certificates</span>
              {form.other_languages.map((l, i) => (
                <div className="lang-row" key={i}>
                  <input value={l.language} placeholder="Language, e.g. German" onChange={(e) => set('other_languages', form.other_languages.map((x, j) => (j === i ? { ...x, language: e.target.value } : x)))} />
                  <input value={l.cert} placeholder="Certificate, e.g. Goethe" onChange={(e) => set('other_languages', form.other_languages.map((x, j) => (j === i ? { ...x, cert: e.target.value } : x)))} />
                  <select value={l.level} onChange={(e) => set('other_languages', form.other_languages.map((x, j) => (j === i ? { ...x, level: e.target.value } : x)))}>
                    <option value="">Level</option>
                    {['A1', 'A2', 'B1', 'B2', 'C1', 'C2', 'Native'].map((x) => <option key={x}>{x}</option>)}
                  </select>
                  <button type="button" className="link" onClick={() => set('other_languages', form.other_languages.filter((_, j) => j !== i))} aria-label="Remove language">Remove</button>
                </div>
              ))}
              <button type="button" className="btn sm" onClick={() => set('other_languages', [...form.other_languages, { language: '', cert: '', level: '' }])}>+ Add a language</button>
            </div>
          </fieldset>

          <fieldset className="card form-section">
            <legend>Tests & experience</legend>
            <div className="form-grid">
              <label>SAT (if taken)
                <input type="number" min="400" max="1600" value={form.sat_score || ''} onChange={(e) => set('sat_score', num(e.target.value))} placeholder="400–1600" />
              </label>
              {master && (
                <>
                  <label>GRE total (if taken)
                    <input type="number" min="260" max="340" value={form.gre_score || ''} onChange={(e) => set('gre_score', num(e.target.value))} placeholder="260–340" />
                  </label>
                  <label>GMAT (if taken)
                    <input type="number" min="205" max="805" value={form.gmat_score || ''} onChange={(e) => set('gmat_score', num(e.target.value))} placeholder="205–805" />
                  </label>
                </>
              )}
              <label>Work experience (years)
                <input type="number" min="0" max="40" value={form.work_years} onChange={(e) => set('work_years', num(e.target.value))} />
              </label>
              <label className="check">
                <input type="checkbox" checked={!!form.has_portfolio} onChange={(e) => set('has_portfolio', e.target.checked)} />
                I have a portfolio of my work (design, architecture, art)
              </label>
            </div>
          </fieldset>

          <div className="save-bar">
            <button className="btn primary" disabled={status === 'saving'}>{status === 'saving' ? 'Saving…' : 'Save'}</button>
            {status === 'saved' && <span className="saved">✓ Saved</span>}
            {status && !['saving', 'saved'].includes(status) && <span className="error small">{status}</span>}
          </div>
        </form>
      )}

      <div className="card row between wrap">
        <div>
          <h3>See how you match</h3>
          <p className="small muted">Open any programme to see your match score and what to fix.</p>
        </div>
        <Link to="/programs" className="btn primary">Browse universities →</Link>
      </div>
    </main>
  );
}
