import { useState } from 'react';
import { api } from './api.js';
import { useT } from './i18n.jsx';

export default function Waitlist({ programId = null, kind = 'student', cta }) {
  const { t } = useT();
  const [email, setEmail] = useState('');
  const [state, setState] = useState('idle'); // idle | busy | done
  const [error, setError] = useState('');

  const submit = async (e) => {
    e.preventDefault();
    setError('');
    setState('busy');
    try {
      await api.joinWaitlist(email, kind, programId);
      setState('done');
    } catch (err) {
      setError(err.message);
      setState('idle');
    }
  };

  if (state === 'done') return <p className="success">{t("✓ You're on the list. We'll email you when it opens.")}</p>;
  return (
    <form className="waitlist" onSubmit={submit}>
      <input type="email" required placeholder="your@email.com" value={email} onChange={(e) => setEmail(e.target.value)} aria-label={t('Email')} />
      <button className="btn primary" disabled={state === 'busy'}>{state === 'busy' ? t('Sending…') : cta || t('Notify me')}</button>
      {error && <span className="error small">{error}</span>}
    </form>
  );
}
