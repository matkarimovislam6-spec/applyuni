import { createContext, useContext, useEffect, useState } from 'react';
import { Link, Navigate, Route, Routes, useNavigate } from 'react-router-dom';
import { api, getToken, setToken } from './api.js';
import Auth from './pages/Auth.jsx';
import Students from './pages/Students.jsx';
import Student from './pages/Student.jsx';
import Application from './pages/Application.jsx';

const MeContext = createContext(null);
export const useMe = () => useContext(MeContext);

export default function App() {
  const [me, setMe] = useState(null);
  const [loading, setLoading] = useState(!!getToken());

  const refresh = async () => {
    if (!getToken()) return setMe(null);
    try {
      setMe(await api.me());
    } catch {
      setToken(null);
      setMe(null);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    refresh();
  }, []);

  if (loading) return <div className="center muted">Loading…</div>;

  return (
    <MeContext.Provider value={{ me, refresh }}>
      {me && <Header />}
      <main className="container">
        <Routes>
          <Route path="/login" element={me ? <Navigate to="/" /> : <Auth mode="login" />} />
          <Route path="/register" element={me ? <Navigate to="/" /> : <Auth mode="register" />} />
          <Route path="/" element={<Protected><Home /></Protected>} />
          <Route path="/students" element={<Protected><Students /></Protected>} />
          <Route path="/students/:id" element={<Protected><Student /></Protected>} />
          <Route path="/applications/:id" element={<Protected><Application /></Protected>} />
          <Route path="*" element={<Navigate to="/" />} />
        </Routes>
      </main>
    </MeContext.Provider>
  );
}

function Protected({ children }) {
  const { me } = useMe();
  return me ? children : <Navigate to="/login" />;
}

function Home() {
  const { me } = useMe();
  if (me.student_id) return <Navigate to={`/students/${me.student_id}`} replace />;
  return <Navigate to="/students" replace />;
}

function Header() {
  const { me, refresh } = useMe();
  const nav = useNavigate();
  const b2b = me.org_kind !== 'personal';
  return (
    <header className="topbar">
      <div className="container topbar-inner">
        <Link to="/" className="brand">ApplyUni</Link>
        <nav>
          {b2b && <Link to="/students">Students</Link>}
          <span className="muted small">
            {me.full_name} · {b2b ? `${me.org_name} (${me.seats_used}/${me.seat_limit} seats)` : 'Student'}
          </span>
          <button
            className="btn ghost"
            onClick={() => {
              setToken(null);
              refresh();
              nav('/login');
            }}
          >
            Log out
          </button>
        </nav>
      </div>
    </header>
  );
}
