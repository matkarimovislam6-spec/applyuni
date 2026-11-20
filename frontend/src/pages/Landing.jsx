import { Link } from 'react-router-dom';
import Waitlist from '../Waitlist.jsx';

const STEPS = [
  ['1', 'Pick your universities', 'Browse English-taught programmes in Italy and choose where you want to apply.'],
  ['2', 'Get your exact document list', 'Answer a few questions. We list every document you need, based on your school, your English and whether you want a scholarship.'],
  ['3', 'Upload, review, approve', 'AI checks every document before it reaches the university. You approve each application with one click, and we submit it.'],
];

const FAQ = [
  ['Do I need an agency to study in Italy?', 'No. The process is long but not hard: university application, Universitaly pre-enrolment, visa and the DSU scholarship. ApplyUni walks you through every step.'],
  ['I finished 11 years of school in Uzbekistan. Can I apply for a bachelor?', 'Italy requires 12 years of schooling. With 11, you usually need at least one completed year of university or a recognised foundation year. Our checklist adds this automatically.'],
  ['Will you write my motivation letter?', 'No. Universities can reject ghost-written letters. Our AI gives feedback on your own draft.'],
  ['Are my documents safe?', 'Every file is encrypted with its own key before it is stored. You can delete everything at any time.'],
];

export default function Landing() {
  return (
    <main>
      <section className="hero">
        <div className="container">
          <span className="eyebrow">🇮🇹 Italy · bachelor & master</span>
          <h1>Apply to European universities yourself, without paying an agency.</h1>
          <p className="lead">
            Agencies charge students 10,000,000+ soums for paperwork you can do yourself. ApplyUni tells you exactly which documents you need,
            checks them with AI, and submits your applications after you approve.
          </p>
          <div className="row wrap">
            <Link to="/programs" className="btn primary big">Get my free document checklist</Link>
            <a href="#how" className="btn big">How it works</a>
          </div>
        </div>
      </section>

      <section id="how" className="container section">
        <h2>How it works</h2>
        <div className="grid three">
          {STEPS.map(([n, title, text]) => (
            <div key={n} className="card">
              <div className="step-num">{n}</div>
              <h3>{title}</h3>
              <p className="muted">{text}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="container section">
        <div className="grid two">
          <div className="card stack">
            <h3>For students</h3>
            <p className="muted">The checklist is free today. Document checks and one-click submission are coming soon. Join the list for early access.</p>
            <Waitlist cta="Get early access" />
          </div>
          <div className="card stack">
            <h3>For agencies & schools</h3>
            <p className="muted">Manage many students from one dashboard. AI checks the documents, and you approve and submit. Pay per seat.</p>
            <Waitlist kind="agency" cta="Talk to us" />
          </div>
        </div>
      </section>

      <section className="container section">
        <h2>Questions</h2>
        <div className="stack">
          {FAQ.map(([q, a]) => (
            <details key={q} className="card faq">
              <summary>{q}</summary>
              <p className="muted">{a}</p>
            </details>
          ))}
        </div>
      </section>
    </main>
  );
}
