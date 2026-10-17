INSERT INTO doctors (name, specialty) VALUES
    ('Dr. Amina Hassan',  'General Practice'),
    ('Dr. Brian Otieno',  'Pediatrics'),
    ('Dr. Carol Wanjiru', 'Dermatology'),
    ('Dr. David Kimani',  'Cardiology'),
    ('Dr. Esther Njeri',  'Gynecology');

-- Mon-Fri 09:00-17:00 for everyone...
INSERT INTO working_hours (doctor_id, weekday, start_time, end_time)
SELECT d.id, w.day, '09:00', '17:00'
FROM doctors d CROSS JOIN generate_series(1, 5) AS w(day);

-- ...plus Saturday mornings for Dr. Amina, to show hours differ per doctor.
INSERT INTO working_hours (doctor_id, weekday, start_time, end_time)
SELECT id, 6, '09:00', '13:00' FROM doctors WHERE name = 'Dr. Amina Hassan';

INSERT INTO patients (name, phone, email) VALUES
    ('Test Patient One', '+254700000001', 'one@example.com'),
    ('Test Patient Two', '+254700000002', 'two@example.com');