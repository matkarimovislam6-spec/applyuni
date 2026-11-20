import { useEffect } from 'react';
import { Link, Navigate, NavLink, Route, Routes, useLocation } from 'react-router-dom';
import Landing from './pages/Landing.jsx';
import Programs from './pages/Programs.jsx';
import Program from './pages/Program.jsx';

export function Logo() {
  return (
    <Link to="/" className="brand" aria-label="ApplyUni home">
      <svg viewBox="0 0 32 32" width="26" height="26" aria-hidden="true">
        <rect width="32" height="32" rx="8" fill="var(--clay)" />
        <path d="M16 7 5 12.5 16 18l11-5.5zM9.5 15.5v4.8c0 1.9 3 3.7 6.5 3.7s6.5-1.8 6.5-3.7v-4.8L16 18.8z" fill="var(--paper)" />
      </svg>
      <span>ApplyUni</span>
    </Link>
  );
}

export default function App() {
  const { pathname } = useLocation();
  useEffect(() => {
    window.scrollTo(0, 0); // braces: newer browsers return a Promise here, which React must not treat as a cleanup
  }, [pathname]);

  return (
    <>
      <header className="topbar">
        <div className="container topbar-inner">
          <Logo />
          <nav>
            <NavLink to="/programs" className="navlink hide-sm">Universities</NavLink>
            <a href="/#how" className="navlink hide-sm">How it works</a>
            <a href="/#agencies" className="navlink hide-sm">For agencies</a>
            <Link to="/programs" className="btn primary sm">Get my checklist</Link>
          </nav>
        </div>
      </header>
      <Routes>
        <Route path="/" element={<Landing />} />
        <Route path="/programs" element={<Programs />} />
        <Route path="/programs/:id" element={<Program />} />
        <Route path="*" element={<Navigate to="/" />} />
      </Routes>
      <footer className="footer">
        <div className="container footer-inner">
          <div>
            <Logo />
            <p className="muted small">Made in Tashkent for students who'd rather spend 10 million soums on their first year abroad.</p>
          </div>
          <p className="mono-label">Always confirm deadlines on the university's official call · © {new Date().getFullYear()}</p>
        </div>
      </footer>
    </>
  );
}
