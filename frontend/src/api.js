async function request(method, path, body) {
  const res = await fetch(`/api${path}`, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `Request failed (${res.status})`);
  return data;
}

export const api = {
  programs: (country = 'IT', degree = '') => request('GET', `/catalog/programs?country=${country}&degree=${degree}`),
  checklist: (programId, profile) => request('POST', `/catalog/programs/${programId}/checklist`, profile),
  joinWaitlist: (email, kind = 'student', programId = null) => request('POST', '/waitlist', { email, kind, program_id: programId }),
};
