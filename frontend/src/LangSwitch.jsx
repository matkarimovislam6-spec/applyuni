import { useEffect, useRef, useState } from 'react';
import { FLAG } from './Flags.jsx';
import { LANGS, useT } from './i18n.jsx';

// Header language switcher: current flag + code; menu with all three.
export default function LangSwitch() {
  const { lang, setLang, t } = useT();
  const [open, setOpen] = useState(false);
  const ref = useRef(null);
  const Current = FLAG[lang];

  useEffect(() => {
    if (!open) return;
    const close = (e) => { if (!ref.current?.contains(e.target)) setOpen(false); };
    const esc = (e) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', esc);
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', esc); };
  }, [open]);

  return (
    <div className="lang-switch" ref={ref}>
      <button type="button" className="lang-btn" onClick={() => setOpen(!open)} aria-haspopup="menu" aria-expanded={open}
        aria-label={t('Language')} title={t('Language')}>
        <Current width={22} height={12} />
        <span className="lang-code">{lang.toUpperCase()}</span>
      </button>
      {open && (
        <ul className="lang-menu" role="menu">
          {LANGS.map(({ code, label }) => {
            const Flag = FLAG[code];
            return (
              <li key={code} role="none">
                <button type="button" role="menuitemradio" aria-checked={code === lang} className={code === lang ? 'on' : ''}
                  onClick={() => { setLang(code); setOpen(false); }}>
                  <Flag width={24} height={13} />
                  <span>{label}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
