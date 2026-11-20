import { useState } from 'react';
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth.jsx';
import { useT } from '../i18n.jsx';

const USERNAME_RE = /^[a-z0-9][a-z0-9._-]{2,31}$/i;
const MIN_PASSWORD = 8;

function PasswordInput({ value, onChange, autoComplete, id, invalid }) {
  const { t } = useT();
  const [show, setShow] = useState(false);
  return (
    <div className="pw">
      <input id={id} type={show ? 'text' : 'password'} value={value} onChange={onChange} autoComplete={autoComplete}
        aria-invalid={invalid || undefined} required />
      <button type="button" className="pw-toggle" onClick={() => setShow(!show)} aria-label={show ? t('Hide password') : t('Show password')}>
        {show ? t('Hide') : t('Show')}
      </button>
    </div>
  );
}

export default function Auth({ mode }) {
  const { t } = useT();
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
        [USERNAME_RE.test(username), t('3–32 characters: letters, numbers, . _ -')],
        [password.length >= MIN_PASSWORD, t('Password has at least {n} characters', { n: MIN_PASSWORD })],
        [password !== '' && password.toLowerCase() !== username.toLowerCase(), t('Password is different from the username')],
        [confirm !== '' && confirm === password, t('Passwords match')],
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
        <p className="mono-label">{isSignup ? t('New here') : t('Welcome back')}</p>
        <h1 className="display-sm">{isSignup ? <>{t('Create your')} <em>{t('account')}</em></> : <>{t('Log')} <em>{t('in')}</em></>}</h1>
        <p className="muted small">
          {isSignup ? t('Save your answers and documents, and pick up where you left off on any visit.') : t('Your saved answers are waiting for you.')}
        </p>

        <label htmlFor="username">{t('Username')}
          <input id="username" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username"
            autoCapitalize="none" spellCheck="false" required autoFocus />
        </label>
        <label htmlFor="password">{t('Password')}
          <PasswordInput id="password" value={password} onChange={(e) => setPassword(e.target.value)}
            autoComplete={isSignup ? 'new-password' : 'current-password'} />
        </label>
        {isSignup && (
          <label htmlFor="confirm">{t('Confirm password')}
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
          {busy ? t('One moment…') : isSignup ? t('Create account') : t('Log in')}
        </button>
        <p className="small muted center-text">
          {isSignup
            ? <>{t('Already have an account?')} <Link to="/login" state={{ from }}>{t('Log in')}</Link></>
            : <>{t('New to Apply?')} <Link to="/signup" state={{ from }}>{t('Create an account')}</Link></>}
        </p>
      </form>
    </main>
  );
}
