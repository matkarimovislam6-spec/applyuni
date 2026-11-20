import { createContext, useContext, useEffect, useMemo, useState } from 'react';
import { DICT } from './locales.js';
import { setApiLang } from './api.js';

// Languages: English, Uzbek (Latin script), Russian.
export const LANGS = [
  { code: 'en', label: 'English', locale: 'en-GB' },
  { code: 'uz', label: "O'zbekcha", locale: 'uz-Latn-UZ' },
  { code: 'ru', label: 'Русский', locale: 'ru-RU' },
];
const KEY = 'applyuni_lang';

function initialLang() {
  try {
    const saved = localStorage.getItem(KEY);
    if (LANGS.some((l) => l.code === saved)) return saved;
  } catch { /* storage unavailable */ }
  const nav = (navigator.language || 'en').toLowerCase();
  if (nav.startsWith('uz')) return 'uz';
  if (nav.startsWith('ru')) return 'ru';
  return 'en';
}

// Russian: 1 документ, 2–4 документа, 5+ документов (11–14 always "many").
function pluralIndex(lang, n) {
  if (lang !== 'ru') return 0;
  const m10 = n % 10, m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return 0;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return 1;
  return 2;
}

const LangContext = createContext(null);

export function LangProvider({ children }) {
  const [lang, setLangState] = useState(initialLang);
  setApiLang(lang); // keep API requests in sync before any child fetches

  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);

  const value = useMemo(() => {
    const idx = { uz: 0, ru: 1 }[lang];
    // t('English text', { name: value }) → translated text with {name} filled in
    const t = (key, vars) => {
      let s = idx === undefined ? key : DICT[key]?.[idx] ?? key;
      // Plurals: "one|few|many" forms (Russian), chosen by vars.n
      if (s.includes('|') && vars?.n !== undefined) s = s.split('|')[pluralIndex(lang, Number(vars.n))] ?? s;
      if (vars) for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(v);
      return s;
    };
    const locale = LANGS.find((l) => l.code === lang).locale;
    const setLang = (code) => {
      setLangState(code);
      try { localStorage.setItem(KEY, code); } catch { /* private mode */ }
    };
    return { lang, setLang, t, locale };
  }, [lang]);

  return <LangContext.Provider value={value}>{children}</LangContext.Provider>;
}

export const useT = () => useContext(LangContext);
