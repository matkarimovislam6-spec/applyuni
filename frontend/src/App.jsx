import { Link, Navigate, Route, Routes } from 'react-router-dom';
import Landing from './pages/Landing.jsx';
import Programs from './pages/Programs.jsx';
import Program from './pages/Program.jsx';

export default function App() {
  return (
    <>
      <header className="topbar">
        <div className="container topbar-inner">
          <Link to="/" className="brand">ApplyUni</Link>
          <nav>
            <Link to="/programs" className="hide-sm">Universities</Link>
            <Link to="/programs" className="btn primary">Get my checklist</Link>
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
        <div className="container small muted">
          © {new Date().getFullYear()} ApplyUni · Always confirm deadlines on the university's official call before applying.
        </div>
      </footer>
    </>
  );
}
