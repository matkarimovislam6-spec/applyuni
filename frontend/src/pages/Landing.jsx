import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api.js';
import Ascii from '../Ascii.jsx';
import Waitlist from '../Waitlist.jsx';
import { useT } from '../i18n.jsx';
import { COUNTRY } from './Programs.jsx';
import CountryMap from '../CountryMap.jsx';
import LogoMarquee from '../LogoMarquee.jsx';

// The headline word and the drawings around it change together, by country.
const PLACE_COUNTRIES = ['IT', 'DE', 'US', 'IT', 'DE', 'US'];

// Each language has its own headline grammar: Uzbek adds the locative "-da",
// Russian uses the prepositional case after "в".
const HEADLINE = {
  en: { places: ['Milan', 'Munich', 'the USA', 'Rome', 'Aachen', 'Illinois'], before: 'Study in', after: '', marker: 'without paying', rest: ' an agency.' },
  uz: { places: ['Milanda', 'Myunxenda', 'AQShda', 'Rimda', 'Axenda', 'Illinoysda'], before: '', after: "o'qing", marker: "agentlikka pul to'lamasdan", rest: '.' },
  ru: { places: ['Милане', 'Мюнхене', 'США', 'Риме', 'Ахене', 'Иллинойсе'], before: 'Учитесь в', after: '', marker: 'без переплаты', rest: ' агентству.' },
};

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
    const id = setInterval(() => setI((n) => (n + 1) % length), interval);
    return () => clearInterval(id);
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
  const { t, lang } = useT();
  const i = useRotation(PLACE_COUNTRIES.length);
  const [catalogue, setCatalogue] = useState([]);
  useEffect(() => {
    api.programs('').then(setCatalogue).catch(() => setCatalogue([]));
  }, []);
  const h = HEADLINE[lang] || HEADLINE.en;
  const word = h.places[i];
  const country = PLACE_COUNTRIES[i];
  const unis = new Set(catalogue.map((p) => p.university)).size;

  return (
    <main>
      <section className="hero">
        <CountryArt country={country} />
        <div className="container hero-inner">
          <p className="mono-label">{t('Italy')} · {t('Germany')} · {t('USA')} · {t('bachelor & master')}</p>
          <h1 className="display">
            {h.before && <>{h.before} </>}<RotatingWord word={word} />{h.after && <> {h.after}</>}
            <br />
            <span className="marker">{h.marker}</span>{h.rest}
          </h1>
          <p className="lead">
            {t('Agencies charge students')} <em>{t('thousands of dollars')}</em> {t('for paperwork you can do yourself.')}{' '}
            {t("Whether it's Italy, Germany or the USA, we tell you every document you need, check them with AI, and submit after you approve.")}
          </p>
          <HeroDemo programs={catalogue} />
        </div>
      </section>

      <section className="band">
        <div className="container band-inner">
          <span className="mono-label">{unis ? t('{u} universities · {p} programmes in the catalogue', { u: unis, p: catalogue.length }) : ''}</span>
        </div>
        <LogoMarquee />
      </section>

      <section id="how" className="container section">
        <p className="mono-label">{t('How it works')}</p>
        <h2 className="display-sm">{t('Three steps.')} <em>{t('You')}</em> {t('stay in charge.')}</h2>
        <div className="steps-grid">
          {[
            ['01', 'Choose', 'Pick the programmes you want. We show what each one actually asks for, with links to the official call.'],
            ['02', 'Collect', 'Upload your documents. AI checks every file: right document, readable, not expired, apostille where needed.'],
            ['03', 'Approve', "When everything is green you review once and approve. We submit, and you never press a button you didn't mean to."],
          ].map(([n, title, d]) => (
            <article key={n} className="step">
              <span className="step-n">{n}</span>
              <h3>{t(title)}</h3>
              <p>{t(d)}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="container section">
        <div className="compare">
          <div className="compare-head">
            <p className="mono-label">{t('The honest comparison')}</p>
            <h2 className="display-sm">{t('Same applications.')}<br /><em>{t('Not')}</em> {t('the same bill.')}</h2>
          </div>
          <table>
            <thead>
              <tr><th></th><th>{t('Typical agency')}</th><th className="us">Apply</th></tr>
            </thead>
            <tbody>
              {[
                ['Price', 'Thousands of dollars', 'Free checklist · low monthly plan'],
                ['Document list', 'Generic, by message', 'Exact, per programme and per student'],
                ['Document check', 'When a person has time', 'AI, within minutes'],
                ['Who submits', 'The agency, often without showing you', 'Only after your approval'],
                ['Your files', 'Telegram and email', 'Encrypted, deletable any time'],
              ].map(([a, b, c]) => <tr key={a}><td>{t(a)}</td><td>{t(b)}</td><td className="us">{t(c)}</td></tr>)}
            </tbody>
          </table>
        </div>
      </section>

      <section id="agencies" className="container section">
        <div className="duo">
          <article className="panel">
            <p className="mono-label">{t('For students')}</p>
            <h3 className="display-xs">{t('Get early access')}</h3>
            <p className="muted">{t('The checklist is free today. Document checks and one-click submission open soon.')}</p>
            <Waitlist cta={t('Join the list')} />
          </article>
          <article className="panel dark">
            <Ascii art="espresso" className="panel-ascii" />
            <p className="mono-label">{t('For agencies & schools')}</p>
            <h3 className="display-xs">{t('Handle 5× the students')}</h3>
            <p>{t('One dashboard for every student. AI does the checking, your team approves and submits. Priced per seat.')}</p>
            <Waitlist kind="agency" cta={t('Talk to us')} />
          </article>
        </div>
      </section>

      <section className="container section narrow">
        <p className="mono-label">{t('Questions')}</p>
        <h2 className="display-sm">{t('Things students ask us')}</h2>
        <div className="faq-list">
          {[
            ['Do I really not need an agency?', 'No, and you don\'t need to do the agency\'s work either. We register you, pay the application fees, submit after your approval and prepare your visa file. You only upload documents and go to the visa appointment in person.'],
            ['I finished 11 years of school. Can I apply for a bachelor?', 'It depends on the country. Italy needs 12 years, so with 11 you need a completed university year or a foundation year. Germany usually needs a Studienkolleg (foundation year) or completed university study. US universities accept a finished secondary school certificate. Your profile shows which applies to you.'],
            ["What do US universities need that Europe doesn't?", 'Essays, recommendation letters and proof of funds for your Form I-20. SAT or ACT is optional at many universities. Deadlines come earlier too: many close between December and January.'],
            ['Is studying in Germany really free?', 'Most public universities charge no tuition, but some states and universities charge non-EU students, and you need a blocked account for the visa. Each programme page shows the details we have verified.'],
            ['Will you write my motivation letter?', 'No. Universities can reject ghost-written letters. Our AI gives feedback on your own draft.'],
            ['Are my documents safe?', 'Every file is encrypted with its own key before it is stored, and you can delete everything at any time.'],
          ].map(([q, a]) => (
            <details key={q} className="faq">
              <summary>{t(q)}<span className="plus" aria-hidden="true">+</span></summary>
              <p>{t(a)}</p>
            </details>
          ))}
        </div>
      </section>
    </main>
  );
}

function HeroDemo({ programs }) {
  const { t } = useT();
  const [step, setStep] = useState(1);
  const [country, setCountry] = useState('');
  const [programId, setProgramId] = useState('');
  const [profile, setProfile] = useState({ school_years: 11, wants_scholarship: true, documents_language: 'uz', english_test: '' });
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');

  // Picking a country preselects a programme there you can apply to right now.
  const pickCountry = (c) => {
    setCountry(c);
    const here = programs.filter((p) => p.country === c);
    const live = (x) => ['open', 'due'].includes(x.deadline_status?.state);
    setProgramId((here.find((x) => live(x) && x.degree === 'bachelor') || here.find(live) || here[0])?.id || '');
    setStep(2);
  };

  const program = programs.find((p) => p.id === programId);
  const build = async () => {
    setError('');
    try {
      setResult(await api.checklist(programId, profile));
      setStep(4);
    } catch (e) {
      setError(e.message);
    }
  };
  const reset = () => { setStep(1); setCountry(''); setResult(null); };
  const degree = (d) => t(d === 'bachelor' ? 'Bachelor' : 'Master').toLowerCase();

  return (
    <div className="demo">
      <div className="demo-bar">
        <span className="dots"><i /><i /><i /></span>
        {['Country', 'Programme', 'Answer', 'Your list'].map((label, n) => (
          <span key={label} className={`demo-tab ${step === n + 1 ? 'on' : ''}`}>{n + 1} · {t(label)}</span>
        ))}
        <button className="link small push" onClick={reset}>{t('Start over')}</button>
      </div>
      <div className="demo-body">
        {error && <div className="error">{error}</div>}
        {step === 1 && (
          <>
            <h3>{t('Which country do you want to study in?')}</h3>
            <div className="country-pick">
              {['IT', 'DE', 'US'].map((c) => {
                const here = programs.filter((p) => p.country === c);
                return (
                  <button key={c} className="country-tile" onClick={() => pickCountry(c)} disabled={!here.length}>
                    <span className="country-disc"><CountryMap code={c} /></span>
                    <span className="country-name">{t(COUNTRY[c])}</span>
                    <span className="country-count">{t('{n} programmes', { n: here.length })}</span>
                  </button>
                );
              })}
            </div>
            <div className="demo-actions">
                            <span className="muted small">{t('No account. Nothing is saved.')}</span>
            </div>
          </>
        )}
        {step === 2 && (
          <>
            <h3>{t('Which programme?')}</h3>
            <select className="big-input" value={programId} onChange={(e) => setProgramId(e.target.value)}>
              {programs.filter((p) => p.country === country).map((p) => <option key={p.id} value={p.id}>{p.name} — {p.university} ({degree(p.degree)})</option>)}
            </select>
            <div className="demo-actions">
              <button className="link small" onClick={() => setStep(1)}>{t('← Country')}</button>
              <span className="hint">{t('click here →')}</span>
              <button className="btn primary" disabled={!programId} onClick={() => setStep(3)}>{t('Next')}</button>
            </div>
          </>
        )}
        {step === 3 && (
          <>
            <h3>{t('Two quick questions')}</h3>
            {program?.degree === 'bachelor' && (
              <div className="q">
                <span>{t('Years of school you finished')}</span>
                <div className="chips">
                  {[11, 12].map((y) => (
                    <button key={y} className={`chip ${profile.school_years === y ? 'on' : ''}`} onClick={() => setProfile({ ...profile, school_years: y })}>{t('{n} years', { n: y })}</button>
                  ))}
                </div>
              </div>
            )}
            {program?.country === 'IT' ? (
              <div className="q">
                <span>{t('Do you want a DSU scholarship?')}</span>
                <div className="chips">
                  {[[true, 'Yes, please'], [false, 'No']].map(([v, l]) => (
                    <button key={l} className={`chip ${profile.wants_scholarship === v ? 'on' : ''}`} onClick={() => setProfile({ ...profile, wants_scholarship: v })}>{t(l)}</button>
                  ))}
                </div>
              </div>
            ) : (
              <div className="q">
                <span>{t('What language are your documents in?')}</span>
                <div className="chips">
                  {[['uz', 'Uzbek'], ['ru', 'Russian'], ['en', 'English'], ['mixed', 'Mixed']].map(([v, l]) => (
                    <button key={v} className={`chip ${profile.documents_language === v ? 'on' : ''}`} onClick={() => setProfile({ ...profile, documents_language: v })}>{t(l)}</button>
                  ))}
                </div>
              </div>
            )}
            <div className="demo-actions">
              <span className="hint">{t('almost →')}</span>
              <button className="btn primary" onClick={build}>{t('Build my checklist')}</button>
            </div>
          </>
        )}
        {step === 4 && result && (
          <>
            <h3>
              {t('You need')} <span className="marker">{t('{n} documents', { n: result.checklist.items.length })}</span> {t('for {university}', { university: result.university })}
            </h3>
            {result.checklist.warnings[0] && <div className="notice small">{result.checklist.warnings[0]}</div>}
            <ol className="mini-list">
              {result.checklist.items.slice(0, 5).map((it) => <li key={it.type}>{it.title}</li>)}
              {result.checklist.items.length > 5 && <li className="more">{t('+ {n} more', { n: result.checklist.items.length - 5 })}</li>}
            </ol>
            <div className="demo-actions">
              <Link to={`/programs/${result.id}`} className="btn primary">{t('See the full list with explanations →')}</Link>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
