package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xMurungi/clinic-booking/internal/booking"
	"github.com/xMurungi/clinic-booking/internal/domain"
)

type Store struct {
	pool *pgxpool.Pool
}

// catch a mismatch at compile time,
var _ booking.Store = (*Store)(nil)

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) DoctorExists(ctx context.Context, id int64) (bool, error) {
	var ok bool

	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM doctors WHERE id = $1)`,
		id,
	).Scan(&ok)

	return ok, err
}

// WorkingWindow returns the doctor's hours for a weekday as minutes since midnight.
// found=false means the doctor doesn't work that day (not an error).
func (s *Store) WorkingWindow(ctx context.Context, doctorID int64, wd time.Weekday) (w booking.Window, found bool, err error) {

	err = s.pool.QueryRow(ctx, `
	SELECT (EXTRACT(EPOCH FROM start_time) / 60)::int,
			(EXTRACT(EPOCH FROM end_time)   / 60)::int
	FROM working_hours
	WHERE doctor_id = $1 AND weekday = $2`,
		doctorID, int(wd)).Scan(&w.StartMin, &w.EndMin)

	if errors.Is(err, pgx.ErrNoRows) {
		return booking.Window{}, false, nil
	}

	if err != nil {
		return booking.Window{}, false, err
	}

	return w, true, nil
}

// BookedStarts returns start times of active bookings in [from, to).
func (s *Store) BookedStarts(ctx context.Context, doctorID int64, from, to time.Time) ([]time.Time, error) {

	rows, err := s.pool.Query(ctx, `
	SELECT start_time FROM appointments
	WHERE doctor_id = $1 AND status = 'booked'
	AND start_time >= $2 AND start_time < $3`,
		doctorID, from, to)

	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var out []time.Time

	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}

	return out, rows.Err()
}

func (s *Store) PatientExists(ctx context.Context, id int64) (bool, error) {

	var ok bool

	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM patients WHERE id = $1)`,
		id,
	).Scan(&ok)

	return ok, err
}

func (s *Store) CreateAppointment(ctx context.Context, doctorID int64, patientID int64, start time.Time) (domain.Appointment, error) {

	var a domain.Appointment
	err := s.pool.QueryRow(ctx, `
		INSERT INTO appointments (doctor_id, patient_id, start_time)
		VALUES ($1, $2, $3)
		RETURNING id, doctor_id, patient_id, start_time, status`,
		doctorID, patientID, start.UTC(),
	).Scan(&a.ID, &a.DoctorID, &a.PatientID, &a.StartTime, &a.Status)

	var pgErr *pgconn.PgError

	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "one_active_booking_per_slot" {
		return domain.Appointment{}, domain.ErrSlotTaken
	}

	return a, err
}

const apptCols = `id, doctor_id, patient_id, start_time, status`

func scanAppt(row pgx.Row) (domain.Appointment, error) {

	var a domain.Appointment
	err := row.Scan(&a.ID, &a.DoctorID, &a.PatientID, &a.StartTime, &a.Status)
	return a, err
}

func isSlotTaken(err error) bool {

	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "one_active_booking_per_slot"
}

func (s *Store) GetAppointment(ctx context.Context, id int64) (domain.Appointment, error) {
	a, err := scanAppt(s.pool.QueryRow(ctx, `
		SELECT `+apptCols+` FROM appointments WHERE id = $1`, id))

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Appointment{}, domain.ErrAppointmentNotFound
	}

	return a, err
}

func (s *Store) CancelAppointment(ctx context.Context, id int64, reason string) (domain.Appointment, error) {

	a, err := scanAppt(s.pool.QueryRow(ctx, `
		UPDATE appointments
		SET status = 'cancelled', cancel_reason = $2, cancelled_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'booked'
		RETURNING `+apptCols, id, reason))

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Appointment{}, s.missingOrCancelled(ctx, id)
	}

	return a, err
}

func (s *Store) RescheduleAppointment(ctx context.Context, id int64, start time.Time) (domain.Appointment, error) {
	a, err := scanAppt(s.pool.QueryRow(ctx, `
		UPDATE appointments
		SET start_time = $2, updated_at = now()
		WHERE id = $1 AND status = 'booked'
		RETURNING `+apptCols, id, start.UTC()))

	if isSlotTaken(err) {
		return domain.Appointment{}, domain.ErrSlotTaken
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Appointment{}, s.missingOrCancelled(ctx, id)
	}

	return a, err
}

// The UPDATE matched nothing: decide whether the row is absent or just cancelled.
func (s *Store) missingOrCancelled(ctx context.Context, id int64) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM appointments WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}

	if !exists {
		return domain.ErrAppointmentNotFound
	}
	return domain.ErrAlreadyCancelled
}

func (s *Store) UpcomingForPatient(ctx context.Context, patientID int64, from time.Time) ([]domain.Appointment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+apptCols+` FROM appointments
		WHERE patient_id = $1 AND status = 'booked' AND start_time >= $2
		ORDER BY start_time ASC`, patientID, from.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Appointment{}
	for rows.Next() {
		a, err := scanAppt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
