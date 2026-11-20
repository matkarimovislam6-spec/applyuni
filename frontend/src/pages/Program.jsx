import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { api } from '../api.js';
import Waitlist from '../Waitlist.jsx';
import { useAuth } from '../auth.jsx';
import { DeadlineFact, FeeFact, Rounds } from '../Deadline.jsx';
import { CheckList, ScoreRing, VERDICTS } from '../Match.jsx';

const DEFAULT = { school_years: 11, completed_university_years: 0, english_test: '', documents_language: 'uz', wants_scholarship: true };

export default function Program() {
  const { id } = useParams();
  const { user, profile: saved, saveProfile } = useAuth();
  const [profile, setProfile] = useState(DEFAULT);
  const [data, setData] = useState(null);
  const [error, setError] = useState('');
  const [savedNote, setSavedNote] = useState('');

  // Logged in: start from the answers saved to the account.
  useEffect(() => {
    if (saved) setProfile((p) => ({ ...p, ...saved }));
  }, [saved]);

  // Logged in: the pre-screening result for this programme.
  const [match, setMatch] = useState(null);
  useEffect(() => {
    if (!user) return setMatch(null);
    api.matchProgram(id).then(setMatch).catch(() => setMatch(null));
  }, [user, id, saved]);

  // Recompute the checklist whenever an answer changes.
  useEffect(() => {
    api.checklist(id, profile).then(setData).catch((e) => setError(e.message));
  }, [id, profile]);

  const set = (k, v) => {
    const next = { ...profile, [k]: v };
    setProfile(next);
    if (user) {
      // remember the change on the account
      saveProfile({ ...(saved || {}), ...next })
        .then(() => setSavedNote('✓ Saved to your account'))
        .catch(() => setSavedNote('Could not save'));
    }
  };

  if (error) return <main className="container section"><div className="error">{error}</div></main>;
  if (!data) return <main className="container section muted">Loading…</main>;

  const cl = data.checklist;
  return (
    <main className="container section stack">
      <Link to="/programs" className="mono-label back">← All programmes</Link>
      <div>
        <p className="mono-label">{data.university} · {data.city}</p>
        <h1 className="display-sm">{data.name}</h1>
        <p className="lead">{data.degree === 'bachelor' ? 'Bachelor' : 'Master'} · {data.language === 'en' ? 'taught in English' : 'taught in Italian'}</p>
      </div>

      <div className="facts">
        <DeadlineFact program={data} />
        <FeeFact program={data} />
        <div className="dl-fact"><span className="mono-label">Source</span><a href={data.source_url || data.call_source_url || data.website} target="_blank" rel="noreferrer">Official website ↗</a></div>
      </div>

      {user && match && (
        <section className={`card your-match ${VERDICTS[match.verdict]?.tone}`}>
          <div className="match-head">
            <ScoreRing score={match.score} tone={VERDICTS[match.verdict]?.tone} />
            <div className="match-title">
              <span className="mono-label">Your match</span>
              <strong className="verdict-big">{VERDICTS[match.verdict]?.label}</strong>
            </div>
            <Link to="/account" className="small">Edit profile →</Link>
          </div>
          <CheckList checks={match.checks} />
        </section>
      )}
      {!user && (
        <div className="notice small"><Link to="/signup" state={{ from: `/programs/${id}` }}>Create an account</Link> to see how well you match this programme and what you'd need to fix.</div>
      )}

      <Rounds program={data} />

      <div className="layout">
        <section className="card stack">
          <h2>About you</h2>
          {user ? (
            <p className="small muted">{savedNote || 'Your saved answers, filled in. Changes are saved to your account.'}</p>
          ) : (
            <p className="small muted">Your answers change the list. <Link to="/signup" state={{ from: `/programs/${id}` }}>Sign up</Link> to save them.</p>
          )}
          {data.degree === 'bachelor' && (
            <>
              <label>Years of school completed
                <select value={profile.school_years} onChange={(e) => set('school_years', Number(e.target.value))}>
                  <option value={11}>11 years</option>
                  <option value={12}>12 years</option>
                </select>
              </label>
              {profile.school_years < 12 && (
                <label>University years completed
                  <select value={profile.completed_university_years} onChange={(e) => set('completed_university_years', Number(e.target.value))}>
                    <option value={0}>None</option>
                    <option value={1}>1 year or more</option>
                  </select>
                </label>
              )}
            </>
          )}
          <label>English certificate
            <select value={profile.english_test} onChange={(e) => set('english_test', e.target.value)}>
              <option value="">Not yet</option>
              <option value="IELTS">IELTS</option>
              <option value="TOEFL">TOEFL</option>
              <option value="Duolingo">Duolingo</option>
            </select>
          </label>
          <label>My documents are in
            <select value={profile.documents_language} onChange={(e) => set('documents_language', e.target.value)}>
              <option value="uz">Uzbek</option>
              <option value="ru">Russian</option>
              <option value="en">English</option>
              <option value="mixed">Mixed (some English, some Uzbek or Russian)</option>
            </select>
          </label>
          <label className="check">
            <input type="checkbox" checked={profile.wants_scholarship} onChange={(e) => set('wants_scholarship', e.target.checked)} />
            I want a regional (DSU) scholarship
          </label>
        </section>

        <section className="stack">
          {cl.warnings.map((w) => <div key={w} className="notice">{w}</div>)}
          <div className="card stack">
            <div className="row between">
              <h2>Your documents</h2>
              <span className="badge">{cl.items.length} documents</span>
            </div>
            <ol className="doclist">
              {cl.items.map((it) => (
                <li key={it.type}>
                  <strong>{it.title}</strong>
                  <p className="small muted">{it.description}</p>
                </li>
              ))}
            </ol>
          </div>
          <div className="card">
            <h2>After the documents</h2>
            <ol className="steps">{cl.steps.map((s) => <li key={s.key}>{s.title}</li>)}</ol>
          </div>
          <div className="card stack cta">
            <h3>Want us to handle the rest?</h3>
            <p className="small">Upload your documents, let AI check them, approve once, and we submit to {data.university}. Coming soon.</p>
            <Waitlist programId={data.id} cta="Notify me" />
          </div>
        </section>
      </div>
    </main>
  );
}
