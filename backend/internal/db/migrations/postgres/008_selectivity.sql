-- Programme selectivity for weighted grade scoring (internal/matching/scale_algorithm.go):
-- 1 = very selective, 2 = selective (default for every other programme), 3 = moderately selective.
-- These are editorial starting values, not official rankings: tune them against real outcomes.

UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 2);

UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 1)
WHERE name = 'Computer Science and Engineering' AND university_id = (SELECT id FROM universities WHERE name = 'Politecnico di Milano');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 1)
WHERE name = 'Management Engineering' AND university_id = (SELECT id FROM universities WHERE name = 'Politecnico di Milano');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 1)
WHERE name = 'Informatics' AND university_id = (SELECT id FROM universities WHERE name = 'Technical University of Munich');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 1)
WHERE name = 'Computer Science' AND university_id = (SELECT id FROM universities WHERE name = 'Georgia Institute of Technology');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 1)
WHERE name = 'Computer Science' AND university_id = (SELECT id FROM universities WHERE name = 'University of Illinois Urbana-Champaign');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 1)
WHERE name = 'Computer Science' AND university_id = (SELECT id FROM universities WHERE name = 'University of Michigan');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 1)
WHERE name = 'Computer Science' AND university_id = (SELECT id FROM universities WHERE name = 'The University of Texas at Austin');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 1)
WHERE name = 'International Economics and Management' AND university_id = (SELECT id FROM universities WHERE name = 'Bocconi University');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 1)
WHERE name = 'Computer Science' AND university_id = (SELECT id FROM universities WHERE name = 'Purdue University');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 3)
WHERE name = 'Computer Science' AND university_id = (SELECT id FROM universities WHERE name = 'Arizona State University');
UPDATE programs SET requirements = requirements || jsonb_build_object('selectivity', 3)
WHERE name = 'Digital Intelligence and Change Management' AND university_id = (SELECT id FROM universities WHERE name = 'University of Pisa');
