import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api.js';
import Ascii from '../Ascii.jsx';
import Waitlist from '../Waitlist.jsx';

const CITIES = ['Milan', 'Bologna', 'Rome', 'Padova', 'Turin'];

function RotatingWord({ words, interval = 2200 }) {
  const [i, setI] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setI((n) => (n + 1) % words.length), interval);
    return () => clearInterval(t);
  }, [words.length, interval]);
  return (
    <span className="rotator" aria-live="polite">
      <span key={words[i]} className="rotator-word">{words[i]}</span>
    </span>
  );
}

export default function Landing() {
  return (
    <main>
      <section className="hero">
        <Ascii art="duomo" className="float a1" />
        <Ascii art="pizza" className="float a2" />
        <Ascii art="espresso" className="float a3" />
        <Ascii art="pisa" className="float a4" />
        <div className="container hero-inner">
          <p className="mono-label">Italy · bachelor & master · 2027 intake</p>
          <h1 className="display">
            Study in <RotatingWord words={CITIES} />
            <br />
            <span className="marker">without paying</span> an agency.
          </h1>
          <p className="lead">
            Agencies charge Uzbek students <em>10,000,000+ soums</em> for paperwork you can do yourself.
            We tell you every document you need, check them with AI, and submit after you approve.
          </p>
          <HeroDemo />
        </div>
      </section>

      <section className="band">
        <div className="container band-inner">
          <span className="mono-label">Universities in the catalogue</span>
          <div className="logos">
            {['Politecnico di Milano', 'Università di Bologna', 'Sapienza Roma', 'Università di Padova', 'Politecnico di Torino', 'Università degli Studi di Milano'].map((u) => (
              <span key={u}>{u}</span>
            ))}
          </div>
        </div>
      </section>

      <section id="how" className="container section">
        <p className="mono-label">How it works</p>
        <h2 className="display-sm">Three steps. <em>You</em> stay in charge.</h2>
        <div className="steps-grid">
          {[
            ['01', 'Choose', 'Pick the programmes you want. We show what each one actually asks for, with links to the official call.'],
            ['02', 'Collect', 'Upload your documents. AI checks every file: right document, readable, not expired, apostille where needed.'],
            ['03', 'Approve', 'When everything is green you review once and approve. We submit, and you never press a button you didn\'t mean to.'],
          ].map(([n, t, d]) => (
            <article key={n} className="step">
              <span className="step-n">{n}</span>
              <h3>{t}</h3>
              <p>{d}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="container section">
        <div className="compare">
          <div className="compare-head">
            <p className="mono-label">The honest comparison</p>
            <h2 className="display-sm">Same applications.<br /><em>Not</em> the same bill.</h2>
          </div>
          <table>
            <thead>
              <tr><th></th><th>Typical agency</th><th className="us">ApplyUni</th></tr>
            </thead>
            <tbody>
              <tr><td>Price</td><td>10–15 million soums</td><td className="us">Free checklist · low monthly plan</td></tr>
              <tr><td>Document list</td><td>Generic, by message</td><td className="us">Exact, per programme and per student</td></tr>
              <tr><td>Document check</td><td>When a person has time</td><td className="us">AI, within minutes</td></tr>
              <tr><td>Who submits</td><td>The agency, often without showing you</td><td className="us">Only after your approval</td></tr>
              <tr><td>Your files</td><td>Telegram and email</td><td className="us">Encrypted, deletable any time</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      <section id="agencies" className="container section">
        <div className="duo">
          <article className="panel">
            <p className="mono-label">For students</p>
            <h3 className="display-xs">Get early access</h3>
            <p className="muted">The checklist is free today. Document checks and one-click submission open soon.</p>
            <Waitlist cta="Join the list" />
          </article>
          <article className="panel dark">
            <Ascii art="espresso" className="panel-ascii" />
            <p className="mono-label">For agencies & schools</p>
            <h3 className="display-xs">Handle 5× the students</h3>
            <p>One dashboard for every student. AI does the checking, your team approves and submits. Priced per seat.</p>
            <Waitlist kind="agency" cta="Talk to us" />
          </article>
        </div>
      </section>

      <section className="container section narrow">
        <p className="mono-label">Questions</p>
        <h2 className="display-sm">Things students ask us</h2>
        <div className="faq-list">
          {[
            ['Do I really not need an agency?', 'No. The process is long but not hard: university application, Universitaly pre-enrolment, visa and the DSU scholarship. We walk you through each step.'],
            ['I finished 11 years of school. Can I apply for a bachelor?', 'Italy requires 12 years of schooling. With 11, you usually need at least one completed year of university or a recognised foundation year. The checklist adds this for you.'],
            ['Will you write my motivation letter?', 'No. Universities can reject ghost-written letters. Our AI gives feedback on your own draft.'],
            ['Are my documents safe?', 'Every file is encrypted with its own key before it is stored, and you can delete everything at any time.'],
          ].map(([q, a]) => (
            <details key={q} className="faq">
              <summary>{q}<span className="plus" aria-hidden="true">+</span></summary>
              <p>{a}</p>
            </details>
          ))}
        </div>
      </section>
    </main>
  );
}

function HeroDemo() {
  const [step, setStep] = useState(1);
  const [programs, setPrograms] = useState([]);
  const [programId, setProgramId] = useState('');
  const [profile, setProfile] = useState({ school_years: 11, wants_scholarship: true, documents_language: 'uz', english_test: '' });
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');

  useEffect(() => {
    api.programs('IT').then((p) => {
      setPrograms(p);
      setProgramId(p.find((x) => x.degree === 'bachelor')?.id || p[0]?.id || '');
    }).catch((e) => setError(e.message));
  }, []);

  const program = programs.find((p) => p.id === programId);
  const build = async () => {
    setError('');
    try {
      setResult(await api.checklist(programId, profile));
      setStep(3);
    } catch (e) {
      setError(e.message);
    }
  };
  const reset = () => { setStep(1); setResult(null); };

  return (
    <div className="demo">
      <div className="demo-bar">
        <span className="dots"><i /><i /><i /></span>
        {['Choose', 'Answer', 'Your list'].map((t, i) => (
          <span key={t} className={`demo-tab ${step === i + 1 ? 'on' : ''}`}>{i + 1} · {t}</span>
        ))}
        <button className="link small push" onClick={reset}>Start over</button>
      </div>
      <div className="demo-body">
        {error && <div className="error">{error}</div>}
        {step === 1 && (
          <>
            <h3>Where would you like to study?</h3>
            <select className="big-input" value={programId} onChange={(e) => setProgramId(e.target.value)}>
              {programs.map((p) => <option key={p.id} value={p.id}>{p.name} — {p.university} ({p.degree})</option>)}
            </select>
            <div className="demo-actions">
              <span className="hint">click here →</span>
              <button className="btn primary" disabled={!programId} onClick={() => setStep(2)}>Next</button>
              <span className="muted small">No account. Nothing is saved.</span>
            </div>
          </>
        )}
        {step === 2 && (
          <>
            <h3>Two quick questions</h3>
            {program?.degree === 'bachelor' && (
              <div className="q">
                <span>Years of school you finished</span>
                <div className="chips">
                  {[11, 12].map((y) => (
                    <button key={y} className={`chip ${profile.school_years === y ? 'on' : ''}`} onClick={() => setProfile({ ...profile, school_years: y })}>{y} years</button>
                  ))}
                </div>
              </div>
            )}
            <div className="q">
              <span>Do you want a DSU scholarship?</span>
              <div className="chips">
                {[[true, 'Yes, please'], [false, 'No']].map(([v, l]) => (
                  <button key={l} className={`chip ${profile.wants_scholarship === v ? 'on' : ''}`} onClick={() => setProfile({ ...profile, wants_scholarship: v })}>{l}</button>
                ))}
              </div>
            </div>
            <div className="demo-actions">
              <span className="hint">almost →</span>
              <button className="btn primary" onClick={build}>Build my checklist</button>
            </div>
          </>
        )}
        {step === 3 && result && (
          <>
            <h3>
              You need <span className="marker">{result.checklist.items.length} documents</span> for {result.university}
            </h3>
            {result.checklist.warnings[0] && <div className="notice small">{result.checklist.warnings[0]}</div>}
            <ol className="mini-list">
              {result.checklist.items.slice(0, 5).map((it) => <li key={it.type}>{it.title}</li>)}
              {result.checklist.items.length > 5 && <li className="more">+ {result.checklist.items.length - 5} more</li>}
            </ol>
            <div className="demo-actions">
              <Link to={`/programs/${result.id}`} className="btn primary">See the full list with explanations →</Link>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
