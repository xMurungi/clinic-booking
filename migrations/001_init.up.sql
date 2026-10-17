CREATE TABLE doctors (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    specialty  TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One working window per doctor per weekday (0 = Sunday, matching Go's time.Weekday).
-- Times are wall-clock in the clinic's timezone (Africa/Nairobi), not UTC.
CREATE TABLE working_hours (
    id         BIGSERIAL PRIMARY KEY,
    doctor_id  BIGINT NOT NULL REFERENCES doctors(id) ON DELETE CASCADE,
    weekday    SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time TIME NOT NULL,
    end_time   TIME NOT NULL,
    CHECK (end_time > start_time),
    UNIQUE (doctor_id, weekday)
);

CREATE TABLE patients (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    phone      TEXT NOT NULL,
    email      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE appointments (
    id            BIGSERIAL PRIMARY KEY,
    doctor_id     BIGINT NOT NULL REFERENCES doctors(id),
    patient_id    BIGINT NOT NULL REFERENCES patients(id),
    start_time    TIMESTAMPTZ NOT NULL,  -- slot start, always UTC in storage
    status        TEXT NOT NULL DEFAULT 'booked'
                  CHECK (status IN ('booked', 'cancelled')),
    cancel_reason TEXT,
    cancelled_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- slots start on :00 or :30 with no seconds
    CHECK (date_trunc('minute', start_time) = start_time
           AND EXTRACT(MINUTE FROM start_time AT TIME ZONE 'UTC') IN (0, 30)),
    -- a cancelled row must carry a reason and timestamp
    CHECK (status <> 'cancelled' OR (cancel_reason IS NOT NULL AND cancelled_at IS NOT NULL))
);

-- The key invariant: at most one ACTIVE booking per doctor per slot.
-- Cancelled rows don't block rebooking.
CREATE UNIQUE INDEX one_active_booking_per_slot
    ON appointments (doctor_id, start_time)
    WHERE status = 'booked';

-- Availability: fetch a doctor's booked slots for a day range.
-- (Covered by the unique index above.)

-- Bonus endpoint: a patient's upcoming appointments.
CREATE INDEX appointments_patient_upcoming
    ON appointments (patient_id, start_time)
    WHERE status = 'booked';