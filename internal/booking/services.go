package booking

import (
	"context"
	"strings"
	"time"

	"github.com/xMurungi/clinic-booking/internal/domain"
)

// Store is what the service needs; defined here (consumer side) so tests can fake it.
type Store interface {
	DoctorExists(ctx context.Context, id int64) (bool, error)
	PatientExists(ctx context.Context, id int64) (bool, error)
	WorkingWindow(ctx context.Context, doctorID int64, wd time.Weekday) (Window, bool, error)
	BookedStarts(ctx context.Context, doctorID int64, from, to time.Time) ([]time.Time, error)
	CreateAppointment(ctx context.Context, doctorID, patientID int64, start time.Time) (domain.Appointment, error)
	GetAppointment(ctx context.Context, id int64) (domain.Appointment, error)
	CancelAppointment(ctx context.Context, id int64, reason string) (domain.Appointment, error)
	RescheduleAppointment(ctx context.Context, id int64, start time.Time) (domain.Appointment, error)
	UpcomingForPatient(ctx context.Context, patientID int64, from time.Time) ([]domain.Appointment, error)
}

type Service struct {
	store   Store
	loc     *time.Location
	now     func() time.Time // Injected clock
	minLead time.Duration    // 0 for now; 1h for the bonus rule
}

func NewService(store Store, loc *time.Location, now func() time.Time, minLead time.Duration) *Service {
	return &Service{
		store:   store,
		loc:     loc,
		now:     now,
		minLead: minLead,
	}
}

// Availability expects date to be midnight in the clinic's timezone.
func (s *Service) Availability(ctx context.Context, doctorID int64, date time.Time) ([]Slot, error) {

	exists, err := s.store.DoctorExists(ctx, doctorID)

	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, domain.ErrDoctorNotFound
	}

	w, works, err := s.store.WorkingWindow(ctx, doctorID, date.Weekday())
	if err != nil {
		return nil, err
	}

	if !works {
		return []Slot{}, nil
	}

	booked, err := s.store.BookedStarts(ctx, doctorID, date, date.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}

	all := GenerateSlots(date, w, s.loc)

	return FilterAvailable(all, booked, s.now(), s.minLead), nil
}

// validateSlot holds the rules shared by booking and (later) rescheduling.
func (s *Service) ValidateSlot(ctx context.Context, doctorID int64, start time.Time) error {

	if !OnGrid(start, s.loc) {
		return domain.ErrInvalidSlot
	}

	w, works, err := s.store.WorkingWindow(ctx, doctorID, start.In(s.loc).Weekday())
	if err != nil {
		return err
	}

	if !works || !InWindow(start, w, s.loc) {
		return domain.ErrOutsideWorkingHours
	}

	if start.Before(s.now().Add(s.minLead)) {
		return domain.ErrSlotInPast
	}

	return nil
}

func (s *Service) Book(ctx context.Context, doctorID int64, patientID int64, start time.Time) (domain.Appointment, error) {

	ok, err := s.store.DoctorExists(ctx, doctorID)

	if err != nil {
		return domain.Appointment{}, err
	}

	if !ok {
		return domain.Appointment{}, domain.ErrDoctorNotFound
	}

	ok, err = s.store.PatientExists(ctx, patientID)
	if err != nil {
		return domain.Appointment{}, err
	}

	if !ok {
		return domain.Appointment{}, domain.ErrPatientNotFound
	}

	if err := s.ValidateSlot(ctx, doctorID, start); err != nil {
		return domain.Appointment{}, err
	}

	return s.store.CreateAppointment(ctx, doctorID, patientID, start)
}

func (s *Service) Cancel(ctx context.Context, id int64, reason string) (domain.Appointment, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return domain.Appointment{}, domain.ErrReasonRequired
	}
	return s.store.CancelAppointment(ctx, id, reason)
}

func (s *Service) Reschedule(ctx context.Context, id int64, start time.Time) (domain.Appointment, error) {
	appt, err := s.store.GetAppointment(ctx, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	if appt.Status == "cancelled" {
		return domain.Appointment{}, domain.ErrAlreadyCancelled
	}
	if appt.StartTime.Equal(start) {
		return domain.Appointment{}, domain.ErrSameSlot
	}
	if err := s.ValidateSlot(ctx, appt.DoctorID, start); err != nil {
		return domain.Appointment{}, err
	}
	return s.store.RescheduleAppointment(ctx, id, start)
}

func (s *Service) PatientAppointments(ctx context.Context, patientID int64) ([]domain.Appointment, error) {
	ok, err := s.store.PatientExists(ctx, patientID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrPatientNotFound
	}
	return s.store.UpcomingForPatient(ctx, patientID, s.now())
}
