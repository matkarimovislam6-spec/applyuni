// Fails if any t('…') key (or table label passed to t) has no Uzbek/Russian
// translation in src/locales.js. Run: node scripts/check-i18n.mjs
import fs from 'fs';
import path from 'path';
import { DICT } from '../src/locales.js';

const files = [];
(function walk(d) {
  for (const f of fs.readdirSync(d)) {
    const p = path.join(d, f);
    if (fs.statSync(p).isDirectory()) walk(p);
    else if (/\.jsx?$/.test(f) && !/i18n\.jsx$|locales\.js$/.test(f)) files.push(p);
  }
})('src');

const keys = new Set();
for (const f of files) {
  const s = fs.readFileSync(f, 'utf8');
  for (const m of s.matchAll(/\bt\(\s*(["'])((?:\\.|(?!\1).)*)\1/g)) keys.add(m[2].replace(/\\(["'])/g, '$1'));
  // labels in tables that are passed to t() through a variable
  for (const m of s.matchAll(/\[\s*(?:'[^']*'|\d+|true|false|"[^"]*")\s*,\s*(["'])((?:(?!\1).)+)\1\s*(?:,\s*(["'])((?:(?!\3).)+)\3)?\s*\]/g)) {
    // only real text (has lowercase letters), not codes like 'C1' or 'DE'
    if (/^[A-Z]/.test(m[2]) && /[a-z]/.test(m[2])) keys.add(m[2]);
    if (m[4] && /^[A-Z]/.test(m[4]) && /[a-z]/.test(m[4])) keys.add(m[4]);
  }
}
for (const k of ['Strong match', 'Possible', 'Long shot', 'Not eligible yet', 'Closed this year', 'taught in English', 'taught in Italian', 'taught in German',
  'highest education', 'bachelor subject', 'GPA', 'school grades', 'interests', 'English test']) keys.add(k);

const missing = [...keys].filter((k) => !DICT[k] || DICT[k].length !== 2 || DICT[k].some((x) => !x));
const source = files.map((f) => fs.readFileSync(f, 'utf8')).join('\n');
const quoted = (k) => [`'${k.replace(/'/g, "\\'")}'`, `"${k}"`, `'${k}'`].some((q) => source.includes(q));
const unused = Object.keys(DICT).filter((k) => !keys.has(k) && !quoted(k));
console.log(`${keys.size} keys used, ${Object.keys(DICT).length} translated`);
if (unused.length) console.log(`unused (fine, but check spelling): ${unused.join(' | ')}`);
if (missing.length) {
  console.error(`MISSING ${missing.length}:\n  ` + missing.join('\n  '));
  process.exit(1);
}
console.log('all keys translated ✓');
