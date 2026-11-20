-- Matching metadata per programme (see internal/matching): subject field,
-- which bachelor's subjects usually qualify for a master's (background_fields)
-- or may qualify after a credit check (adjacent_fields), entrance tests.
-- Programme minimums (english_min_ielts, min_gpa_pct) are added only once
-- verified on the official call.

UPDATE programs SET requirements = '{"field": "computer_science", "background_fields": ["computer_science", "electrical_engineering"], "adjacent_fields": ["mathematics", "physics", "statistics_data", "engineering_other"]}'::jsonb
WHERE name = 'Computer Science and Engineering' AND university_id = (SELECT id FROM universities WHERE name = 'Politecnico di Milano');

UPDATE programs SET requirements = '{"field": "architecture_design", "needs_portfolio": true, "extra_documents": [{"type": "portfolio", "title": "Portfolio", "description": "Selection of your design work, if requested by the admission call."}]}'::jsonb
WHERE name = 'Architectural Design' AND university_id = (SELECT id FROM universities WHERE name = 'Politecnico di Milano');

UPDATE programs SET requirements = '{"field": "computer_science", "background_fields": ["computer_science", "electrical_engineering", "mathematics", "statistics_data"], "adjacent_fields": ["physics", "engineering_other"]}'::jsonb
WHERE name = 'Artificial Intelligence' AND university_id = (SELECT id FROM universities WHERE name = 'University of Bologna');

UPDATE programs SET requirements = '{"field": "economics_business"}'::jsonb
WHERE name = 'Business and Economics' AND university_id = (SELECT id FROM universities WHERE name = 'University of Bologna');

UPDATE programs SET requirements = '{"field": "computer_science"}'::jsonb
WHERE name = 'Applied Computer Science and Artificial Intelligence' AND university_id = (SELECT id FROM universities WHERE name = 'Sapienza University of Rome');

UPDATE programs SET requirements = '{"field": "statistics_data", "background_fields": ["computer_science", "statistics_data", "mathematics"], "adjacent_fields": ["physics", "electrical_engineering", "engineering_other", "economics_business"]}'::jsonb
WHERE name = 'Data Science' AND university_id = (SELECT id FROM universities WHERE name = 'Sapienza University of Rome');

UPDATE programs SET requirements = '{"field": "computer_science", "background_fields": ["computer_science", "electrical_engineering"], "adjacent_fields": ["mathematics", "physics", "engineering_other"]}'::jsonb
WHERE name = 'Computer Engineering' AND university_id = (SELECT id FROM universities WHERE name = 'University of Padova');

UPDATE programs SET requirements = '{"field": "psychology"}'::jsonb
WHERE name = 'Psychological Science' AND university_id = (SELECT id FROM universities WHERE name = 'University of Padova');

UPDATE programs SET requirements = '{"field": "computer_science", "entry_test": "From 2027/28 Politecnico di Torino uses CISIA admission tests, taken in person.", "entry_test_source": "https://www.polito.it/en/education/applying-studying-graduating/admissions-and-enrolment/bachelor-s-degree-programmes/calls-for-application-regulations-and-ranking-lists"}'::jsonb
WHERE name = 'Computer Engineering' AND university_id = (SELECT id FROM universities WHERE name = 'Politecnico di Torino');

UPDATE programs SET requirements = '{"field": "political_science"}'::jsonb
WHERE name = 'International Politics, Law and Economics' AND university_id = (SELECT id FROM universities WHERE name = 'University of Milan');
