// Deadline and fee display. All dates come from the university's official
// page (see source_url / verified_at); nothing here is estimated.

const fmt = (d, opts = { day: 'numeric', month: 'short', year: 'numeric' }) =>
  d ? new Date(d).toLocaleDateString('en-GB', { timeZone: 'UTC', ...opts }) : '';

const daysText = (n) => (n <= 1 ? 'closes today' : `${n} days left`);

// Small badge for lists: lets students screen programmes at a glance.
export function DeadlineBadge({ status }) {
  if (!status) return null;
  const { state, next, days_left: days, last_year: last } = status;
  if (state === 'open') {
    return <span className={`dl-badge open ${days <= 14 ? 'urgent' : ''}`}>Open · {daysText(days)}</span>;
  }
  if (state === 'upcoming') return <span className="dl-badge upcoming">Opens {fmt(next.opens_on, { day: 'numeric', month: 'short' })}</span>;
  if (state === 'closed') return <span className="dl-badge closed">Closed</span>;
  return <span className="dl-badge unknown" title={last ? `Last year: ${fmt(last.closes_on)}` : undefined}>Not published yet</span>;
}

// The "Deadline" fact on a programme page.
export function DeadlineFact({ program }) {
  const st = program.deadline_status;
  const { next, days_left: days, last_year: last } = st || {};
  if (st?.state === 'open' || st?.state === 'upcoming') {
    return (
      <div className="dl-fact">
        <span className="mono-label">Next deadline</span>
        <strong className="dl-date">{fmt(next.closes_on)}</strong>
        <span className={`small ${st.state === 'open' && days <= 14 ? 'urgent-text' : 'muted'}`}>
          {st.state === 'open' ? daysText(days) : `opens ${fmt(next.opens_on)}`} · {next.round}
        </span>
      </div>
    );
  }
  return (
    <div className="dl-fact">
      <span className="mono-label">Deadline</span>
      <strong className="dl-date muted-strong">{st?.state === 'closed' ? 'All rounds closed' : 'Not published yet'}</strong>
      <span className="small muted">
        {program.call_note || (last ? `Last year closed ${fmt(last.closes_on)}` : 'We add it the day the university publishes it')}
      </span>
    </div>
  );
}

// The "Application fee" fact: included in the student's package.
export function FeeFact({ program }) {
  // the fee of the round you'd apply in now (e.g. €50 early bird), else the standard fee
  const next = program.deadline_status?.next;
  const fee = next?.fee_eur ?? program.application_fee_eur;
  return (
    <div className="dl-fact">
      <span className="mono-label">Application fee</span>
      <strong className="dl-date">{fee != null ? `€${fee}` : 'Announced with the call'}</strong>
      <span className="small included">✓ Included in your package, we pay it for you</span>
      {program.fee_note && <span className="small muted">{program.fee_note}</span>}
    </div>
  );
}

// Every round, with its state and official source.
export function Rounds({ program }) {
  const rounds = program.deadlines || [];
  return (
    <section className="card stack">
      <div className="row between wrap">
        <h2>Application rounds</h2>
        <span className="mono-label">Non-EU applicants living abroad</span>
      </div>
      {rounds.length === 0 ? (
        <p className="small muted">
          The 2027/28 dates aren't published yet{program.call_note ? ` (${program.call_note.replace(/\.$/, '')})` : ''}.
          We add them the day the university publishes them. {program.call_source_url && <a href={program.call_source_url} target="_blank" rel="noreferrer">Official page ↗</a>}
        </p>
      ) : (
        <ul className="rounds">
          {rounds.map((r) => (
            <li key={r.academic_year + r.round} className={`round ${r.state}`}>
              <div>
                <strong>{r.round}</strong> <span className="muted small">{r.academic_year}</span>
                <div className="small">
                  {r.opens_on ? `${fmt(r.opens_on)} – ` : 'until '}{fmt(r.closes_on)}
                  {r.fee_eur != null && <> · fee €{r.fee_eur}</>}
                </div>
                {r.note && <div className="small muted">{r.note}</div>}
              </div>
              <div className="round-side">
                <span className={`dl-badge ${r.state === 'reference' ? 'unknown' : r.state}`}>
                  {{ open: 'Open', upcoming: 'Upcoming', closed: 'Closed', reference: 'Last year' }[r.state]}
                </span>
                <a className="small" href={r.source_url} target="_blank" rel="noreferrer">Source ↗</a>
                <span className="tiny muted">checked {fmt(r.verified_at)}</span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
