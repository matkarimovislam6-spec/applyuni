-- Italy catalogue seed (SQLite). Generated from migrations/postgres/002_seed_italy.sql;
-- keep both in sync. Deadlines, fees and verified_at stay NULL until verified
-- from the official source.

INSERT INTO countries (code, name, source_url, rules) VALUES ('IT', 'Italy', 'https://www.universitaly.it', '
{
  "documents": [
    {"type": "passport", "title": "Passport", "degrees": ["bachelor", "master"],
     "description": "Valid international passport, photo page scan. Must be valid for the whole visa period."},
    {"type": "photo", "title": "Passport photo", "degrees": ["bachelor", "master"],
     "description": "Recent passport-style photo on a white background."},
    {"type": "school_diploma", "title": "High school diploma (attestat)", "degrees": ["bachelor"],
     "description": "Final secondary school certificate, with apostille."},
    {"type": "school_transcript", "title": "High school transcript", "degrees": ["bachelor"],
     "description": "Grades for the final years of school, with apostille."},
    {"type": "university_year_certificate", "title": "Proof of completed university year(s)", "degrees": ["bachelor"],
     "condition": "school_years_lt_12",
     "description": "Italy requires 12 years of schooling. With 11 years (common in Uzbekistan) you need at least one completed year of university or a recognised foundation year. Transcript of exams passed, with apostille."},
    {"type": "bachelor_diploma", "title": "Bachelor''s diploma", "degrees": ["master"],
     "description": "Bachelor''s degree certificate (or expected graduation letter if still studying), with apostille."},
    {"type": "bachelor_transcript", "title": "Bachelor''s transcript", "degrees": ["master"],
     "description": "Full list of exams, grades and credits, with apostille."},
    {"type": "comparability_statement", "title": "CIMEA Statement of Comparability or Declaration of Value", "degrees": ["bachelor", "master"],
     "description": "Recognition of your qualification in Italy. CIMEA statement (online) is accepted by most universities; otherwise a Dichiarazione di Valore from the Italian embassy in Tashkent."},
    {"type": "english_certificate", "title": "English certificate (IELTS/TOEFL)", "degrees": ["bachelor", "master"],
     "condition": "english_taught",
     "description": "Required for English-taught programmes. Check the programme''s minimum score."},
    {"type": "cv", "title": "CV (Europass format)", "degrees": ["master"],
     "description": "Education, experience, skills. The platform can generate this from your profile."},
    {"type": "motivation_letter", "title": "Motivation letter", "degrees": ["bachelor", "master"],
     "description": "Written by you. The AI coach gives feedback but does not write it."},
    {"type": "official_translations", "title": "Official translations", "degrees": ["bachelor", "master"],
     "condition": "documents_not_in_english",
     "description": "Certified translations into English or Italian for any document issued only in Uzbek or Russian."},
    {"type": "isee_family_documents", "title": "Family income & assets documents (for DSU scholarship)", "degrees": ["bachelor", "master"],
     "condition": "wants_scholarship",
     "description": "Family composition, income and property certificates for the previous year, with apostille and translation, used to compute ISEE parificato for regional DSU scholarships."}
  ],
  "steps": [
    {"key": "university_application", "title": "Apply on the university portal and receive admission letter"},
    {"key": "universitaly_preenrolment", "title": "Universitaly pre-enrolment (pre-iscrizione) for the study visa"},
    {"key": "visa", "title": "Type D study visa appointment at the Italian visa centre in Tashkent"},
    {"key": "dsu_scholarship", "title": "Regional DSU scholarship application (if eligible)"}
  ]
}
');

WITH u(country_code, name, city, portal, website) AS (VALUES
  ('IT', 'Politecnico di Milano',             'Milan',   'polimi',     'https://www.polimi.it'),
  ('IT', 'University of Bologna',             'Bologna', 'unibo',      'https://www.unibo.it'),
  ('IT', 'Sapienza University of Rome',       'Rome',    'sapienza',   'https://www.uniroma1.it'),
  ('IT', 'University of Padova',              'Padova',  'unipd',      'https://www.unipd.it'),
  ('IT', 'Politecnico di Torino',             'Turin',   'polito',     'https://www.polito.it'),
  ('IT', 'University of Milan',               'Milan',   'unimi',      'https://www.unimi.it')
)
INSERT INTO universities (id, country_code, name, city, portal, website)
SELECT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))), 2) || '-' || substr('89ab', 1 + (abs(random()) % 4), 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6)))), country_code, name, city, portal, website FROM u;

WITH p(uni, name, degree, req) AS (VALUES
  ('Politecnico di Milano',       'Computer Science and Engineering',                         'master',   '{}'),
  ('Politecnico di Milano',       'Architectural Design',                                     'bachelor', '{"extra_documents":[{"type":"portfolio","title":"Portfolio","description":"Selection of your design work, if requested by the admission call."}]}'),
  ('University of Bologna',       'Artificial Intelligence',                                  'master',   '{}'),
  ('University of Bologna',       'Business and Economics',                                   'bachelor', '{}'),
  ('Sapienza University of Rome', 'Applied Computer Science and Artificial Intelligence',     'bachelor', '{}'),
  ('Sapienza University of Rome', 'Data Science',                                             'master',   '{}'),
  ('University of Padova',        'Computer Engineering',                                     'master',   '{}'),
  ('University of Padova',        'Psychological Science',                                    'bachelor', '{}'),
  ('Politecnico di Torino',       'Computer Engineering',                                     'bachelor', '{}'),
  ('University of Milan',         'International Politics, Law and Economics',                'bachelor', '{}')
)
INSERT INTO programs (id, university_id, name, degree, language, requirements)
SELECT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))), 2) || '-' || substr('89ab', 1 + (abs(random()) % 4), 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6)))), u.id, p.name, p.degree, 'en', p.req
FROM p JOIN universities u ON u.name = p.uni;
