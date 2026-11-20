import { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { api } from './api.js';

const AuthContext = createContext(null);
export const useAuth = () => useContext(AuthContext);

// Holds the logged-in user (or null) and their saved profile. The session
// itself lives in an HttpOnly cookie, so it survives reloads and restarts.
export function AuthProvider({ children }) {
  const [user, setUser] = useState(undefined); // undefined = still checking
  const [profile, setProfile] = useState(null);

  const load = useCallback(async () => {
    try {
      const me = await api.me();
      setUser(me);
      if (me?.student_id) {
        const st = await api.student(me.student_id);
        setProfile(st.profile || {});
      } else {
        setProfile(null);
      }
      return me;
    } catch {
      setUser(null);
      setProfile(null);
      return null;
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const signup = async (username, password, confirm) => {
    await api.signup(username, password, confirm);
    return load();
  };
  const login = async (username, password) => {
    await api.login(username, password);
    return load();
  };
  const logout = async () => {
    await api.logout().catch(() => {});
    setUser(null);
    setProfile(null);
  };
  const saveProfile = async (next) => {
    if (!user?.student_id) return;
    const st = await api.saveProfile(user.student_id, next);
    setProfile(st.profile || next);
  };

  return (
    <AuthContext.Provider value={{ user, profile, signup, login, logout, saveProfile, reload: load }}>
      {children}
    </AuthContext.Provider>
  );
}
