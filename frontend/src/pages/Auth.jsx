import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { api, setToken } from '../api.js';
import { useMe } from '../App.jsx';

export default function Auth({ mode }) {
  const isRegister = mode === 'register';
  const { refresh } = useMe();
  const nav = useNavigate();
  const [form, setForm] = useState({ email: '', password: '', full_name: '', account_type: 'student', org_name: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (k) => (e) => setForm({ ...form, [k]: e.target.value });

  const submit = async (e) => {
    e.preventDefault();
    setError('');
    setBusy(true);
    try {
      const s = isRegister ? await api.register(form) : await api.login({ email: form.email, password: form.password });
      setToken(s.token);
      await refresh();
      nav('/');
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="auth">
      <h1>ApplyUni</h1>
      <p className="muted">Apply to universities abroad without paying a consultant. Upload your documents, review, approve — we do the rest.</p>
      <form className="card stack" onSubmit={submit}>
        <h2>{isRegister ? 'Create account' : 'Log in'}</h2>
        {isRegister && (
          <>
            <div className="segmented">
              {[
                ['student', 'I am a student'],
                ['agency', 'I am an agency / school'],
              ].map(([v, label]) => (
                <button type="button" key={v} className={form.account_type === v ? 'on' : ''} onClick={() => setForm({ ...form, account_type: v })}>
                  {label}
                </button>
              ))}
            </div>
            <label>Full name<input value={form.full_name} onChange={set('full_name')} required /></label>
            {form.account_type === 'agency' && (
              <label>Organization name<input value={form.org_name} onChange={set('org_name')} required /></label>
            )}
          </>
        )}
        <label>Email<input type="email" value={form.email} onChange={set('email')} required autoComplete="email" /></label>
        <label>
          Password
          <input type="password" value={form.password} onChange={set('password')} required minLength={isRegister ? 10 : undefined}
            autoComplete={isRegister ? 'new-password' : 'current-password'} />
        </label>
        {error && <div className="error">{error}</div>}
        <button className="btn primary" disabled={busy}>{busy ? 'Please wait…' : isRegister ? 'Create account' : 'Log in'}</button>
        <p className="small muted">
          {isRegister ? <>Already have an account? <Link to="/login">Log in</Link></> : <>New here? <Link to="/register">Create an account</Link></>}
        </p>
      </form>
    </div>
  );
}
