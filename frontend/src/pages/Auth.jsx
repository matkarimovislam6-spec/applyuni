import { useState } from 'react';
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth.jsx';

const USERNAME_RE = /^[a-z0-9][a-z0-9._-]{2,31}$/i;
const MIN_PASSWORD = 8;

function PasswordInput({ value, onChange, autoComplete, id, invalid }) {
  const [show, setShow] = useState(false);
  return (
    <div className="pw">
      <input id={id} type={show ? 'text' : 'password'} value={value} onChange={onChange} autoComplete={autoComplete}
        aria-invalid={invalid || undefined} required />
      <button type="button" className="pw-toggle" onClick={() => setShow(!show)} aria-label={show ? 'Hide password' : 'Show password'}>
        {show ? 'Hide' : 'Show'}
      </button>
    </div>
  );
}

export default function Auth({ mode }) {
  const isSignup = mode === 'signup';
  const { user, signup, login } = useAuth();
  const nav = useNavigate();
  const from = useLocation().state?.from || '/account';
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [touched, setTouched] = useState(false);

  if (user) return <Navigate to={from} replace />;

  // live checks, shown once the visitor has started typing
  const checks = isSignup
    ? [
        [USERNAME_RE.test(username), '3–32 characters: letters, numbers, . _ -'],
        [password.length >= MIN_PASSWORD, `Password has at least ${MIN_PASSWORD} characters`],
        [password !== '' && password.toLowerCase() !== username.toLowerCase(), 'Password is different from the username'],
        [confirm !== '' && confirm === password, 'Passwords match'],
      ]
    : [];
  const valid = isSignup ? checks.every(([ok]) => ok) : username.trim() && password;

  const submit = async (e) => {
    e.preventDefault();
    setTouched(true);
    if (!valid) return;
    setError('');
    setBusy(true);
    try {
      if (isSignup) await signup(username.trim(), password, confirm);
      else await login(username.trim(), password);
      nav(from, { replace: true });
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="auth-page">
      <form className="auth-card" onSubmit={submit} noValidate>
        <p className="mono-label">{isSignup ? 'New here' : 'Welcome back'}</p>
        <h1 className="display-sm">{isSignup ? <>Create your <em>account</em></> : <>Log <em>in</em></>}</h1>
        <p className="muted small">
          {isSignup ? 'Save your answers and documents, and pick up where you left off on any visit.' : 'Your saved answers are waiting for you.'}
        </p>

        <label htmlFor="username">Username
          <input id="username" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username"
            autoCapitalize="none" spellCheck="false" required autoFocus />
        </label>
        <label htmlFor="password">Password
          <PasswordInput id="password" value={password} onChange={(e) => setPassword(e.target.value)}
            autoComplete={isSignup ? 'new-password' : 'current-password'} />
        </label>
        {isSignup && (
          <label htmlFor="confirm">Confirm password
            <PasswordInput id="confirm" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password"
              invalid={confirm !== '' && confirm !== password} />
          </label>
        )}

        {isSignup && (touched || username || password) && (
          <ul className="checks">
            {checks.map(([ok, text]) => <li key={text} className={ok ? 'ok' : ''}>{ok ? '✓' : '·'} {text}</li>)}
          </ul>
        )}

        {error && <div className="error small" role="alert">{error}</div>}
        <button className="btn primary big" disabled={busy || (touched && !valid)}>
          {busy ? 'One moment…' : isSignup ? 'Create account' : 'Log in'}
        </button>
        <p className="small muted center-text">
          {isSignup
            ? <>Already have an account? <Link to="/login" state={{ from }}>Log in</Link></>
            : <>New to ApplyUni? <Link to="/signup" state={{ from }}>Create an account</Link></>}
        </p>
      </form>
    </main>
  );
}
