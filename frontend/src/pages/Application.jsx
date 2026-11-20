import { useCallback, useEffect, useState } from 'react';
import { Link, Navigate, useParams } from 'react-router-dom';
import { api } from '../api.js';
import { useAuth } from '../auth.jsx';
import { useT } from '../i18n.jsx';

// One application: upload each required document, approve once, then follow
// what the submission engine does on the university portal.

const ITEM_STATUS = {
  missing: ['Not uploaded', ''],
  uploaded: ['Uploaded', ''],
  analyzing: ['Checking…', ''],
  needs_review: ['Waiting for review', 'warn-text'],
  accepted: ['Accepted', 'ok-text'],
  rejected: ['Needs a new file', 'urgent-text'],
};

export const APP_STATUS = {
  collecting_documents: 'Collecting documents',
  ready_for_review: 'Ready for your approval',
  approved: 'Approved: we are preparing the submission',
  submitting: 'Submitting to the university',
  submitted: 'Submitted',
  needs_attention: 'Needs attention',
};

// Statuses where something is still happening in the background.
const BUSY = new Set(['approved', 'submitting']);

export default function Application() {
  const { t, locale } = useT();
  const { id } = useParams();
  const { user } = useAuth();
  const [app, setApp] = useState(null);
  const [docs, setDocs] = useState({});
  const [error, setError] = useState('');
  const [busyType, setBusyType] = useState('');
  const [confirm, setConfirm] = useState(false);
  const [approving, setApproving] = useState(false);

  const load = useCallback(async () => {
    try {
      const a = await api.application(id);
      setApp(a);
      const list = await api.documents(a.student_id);
      setDocs(Object.fromEntries(list.map((d) => [d.id, d])));
    } catch (e) {
      setError(e.message);
    }
  }, [id]);

  useEffect(() => { if (user) load(); }, [user, load]);

  // Refresh while documents are being checked or the engine is working.
  const pending = app && (BUSY.has(app.status) || app.checklist.items.some((i) => i.status === 'analyzing' || i.status === 'uploaded'));
  useEffect(() => {
    if (!pending) return undefined;
    const timer = setInterval(load, 4000);
    return () => clearInterval(timer);
  }, [pending, load]);

  if (user === undefined) return <main className="container section muted">{t('Loading…')}</main>;
  if (!user) return <Navigate to="/login" state={{ from: `/applications/${id}` }} replace />;
  if (error && !app) return <main className="container section"><div className="error">{error}</div></main>;
  if (!app) return <main className="container section muted">{t('Loading…')}</main>;

  const upload = async (type, file) => {
    if (!file) return;
    setBusyType(type);
    setError('');
    try {
      await api.uploadDocument(app.student_id, type, file);
      await load();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusyType('');
    }
  };

  const approve = async () => {
    setApproving(true);
    setError('');
    try {
      setApp(await api.approve(app.id));
    } catch (e) {
      setError(e.message);
    } finally {
      setApproving(false);
    }
  };

  const p = app.program;
  const items = app.checklist.items;
  const accepted = items.filter((i) => i.status === 'accepted').length;
  const locked = !['collecting_documents', 'ready_for_review', 'needs_attention'].includes(app.status);

  return (
    <main className="container section stack">
      <Link to="/account" className="mono-label back">{t('← My account')}</Link>
      <div className="row between wrap">
        <div>
          <p className="mono-label">{t('Application')}</p>
          <h1 className="display-sm">{p.name}</h1>
          <p className="muted">{p.university} · {p.city}</p>
        </div>
        <span className="badge">{t(APP_STATUS[app.status] || app.status)}</span>
      </div>

      <Progress app={app} />

      {app.status_note && <div className="notice">{app.status_note}</div>}
      {app.portal_ref && <div className="success">{t('University reference: {ref}', { ref: app.portal_ref })}</div>}
      {app.decision && <div className="success">{t('Decision: {d}', { d: app.decision })}</div>}
      {error && <div className="error">{error}</div>}

      <section className="card stack">
        <div className="row between">
          <h2>{t('Your documents')}</h2>
          <span className="badge">{t('{a} of {n} accepted', { a: accepted, n: items.length })}</span>
        </div>
        <ol className="doclist">
          {items.map((it) => {
            const [label, cls] = ITEM_STATUS[it.status] || [it.status, ''];
            const doc = it.document_id ? docs[it.document_id] : null;
            const issues = [...(doc?.verdict?.issues || []), ...(doc?.verdict?.cross || [])];
            const canUpload = !locked && it.status !== 'accepted';
            return (
              <li key={it.type} className="stack">
                <div className="row between wrap">
                  <strong>{it.title}</strong>
                  <span className={`small ${cls}`}>{t(label)}</span>
                </div>
                <p className="small muted">{it.description}</p>
                {doc && <p className="small">{doc.filename}</p>}
                {issues.map((x) => <p key={x} className="small urgent-text">{x}</p>)}
                {canUpload && (
                  <label className="btn small">
                    {busyType === it.type ? t('Uploading…') : doc ? t('Replace file') : t('Upload file')}
                    <input
                      type="file" hidden accept=".pdf,.jpg,.jpeg,.png"
                      disabled={!!busyType}
                      onChange={(e) => { upload(it.type, e.target.files[0]); e.target.value = ''; }}
                    />
                  </label>
                )}
              </li>
            );
          })}
        </ol>
      </section>

      {app.status === 'ready_for_review' && (
        <section className="card stack cta">
          <h2>{t('Approve and submit')}</h2>
          <p className="small">{t('We will register you on the {university} portal and submit exactly these documents and your profile. Nothing is sent before you approve.', { university: p.university })}</p>
          <label className="check">
            <input type="checkbox" checked={confirm} onChange={(e) => setConfirm(e.target.checked)} />
            {t('I confirm my details and documents are correct, and I authorise ApplyUni to apply on my behalf.')}
          </label>
          <button className="btn" disabled={!confirm || approving} onClick={approve}>
            {approving ? t('Approving…') : t('Approve application')}
          </button>
        </section>
      )}

      {app.events?.length > 0 && (
        <section className="card stack">
          <h2>{t('Timeline')}</h2>
          <ol className="steps">
            {app.events.map((e, i) => (
              <li key={i}>
                <span className="small muted">{new Date(e.at).toLocaleString(locale)}</span>{' '}
                <strong>{t(APP_STATUS[e.kind] || e.kind)}</strong>
                {e.note && <p className="small">{e.note}</p>}
              </li>
            ))}
          </ol>
        </section>
      )}
    </main>
  );
}

// The five stages every application goes through, with the current one
// explained in plain words so the applicant always knows what happens next.
function Progress({ app }) {
  const { t } = useT();
  const items = app.checklist.items;
  const allUploaded = items.every((i) => !['missing', 'rejected'].includes(i.status));
  const rejected = items.filter((i) => i.status === 'rejected').length;
  const waiting = items.filter((i) => ['uploaded', 'analyzing', 'needs_review'].includes(i.status)).length;
  const s = app.status;

  let current;
  if (app.decision) current = 5;
  else if (s === 'submitted') current = 5;
  else if (s === 'approved' || s === 'submitting') current = 4;
  else if (s === 'ready_for_review') current = 3;
  else if (allUploaded) current = 2;
  else current = 1;

  const stages = [
    ['Upload documents', 'Upload every document in the list below.'],
    ['Document check', 'We check each file: right document, readable, valid, and consistent with your other documents.'],
    ['Your approval', 'Review everything once and approve. Nothing is sent to the university before this.'],
    ['We apply for you', 'We create your account on the university portal, fill in the application and upload your documents.'],
    ['University decision', 'The university reviews your application. We tell you as soon as its answer arrives.'],
  ];

  let now;
  if (current === 1) now = rejected
    ? t('{n} document(s) need a new file. See the notes below.', { n: rejected })
    : t('Upload the remaining documents to continue.');
  else if (current === 2) now = t('All documents are uploaded. {n} are waiting to be checked. You don’t need to do anything: when every document is accepted, the approval step opens here.', { n: waiting });
  else if (current === 3) now = t('Everything is accepted. Please review and approve below.');
  else if (current === 4) now = t('Approved. We are applying on the university portal. Follow the timeline below.');
  else now = app.decision ? t('The university has answered.') : t('Submitted. Universities usually answer within a few weeks; we will notify you.');

  return (
    <section className="card stack">
      <h2>{t('What happens next')}</h2>
      <ol className="progress-steps">
        {stages.map(([title, desc], i) => {
          const n = i + 1;
          const state = n < current || (n === 5 && app.decision) ? 'done' : n === current ? 'now' : 'next';
          return (
            <li key={title} className={`ps ${state}`}>
              <span className="ps-n">{state === 'done' ? '✓' : n}</span>
              <div>
                <strong>{t(title)}</strong>
                <p className="small muted">{t(desc)}</p>
              </div>
            </li>
          );
        })}
      </ol>
      <div className={`notice small`}>{now}</div>
    </section>
  );
}
