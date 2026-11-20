import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { api } from '../api.js';
import { useMe } from '../App.jsx';
import { StatusBadge } from '../components.jsx';

export default function Application() {
  const { id } = useParams();
  const { me } = useMe();
  const [app, setApp] = useState(null);
  const [docs, setDocs] = useState([]);
  const [error, setError] = useState('');

  const load = async () => {
    try {
      const a = await api.application(id);
      setApp(a);
      setDocs(await api.documents(a.student_id));
    } catch (e) {
      setError(e.message);
    }
  };
  useEffect(() => {
    load();
  }, [id]);

  // Poll while the AI or the portal agent is working.
  const busy = app && (['approved', 'submitting'].includes(app.status) || app.checklist.items.some((i) => i.status === 'analyzing'));
  useEffect(() => {
    if (!busy) return;
    const t = setInterval(load, 3000);
    return () => clearInterval(t);
  }, [busy]);

  if (error) return <div className="error">{error}</div>;
  if (!app) return <p className="muted">Loading…</p>;

  const docById = Object.fromEntries(docs.map((d) => [d.id, d]));
  const cl = app.checklist;
  const done = cl.items.filter((i) => i.status === 'accepted').length;
  const p = app.program;

  return (
    <div className="stack">
      <Link to={`/students/${app.student_id}`} className="small">← Back</Link>
      <div className="row between wrap">
        <div>
          <h1>{p.university}</h1>
          <p className="muted">{p.name} · {p.degree} · {p.city}</p>
        </div>
        <StatusBadge status={app.status} />
      </div>

      <div className="card row wrap small">
        <span>Deadline: {p.deadline ? new Date(p.deadline).toLocaleDateString() : <span className="warn-text">not verified yet — check the official call</span>}</span>
        <span>Application fee: {p.application_fee_eur != null ? `€${p.application_fee_eur}` : '—'}</span>
        <span>{p.verified_at ? `Verified ${new Date(p.verified_at).toLocaleDateString()}` : 'Requirements not yet verified'}</span>
        <a href={p.source_url || p.website} target="_blank" rel="noreferrer">Official source ↗</a>
      </div>

      {cl.warnings.map((w) => <div key={w} className="notice">{w}</div>)}
      {app.status_note && <div className={app.status === 'needs_attention' ? 'error' : 'notice'}>{app.status_note}</div>}

      <ApprovePanel app={app} onApproved={setApp} />

      <section className="card stack">
        <div className="row between">
          <h2>Documents</h2>
          <span className="small muted">{done} / {cl.items.length} accepted</span>
        </div>
        <div className="progress"><div style={{ width: `${(done / Math.max(cl.items.length, 1)) * 100}%` }} /></div>
        {cl.items.map((item) => (
          <ChecklistItem key={item.type} item={item} doc={docById[item.document_id]} studentId={app.student_id} staff={me.role === 'owner' || me.role === 'counselor'} onChange={load} />
        ))}
      </section>

      <section className="card">
        <h2>Next steps after documents</h2>
        <ol className="steps">{cl.steps.map((s) => <li key={s.key}>{s.title}</li>)}</ol>
      </section>
    </div>
  );
}

function ChecklistItem({ item, doc, studentId, staff, onChange }) {
  const input = useRef();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const issues = doc?.ai_result?.issues || [];

  const upload = async (e) => {
    const file = e.target.files[0];
    if (!file) return;
    setBusy(true);
    setError('');
    try {
      await api.upload(studentId, item.type, file);
      onChange();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
      e.target.value = '';
    }
  };
  const review = async (accept) => {
    const note = accept ? '' : prompt('What is wrong with this document?') || '';
    await api.review(doc.id, accept, note);
    onChange();
  };

  return (
    <div className="item">
      <div className="item-main">
        <div className="row between wrap">
          <strong>{item.title}</strong>
          <StatusBadge status={item.status} />
        </div>
        <p className="small muted">{item.description}</p>
        {doc && <p className="small">📄 {doc.filename} <button className="link" onClick={() => api.download(doc.id)}>download</button></p>}
        {issues.length > 0 && <ul className="issues">{issues.map((i) => <li key={i}>{i}</li>)}</ul>}
        {error && <div className="error small">{error}</div>}
      </div>
      <div className="item-actions">
        <input ref={input} type="file" accept="application/pdf,image/jpeg,image/png,image/webp" hidden onChange={upload} />
        <button className="btn" disabled={busy} onClick={() => input.current.click()}>
          {busy ? 'Encrypting…' : item.status === 'missing' ? 'Upload' : 'Replace'}
        </button>
        {staff && doc && item.status !== 'accepted' && (
          <>
            <button className="btn ok" onClick={() => review(true)}>Accept</button>
            <button className="btn ghost" onClick={() => review(false)}>Reject</button>
          </>
        )}
      </div>
    </div>
  );
}

function ApprovePanel({ app, onApproved }) {
  const [agree, setAgree] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  if (app.approval) {
    return (
      <div className="card approved">
        <strong>✓ Approved by {app.approval.approved_by}</strong> on {new Date(app.approval.approved_at).toLocaleString()}
        {app.portal_ref && <div className="small">Portal reference: <span className="mono">{app.portal_ref}</span></div>}
        <div className="small muted mono">Snapshot {app.approval.snapshot_sha256.slice(0, 16)}…</div>
      </div>
    );
  }
  if (app.status !== 'ready_for_review') return null;

  const approve = async () => {
    setBusy(true);
    setError('');
    try {
      onApproved(await api.approve(app.id));
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="card approve stack">
      <h2>Everything is ready. Review and approve.</h2>
      <p className="small">
        After you approve, our agent creates your account on the {app.program.university} portal, fills in the forms with your profile, uploads the
        documents listed below exactly as they are now, and submits. If any document changes later, it will ask you again.
      </p>
      <label className="check">
        <input type="checkbox" checked={agree} onChange={(e) => setAgree(e.target.checked)} />
        I confirm my information and documents are true and complete, and I authorise ApplyUni to submit this application on my behalf.
      </label>
      {error && <div className="error">{error}</div>}
      <button className="btn primary big" disabled={!agree || busy} onClick={approve}>{busy ? 'Approving…' : 'Approve & submit'}</button>
    </div>
  );
}
