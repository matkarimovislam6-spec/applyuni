// Deadline and fee display. All dates come from the university's official
// page (see source_url / verified_at); nothing here is estimated.
import { useT } from './i18n.jsx';

function useFormat() {
  const { t, locale } = useT();
  const fmt = (d, opts = { day: 'numeric', month: 'short', year: 'numeric' }) =>
    d ? new Date(d).toLocaleDateString(locale, { timeZone: 'UTC', ...opts }) : '';
  const daysText = (n) => (n <= 1 ? t('closes today') : t('{n} days left', { n }));
  return { t, fmt, daysText, locale };
}

export const money = (amount, currency = 'EUR', locale = 'en') =>
  new Intl.NumberFormat(locale, { style: 'currency', currency, maximumFractionDigits: 0 }).format(amount);

// Small badge for lists: lets students screen programmes at a glance.
export function DeadlineBadge({ status }) {
  const { t, fmt, daysText } = useFormat();
  if (!status) return null;
  const { state, next, days_left: days, last_year: last } = status;
  if (state === 'open') {
    return <span className={`dl-badge open ${days <= 14 ? 'urgent' : ''}`}>{t('Open')} · {daysText(days)}</span>;
  }
  if (state === 'due') return <span className={`dl-badge due ${days <= 14 ? 'urgent' : ''}`}>{t('Due in {n} days', { n: days })}</span>;
  if (state === 'upcoming') return <span className="dl-badge upcoming">{t('Opens {date}', { date: fmt(next.opens_on, { day: 'numeric', month: 'short' }) })}</span>;
  if (state === 'closed') return <span className="dl-badge closed">{t('Closed')}</span>;
  return <span className="dl-badge unknown" title={last ? t('Last year: {date}', { date: fmt(last.closes_on) }) : undefined}>{t('Not published yet')}</span>;
}

// The "Deadline" fact on a programme page.
export function DeadlineFact({ program }) {
  const { t, fmt, daysText } = useFormat();
  const st = program.deadline_status;
  const { next, days_left: days, last_year: last } = st || {};
  if (st?.state === 'open' || st?.state === 'due' || st?.state === 'upcoming') {
    return (
      <div className="dl-fact">
        <span className="mono-label">{t('Next deadline')}</span>
        <strong className="dl-date">{fmt(next.closes_on)}</strong>
        <span className={`small ${st.state !== 'upcoming' && days <= 14 ? 'urgent-text' : 'muted'}`}>
          {st.state === 'upcoming' ? t('opens {date}', { date: fmt(next.opens_on) }) : daysText(days)} · {next.round}
        </span>
      </div>
    );
  }
  return (
    <div className="dl-fact">
      <span className="mono-label">{t('Deadline')}</span>
      <strong className="dl-date muted-strong">{st?.state === 'closed' ? t('All rounds closed') : t('Not published yet')}</strong>
      <span className="small muted">
        {program.call_note || (last ? t('Last year closed {date}', { date: fmt(last.closes_on) }) : t('We add it the day the university publishes it'))}
      </span>
    </div>
  );
}

// The "Application fee" fact: included in the student's package.
export function FeeFact({ program }) {
  const { t, locale } = useFormat();
  // the fee of the round you'd apply in now (e.g. €50 early bird), else the standard fee
  const next = program.deadline_status?.next;
  const fee = next?.fee_eur ?? program.application_fee_eur;
  return (
    <div className="dl-fact">
      <span className="mono-label">{t('Application fee')}</span>
      <strong className="dl-date">{fee != null ? money(fee, program.currency, locale) : t('Not confirmed yet')}</strong>
      <span className="small included">{t('✓ Included in your package, we pay it for you')}</span>
      {program.fee_note && <span className="small muted">{program.fee_note}</span>}
    </div>
  );
}

// Every round, with its state and official source.
export function Rounds({ program }) {
  const { t, fmt, locale } = useFormat();
  const rounds = program.deadlines || [];
  const STATE = { open: t('Open'), due: t('Due'), upcoming: t('Upcoming'), closed: t('Closed'), reference: t('Last year') };
  return (
    <section className="card stack">
      <div className="row between wrap">
        <h2>{t('Application rounds')}</h2>
        <span className="mono-label">{t('Non-EU applicants living abroad')}</span>
      </div>
      {rounds.length === 0 ? (
        <p className="small muted">
          {t("The 2027/28 dates aren't published yet.")} {program.call_note && <>{program.call_note} </>}
          {t('We add them the day the university publishes them.')}{' '}
          {program.call_source_url && <a href={program.call_source_url} target="_blank" rel="noreferrer">{t('Official page ↗')}</a>}
        </p>
      ) : (
        <ul className="rounds">
          {rounds.map((r) => (
            <li key={r.academic_year + r.round} className={`round ${r.state}`}>
              <div>
                <strong>{r.round}</strong> <span className="muted small">{r.academic_year}</span>
                <div className="small">
                  {r.opens_on ? `${fmt(r.opens_on)} – ` : `${t('until')} `}{fmt(r.closes_on)}
                  {r.fee_eur != null && <> · {t('fee')} {money(r.fee_eur, program.currency, locale)}</>}
                </div>
                {r.note && <div className="small muted">{r.note}</div>}
              </div>
              <div className="round-side">
                <span className={`dl-badge ${r.state === 'reference' ? 'unknown' : r.state}`}>{STATE[r.state]}</span>
                <a className="small" href={r.source_url} target="_blank" rel="noreferrer">{t('Source ↗')}</a>
                <span className="tiny muted">{t('checked {date}', { date: fmt(r.verified_at) })}</span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
