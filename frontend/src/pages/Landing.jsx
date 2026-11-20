import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api.js';
import Ascii from '../Ascii.jsx';
import Waitlist from '../Waitlist.jsx';

// The headline word and the drawings around it change together, by country.
const PLACES = [
  ['Milan', 'IT'], ['Munich', 'DE'], ['the USA', 'US'],
  ['Rome', 'IT'], ['Aachen', 'DE'], ['Illinois', 'US'],
];

// Four drawings per country, one per corner slot (a1 top-left … a4 right).
const ART_SETS = {
  IT: ['duomo', 'pizza', 'espresso', 'pisa'],
  DE: ['brandenburg', 'tvtower', 'pretzel', 'beer'],
  US: ['empire', 'goldengate', 'burger', 'liberty'],
};

function useRotation(length, interval = 2600) {
  const [i, setI] = useState(0);
  useEffect(() => {
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
    const t = setInterval(() => setI((n) => (n + 1) % length), interval);
    return () => clearInterval(t);
  }, [length, interval]);
  return i;
}

function RotatingWord({ word }) {
  return (
    <span className="rotator" aria-live="polite">
      <span key={word} className="rotator-word">{word}</span>
    </span>
  );
}

function CountryArt({ country }) {
  return (
    <div key={country} className="art-set" aria-hidden="true">
      {ART_SETS[country].map((art, n) => <Ascii key={art} art={art} className={`float a${n + 1}`} />)}
    </div>
  );
}

export default function Landing() {
  const i = useRotation(PLACES.length);
  const [catalogue, setCatalogue] = useState([]);
  useEffect(() => {
    api.programs('').then(setCatalogue).catch(() => setCatalogue([]));
  }, []);
  // universities per country, straight from the catalogue
  const byCountry = [['IT', 'Italy'], ['DE', 'Germany'], ['US', 'USA']].map(([code, label]) => [
    label,
    [...new Set(catalogue.filter((p) => p.country === code).map((p) => p.university))].sort(),
  ]);
  const [word, country] = PLACES[i];
  return (
    <main>
      <section className="hero">
        <CountryArt country={country} />
        <div className="container hero-inner">
          <p className="mono-label">Italy · Germany · USA · bachelor & master</p>
          <h1 className="display">
            Study in <RotatingWord word={word} />
            <br />
            <span className="marker">without paying</span> an agency.
          </h1>
          <p className="lead">
            Agencies charge students <em>thousands of dollars</em> for paperwork you can do yourself.
            Whether it's Italy, Germany or the USA, we tell you every document you need, check them with AI, and submit after you approve.
          </p>
          <HeroDemo programs={catalogue} />
        </div>
      </section>

      <section className="band">
        <div className="container band-inner">
          <span className="mono-label">{new Set(catalogue.map((p) => p.university)).size || ''} universities · {catalogue.length || ''} programmes in the catalogue</span>
          <div className="logo-groups">
            {byCountry.filter(([, us]) => us.length).map(([c, us]) => (
              <div key={c} className="logo-group">
                <span className="mono-label">{c}</span>
                <div className="logos">{us.map((u) => <span key={u}>{u}</span>)}</div>
              </div>
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
              <tr><td>Price</td><td>Thousands of dollars</td><td className="us">Free checklist · low monthly plan</td></tr>
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
            ['Do I really not need an agency?', 'No. The process is long but not hard: the university application, then the visa steps for your country. We walk you through each one.'],
            ['I finished 11 years of school. Can I apply for a bachelor?', 'It depends on the country. Italy needs 12 years, so with 11 you need a completed university year or a foundation year. Germany usually needs a Studienkolleg (foundation year) or completed university study. US universities accept a finished secondary school certificate. Your profile shows which applies to you.'],
            ['What do US universities need that Europe doesn\'t?', 'Essays, recommendation letters and proof of funds for your Form I-20. SAT or ACT is optional at many universities. Deadlines come earlier too: many close between December and January.'],
            ['Is studying in Germany really free?', 'Most public universities charge no tuition, but some states and universities charge non-EU students, and you need a blocked account for the visa. Each programme page shows the details we have verified.'],
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

function HeroDemo({ programs }) {
  const [step, setStep] = useState(1);
  const [programId, setProgramId] = useState('');
  const [profile, setProfile] = useState({ school_years: 11, wants_scholarship: true, documents_language: 'uz', english_test: '' });
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!programId && programs.length) {
      // start on a programme you can apply to right now
      const live = (x) => ['open', 'due'].includes(x.deadline_status?.state);
      setProgramId((programs.find((x) => live(x) && x.degree === 'bachelor') || programs.find(live) || programs[0]).id);
    }
  }, [programs, programId]);

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
              {[['IT', 'Italy'], ['DE', 'Germany'], ['US', 'USA']].map(([c, label]) => (
                <optgroup key={c} label={label}>
                  {programs.filter((p) => p.country === c).map((p) => <option key={p.id} value={p.id}>{p.name} — {p.university} ({p.degree})</option>)}
                </optgroup>
              ))}
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
            {program?.country === 'IT' ? (
              <div className="q">
                <span>Do you want a DSU scholarship?</span>
                <div className="chips">
                  {[[true, 'Yes, please'], [false, 'No']].map(([v, l]) => (
                    <button key={l} className={`chip ${profile.wants_scholarship === v ? 'on' : ''}`} onClick={() => setProfile({ ...profile, wants_scholarship: v })}>{l}</button>
                  ))}
                </div>
              </div>
            ) : (
              <div className="q">
                <span>What language are your documents in?</span>
                <div className="chips">
                  {[['uz', 'Uzbek'], ['ru', 'Russian'], ['en', 'English'], ['mixed', 'Mixed']].map(([v, l]) => (
                    <button key={v} className={`chip ${profile.documents_language === v ? 'on' : ''}`} onClick={() => setProfile({ ...profile, documents_language: v })}>{l}</button>
                  ))}
                </div>
              </div>
            )}
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
