-- Fix a lost apostrophe in a deadline note written by 004.
UPDATE program_deadlines SET note = 'Last year''s dates. The 2027/28 pre-selection is not published yet.'
WHERE note = 'Last years dates. The 2027/28 pre-selection is not published yet.';
