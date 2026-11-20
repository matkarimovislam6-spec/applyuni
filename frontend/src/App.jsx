import { useEffect } from 'react';
import { Link, Navigate, NavLink, Route, Routes, useLocation } from 'react-router-dom';
import Landing from './pages/Landing.jsx';
import Programs from './pages/Programs.jsx';
import Program from './pages/Program.jsx';
import ThemeToggle from './ThemeToggle.jsx';
import MadeIn from './MadeIn.jsx';
import Auth from './pages/Auth.jsx';
import Account from './pages/Account.jsx';
import { AuthProvider, useAuth } from './auth.jsx';
import { LangProvider, useT } from './i18n.jsx';
import LangSwitch from './LangSwitch.jsx';

export function Logo() {
  const { t } = useT();
  return (
    <Link to="/" className="brand" aria-label={t('Apply home')}>
      <svg viewBox="0 0 32 32" width="26" height="26" aria-hidden="true">
        <rect width="32" height="32" rx="8" fill="var(--clay)" />
        <path d="M16 7 5 12.5 16 18l11-5.5zM9.5 15.5v4.8c0 1.9 3 3.7 6.5 3.7s6.5-1.8 6.5-3.7v-4.8L16 18.8z" fill="var(--paper)" />
      </svg>
      <span>Apply</span>
    </Link>
  );
}

function AccountNav() {
  const { user } = useAuth();
  const { t } = useT();
  if (user === undefined) return <span className="nav-placeholder" />; // checking the session
  if (user) {
    return (
      <Link to="/account" className="avatar-link" title={t('My account')}>
        <span className="avatar" aria-hidden="true">{user.username[0].toUpperCase()}</span>
        <span className="hide-sm">{user.username}</span>
      </Link>
    );
  }
  return (
    <>
      <NavLink to="/login" className="navlink">{t('Log in')}</NavLink>
      <Link to="/signup" className="btn primary sm">{t('Sign up')}</Link>
    </>
  );
}

export default function App() {
  return (
    <LangProvider>
      <AuthProvider>
        <Shell />
      </AuthProvider>
    </LangProvider>
  );
}

function Shell() {
  const { pathname } = useLocation();
  const { t, lang } = useT();
  useEffect(() => {
    window.scrollTo(0, 0); // braces: newer browsers return a Promise here, which React must not treat as a cleanup
  }, [pathname]);

  return (
    <>
      <header className="topbar">
        <div className="topbar-inner">
          <Logo />
          <nav>
            <NavLink to="/programs" className="navlink hide-sm">{t('Universities')}</NavLink>
            <a href="/#how" className="navlink hide-sm">{t('How it works')}</a>
            <a href="/#agencies" className="navlink hide-sm">{t('For agencies')}</a>
            <LangSwitch />
            <ThemeToggle />
            <AccountNav />
          </nav>
        </div>
      </header>
      {/* remount pages on language change so server text is fetched again in the new language */}
      <Routes key={lang}>
        <Route path="/" element={<Landing />} />
        <Route path="/programs" element={<Programs />} />
        <Route path="/programs/:id" element={<Program />} />
        <Route path="/signup" element={<Auth mode="signup" />} />
        <Route path="/login" element={<Auth mode="login" />} />
        <Route path="/account" element={<Account />} />
        <Route path="*" element={<Navigate to="/" />} />
      </Routes>
      <footer className="footer">
        <div className="container footer-inner">
          <div>
            <Logo />
            <MadeIn />
          </div>
          <p className="mono-label">{t("Always confirm deadlines on the university's official call")} · © {new Date().getFullYear()}</p>
        </div>
      </footer>
    </>
  );
}
