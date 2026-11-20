import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api.js';
import { useMe } from '../App.jsx';

// B2B dashboard: an agency or school manages many students.
export default function Students() {
  const { me, refresh } = useMe();
  const [students, setStudents] = useState([]);
  const [form, setForm] = useState({ full_name: '', email: '' });
  const [error, setError] = useState('');

  const load = () => api.students().then(setStudents).catch((e) => setError(e.message));
  useEffect(() => {
    load();
  }, []);

  const add = async (e) => {
    e.preventDefault();
    setError('');
    try {
      await api.createStudent(form);
      setForm({ full_name: '', email: '' });
      load();
      refresh();
    } catch (err) {
      setError(err.message);
    }
  };

  return (
    <div className="stack">
      <div className="row between">
        <h1>Students</h1>
        <span className="pill">{me.seats_used} / {me.seat_limit} seats · {me.plan} plan</span>
      </div>
      <form className="card row wrap" onSubmit={add}>
        <input placeholder="Student full name" value={form.full_name} onChange={(e) => setForm({ ...form, full_name: e.target.value })} required />
        <input placeholder="Student email (optional)" type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
        <button className="btn primary">Add student</button>
      </form>
      {error && <div className="error">{error}</div>}
      <div className="card">
        {students.length === 0 ? (
          <p className="muted">No students yet.</p>
        ) : (
          <table>
            <thead><tr><th>Name</th><th>Email</th><th>Agent inbox</th><th>Added</th></tr></thead>
            <tbody>
              {students.map((s) => (
                <tr key={s.id}>
                  <td><Link to={`/students/${s.id}`}>{s.full_name}</Link></td>
                  <td>{s.email || '—'}</td>
                  <td className="mono small">{s.email_alias}</td>
                  <td>{new Date(s.created_at).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
