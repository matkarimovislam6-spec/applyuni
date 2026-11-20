const LABELS = {
  collecting_documents: 'Collecting documents',
  ready_for_review: 'Ready for your approval',
  approved: 'Approved — queued',
  submitting: 'Submitting…',
  submitted: 'Submitted',
  needs_attention: 'Needs attention',
  withdrawn: 'Withdrawn',
  missing: 'Missing',
  uploaded: 'Waiting for review',
  analyzing: 'AI checking…',
  accepted: 'Accepted',
  rejected: 'Needs a new file',
};

const TONE = {
  ready_for_review: 'info', approved: 'info', submitting: 'info', analyzing: 'info',
  submitted: 'ok', accepted: 'ok',
  needs_attention: 'bad', rejected: 'bad',
  missing: 'muted', withdrawn: 'muted',
};

export function StatusBadge({ status }) {
  return <span className={`badge ${TONE[status] || 'warn'}`}>{LABELS[status] || status}</span>;
}
