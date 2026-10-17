DELETE FROM patients WHERE email IN ('one@example.com', 'two@example.com');
DELETE FROM doctors;  -- working_hours cascades