// All calls go to the same origin, so the browser sends the HttpOnly session
// cookie automatically. The page never sees or stores the token itself.
async function request(method, path, body) {
  const res = await fetch(`/api${path}`, {
    method,
    credentials: 'same-origin',
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return null;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || `Request failed (${res.status})`);
    err.status = res.status;
    throw err;
  }
  return data;
}

export const api = {
  // public website
  programs: (country = 'IT', degree = '') => request('GET', `/catalog/programs?country=${country}&degree=${degree}`),
  checklist: (programId, profile) => request('POST', `/catalog/programs/${programId}/checklist`, profile),
  joinWaitlist: (email, kind = 'student', programId = null) => request('POST', '/waitlist', { email, kind, program_id: programId }),

  // accounts
  signup: (username, password, confirmPassword) =>
    request('POST', '/auth/register', { username, password, confirm_password: confirmPassword, account_type: 'student' }),
  login: (username, password) => request('POST', '/auth/login', { username, password }),
  logout: () => request('POST', '/auth/logout'),
  me: async () => {
    try {
      return await request('GET', '/me');
    } catch (e) {
      if (e.status === 401 || e.status === 404) return null; // not logged in
      throw e;
    }
  },

  // the logged-in student's saved data
  student: (id) => request('GET', `/students/${id}`),
  saveProfile: (id, profile) => request('PUT', `/students/${id}/profile`, profile),
  matches: () => request('GET', '/me/matches'),
};
