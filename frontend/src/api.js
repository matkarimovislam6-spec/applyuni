const TOKEN_KEY = 'applyuni_token';

export const getToken = () => localStorage.getItem(TOKEN_KEY);
export const setToken = (t) => (t ? localStorage.setItem(TOKEN_KEY, t) : localStorage.removeItem(TOKEN_KEY));

async function request(method, path, body, isForm = false) {
  const headers = {};
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body && !isForm) headers['Content-Type'] = 'application/json';
  const res = await fetch(`/api${path}`, {
    method,
    headers,
    body: body ? (isForm ? body : JSON.stringify(body)) : undefined,
  });
  if (res.status === 401 && token) {
    setToken(null);
    window.location.href = '/login';
    return;
  }
  if (res.status === 204) return null;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `Request failed (${res.status})`);
  return data;
}

export const api = {
  register: (b) => request('POST', '/auth/register', b),
  login: (b) => request('POST', '/auth/login', b),
  me: () => request('GET', '/me'),
  programs: (country = 'IT', degree = '') => request('GET', `/catalog/programs?country=${country}&degree=${degree}`),
  students: () => request('GET', '/students'),
  createStudent: (b) => request('POST', '/students', b),
  student: (id) => request('GET', `/students/${id}`),
  saveProfile: (id, profile) => request('PUT', `/students/${id}/profile`, profile),
  deleteStudent: (id) => request('DELETE', `/students/${id}`),
  applications: (id) => request('GET', `/students/${id}/applications`),
  createApplication: (id, programId) => request('POST', `/students/${id}/applications`, { program_id: programId }),
  application: (id) => request('GET', `/applications/${id}`),
  approve: (id) => request('POST', `/applications/${id}/approve`, { confirm: true }),
  documents: (id) => request('GET', `/students/${id}/documents`),
  upload: (studentId, docType, file) => {
    const fd = new FormData();
    fd.append('doc_type', docType);
    fd.append('file', file);
    return request('POST', `/students/${studentId}/documents`, fd, true);
  },
  review: (docId, accept, note) => request('POST', `/documents/${docId}/review`, { accept, note }),
  deleteDocument: (docId) => request('DELETE', `/documents/${docId}`),
  download: async (docId) => {
    const res = await fetch(`/api/documents/${docId}/download`, { headers: { Authorization: `Bearer ${getToken()}` } });
    if (!res.ok) throw new Error('Download failed');
    const blob = await res.blob();
    const name = /filename="?([^"]+)"?/.exec(res.headers.get('Content-Disposition') || '')?.[1] || 'document';
    const url = URL.createObjectURL(blob);
    const a = Object.assign(document.createElement('a'), { href: url, download: name });
    a.click();
    URL.revokeObjectURL(url);
  },
};
