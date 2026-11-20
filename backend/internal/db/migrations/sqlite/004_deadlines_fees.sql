-- Application deadlines per programme and round, for non-EU applicants living abroad.
-- Only dates read on the university's official page are stored, each with its
-- source and the date it was checked. kind = 'official' (published for the
-- coming intake) or 'previous_year' (last cycle, shown for reference only).
CREATE TABLE program_deadlines (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  program_id     TEXT NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
  academic_year  TEXT NOT NULL,
  round          TEXT NOT NULL,
  opens_on       DATE,
  closes_on      DATE NOT NULL,
  kind           TEXT NOT NULL CHECK (kind IN ('official', 'previous_year')),
  fee_eur        INTEGER,
  source_url     TEXT NOT NULL,
  verified_at    DATE NOT NULL,
  note           TEXT
);
CREATE INDEX program_deadlines_program ON program_deadlines (program_id);

ALTER TABLE programs ADD COLUMN fee_note TEXT;
ALTER TABLE programs ADD COLUMN fee_source_url TEXT;
ALTER TABLE programs ADD COLUMN call_note TEXT;
ALTER TABLE programs ADD COLUMN call_source_url TEXT;

INSERT INTO program_deadlines (program_id, academic_year, round, opens_on, closes_on, kind, fee_eur, source_url, verified_at, note)
SELECT p.id, '2027/28', '1st call (early bird)', '2026-10-01', '2026-12-01', 'official', 50, 'https://www.polimi.it/en/prospective-students/how-to-apply/admission-to-laurea-magistrale/foreign-qualification/deadlines', '2026-11-20', 'Closes 11.59pm CET. Outcomes by end of May 2027.'
FROM programs p JOIN universities u ON u.id = p.university_id WHERE u.name = 'Politecnico di Milano' AND p.name = 'Computer Science and Engineering';

INSERT INTO program_deadlines (program_id, academic_year, round, opens_on, closes_on, kind, fee_eur, source_url, verified_at, note)
SELECT p.id, '2027/28', '2nd call', '2027-01-12', '2027-02-25', 'official', 150, 'https://www.polimi.it/en/prospective-students/how-to-apply/admission-to-laurea-magistrale/foreign-qualification/deadlines', '2026-11-20', 'Closes 11.59pm CET. Outcomes by end of May 2027.'
FROM programs p JOIN universities u ON u.id = p.university_id WHERE u.name = 'Politecnico di Milano' AND p.name = 'Computer Science and Engineering';

INSERT INTO program_deadlines (program_id, academic_year, round, opens_on, closes_on, kind, fee_eur, source_url, verified_at, note)
SELECT p.id, '2027/28', '1st call', '2026-09-15', '2026-11-15', 'official', NULL, 'https://www.unipd.it/en/how-apply', '2026-11-20', 'First call for most programmes. Later rounds not announced yet.'
FROM programs p JOIN universities u ON u.id = p.university_id WHERE u.name = 'University of Padova' AND p.name = 'Computer Engineering';

INSERT INTO program_deadlines (program_id, academic_year, round, opens_on, closes_on, kind, fee_eur, source_url, verified_at, note)
SELECT p.id, '2027/28', '1st call', '2026-09-15', '2026-11-15', 'official', NULL, 'https://www.unipd.it/en/how-apply', '2026-11-20', 'First call for most programmes. Later rounds not announced yet.'
FROM programs p JOIN universities u ON u.id = p.university_id WHERE u.name = 'University of Padova' AND p.name = 'Psychological Science';

INSERT INTO program_deadlines (program_id, academic_year, round, opens_on, closes_on, kind, fee_eur, source_url, verified_at, note)
SELECT p.id, '2026/27', 'Pre-selection (non-EU, visa)', '2025-12-22', '2026-05-15', 'previous_year', 30, 'https://www.uniroma1.it/it/pagina/international-admissions-2026-2027', '2026-11-20', 'Last years dates. The 2027/28 pre-selection is not published yet.'
FROM programs p JOIN universities u ON u.id = p.university_id WHERE u.name = 'Sapienza University of Rome' AND p.name = 'Applied Computer Science and Artificial Intelligence';

INSERT INTO program_deadlines (program_id, academic_year, round, opens_on, closes_on, kind, fee_eur, source_url, verified_at, note)
SELECT p.id, '2026/27', 'Pre-selection (non-EU, visa)', '2025-12-22', '2026-05-15', 'previous_year', 30, 'https://www.uniroma1.it/it/pagina/international-admissions-2026-2027', '2026-11-20', 'Last years dates. The 2027/28 pre-selection is not published yet.'
FROM programs p JOIN universities u ON u.id = p.university_id WHERE u.name = 'Sapienza University of Rome' AND p.name = 'Data Science';

UPDATE programs SET call_note = NULL, call_source_url = NULL, application_fee_eur = 150, fee_note = '€50 if you apply by 1 December 2026 (early bird), €150 after.', fee_source_url = 'https://www.polimi.it/en/prospective-students/how-to-apply/admission-to-laurea-magistrale/foreign-qualification/deadlines', verified_at = '2026-11-20', source_url = 'https://www.polimi.it/en/prospective-students/how-to-apply/admission-to-laurea-magistrale/foreign-qualification/deadlines'
WHERE name = 'Computer Science and Engineering' AND university_id = (SELECT id FROM universities WHERE name = 'Politecnico di Milano');

UPDATE programs SET call_note = '2027/28 pre-selection not published yet.', call_source_url = 'https://www.uniroma1.it/it/pagina/international-admissions-2026-2027', application_fee_eur = 30, fee_note = 'Per application (2026/27 amount; 2027/28 not published yet).', fee_source_url = 'https://www.uniroma1.it/it/pagina/international-admissions-2026-2027'
WHERE name = 'Applied Computer Science and Artificial Intelligence' AND university_id = (SELECT id FROM universities WHERE name = 'Sapienza University of Rome');

UPDATE programs SET call_note = '2027/28 pre-selection not published yet.', call_source_url = 'https://www.uniroma1.it/it/pagina/international-admissions-2026-2027', application_fee_eur = 30, fee_note = 'Per application (2026/27 amount; 2027/28 not published yet).', fee_source_url = 'https://www.uniroma1.it/it/pagina/international-admissions-2026-2027'
WHERE name = 'Data Science' AND university_id = (SELECT id FROM universities WHERE name = 'Sapienza University of Rome');

UPDATE programs SET call_note = 'Next round not announced yet.', call_source_url = 'https://www.unipd.it/en/how-apply'
WHERE name = 'Computer Engineering' AND university_id = (SELECT id FROM universities WHERE name = 'University of Padova');

UPDATE programs SET call_note = 'Next round not announced yet.', call_source_url = 'https://www.unipd.it/en/how-apply'
WHERE name = 'Psychological Science' AND university_id = (SELECT id FROM universities WHERE name = 'University of Padova');

UPDATE programs SET call_note = '2027/28 call "being finalized" (official site, 20 Nov 2026).', call_source_url = 'https://www.polito.it/en/education/applying-studying-graduating/admissions-and-enrolment/bachelor-s-degree-programmes/calls-for-application-regulations-and-ranking-lists'
WHERE name = 'Computer Engineering' AND university_id = (SELECT id FROM universities WHERE name = 'Politecnico di Torino');

UPDATE programs SET call_note = '2027/28 call not published yet; the 2026/27 call is closed for non-EU applicants.', call_source_url = 'https://corsi.unibo.it/2cycle/artificial-intelligence/how-to-enrol'
WHERE name = 'Artificial Intelligence' AND university_id = (SELECT id FROM universities WHERE name = 'University of Bologna');
