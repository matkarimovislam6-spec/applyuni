import { useState } from 'react';
import { api } from './api.js';

export default function Waitlist({ programId = null, kind = 'student', cta = 'Notify me' }) {
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

  if (state === 'done') return <p className="success">✓ You're on the list. We'll email you when it opens.</p>;
  return (
    <form className="waitlist" onSubmit={submit}>
      <input type="email" required placeholder="your@email.com" value={email} onChange={(e) => setEmail(e.target.value)} aria-label="Email" />
      <button className="btn primary" disabled={state === 'busy'}>{state === 'busy' ? 'Sending…' : cta}</button>
      {error && <span className="error small">{error}</span>}
    </form>
  );
}
