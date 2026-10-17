package booking

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/xMurungi/clinic-booking/internal/domain"
)

// fakeStore is an in-memory stand-in mirroring the DB's rules:
// one active booking per doctor+slot; cancelled rows don't block.
type fakeStore struct {
	appts  map[int64]domain.Appointment
	nextID int64
}

var _ Store = (*fakeStore)(nil) // compile-time check: fake stays in sync with Store

func newFakeStore() *fakeStore {
	return &fakeStore{appts: map[int64]domain.Appointment{}, nextID: 1}
}

func (f *fakeStore) slotTaken(doctorID int64, start time.Time, exceptID int64) bool {
	for _, a := range f.appts {
		if a.ID != exceptID && a.DoctorID == doctorID &&
			a.Status == "booked" && a.StartTime.Equal(start) {
			return true
		}
	}
	return false
}

func (f *fakeStore) DoctorExists(context.Context, int64) (bool, error)  { return true, nil }
func (f *fakeStore) PatientExists(context.Context, int64) (bool, error) { return true, nil }

func (f *fakeStore) WorkingWindow(context.Context, int64, time.Weekday) (Window, bool, error) {
	return workday, true, nil
}

func (f *fakeStore) BookedStarts(_ context.Context, doctorID int64, from, to time.Time) ([]time.Time, error) {
	var out []time.Time
	for _, a := range f.appts {
		if a.DoctorID == doctorID && a.Status == "booked" &&
			!a.StartTime.Before(from) && a.StartTime.Before(to) {
			out = append(out, a.StartTime)
		}
	}
	return out, nil
}

func (f *fakeStore) CreateAppointment(_ context.Context, d, p int64, start time.Time) (domain.Appointment, error) {
	if f.slotTaken(d, start, 0) {
		return domain.Appointment{}, domain.ErrSlotTaken
	}
	a := domain.Appointment{ID: f.nextID, DoctorID: d, PatientID: p, StartTime: start, Status: "booked"}
	f.appts[a.ID] = a
	f.nextID++
	return a, nil
}

func (f *fakeStore) GetAppointment(_ context.Context, id int64) (domain.Appointment, error) {
	a, ok := f.appts[id]
	if !ok {
		return domain.Appointment{}, domain.ErrAppointmentNotFound
	}
	return a, nil
}

func (f *fakeStore) CancelAppointment(_ context.Context, id int64, _ string) (domain.Appointment, error) {
	a, ok := f.appts[id]
	if !ok {
		return domain.Appointment{}, domain.ErrAppointmentNotFound
	}
	if a.Status == "cancelled" {
		return domain.Appointment{}, domain.ErrAlreadyCancelled
	}
	a.Status = "cancelled"
	f.appts[id] = a
	return a, nil
}

func (f *fakeStore) RescheduleAppointment(_ context.Context, id int64, start time.Time) (domain.Appointment, error) {
	a, ok := f.appts[id]
	if !ok {
		return domain.Appointment{}, domain.ErrAppointmentNotFound
	}
	if a.Status == "cancelled" {
		return domain.Appointment{}, domain.ErrAlreadyCancelled
	}
	if f.slotTaken(a.DoctorID, start, id) {
		return domain.Appointment{}, domain.ErrSlotTaken
	}
	a.StartTime = start
	f.appts[id] = a
	return a, nil
}

func (f *fakeStore) UpcomingForPatient(_ context.Context, patientID int64, from time.Time) ([]domain.Appointment, error) {
	var out []domain.Appointment
	for _, a := range f.appts {
		if a.PatientID == patientID && a.Status == "booked" && !a.StartTime.Before(from) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartTime.Before(out[j].StartTime) })
	return out, nil
}

// newSvc gives each test a fresh store and a clock fixed at Monday 08:00.
func newSvc() *Service {
	now := at(8, 0)
	return NewService(newFakeStore(), nairobi, func() time.Time { return now }, 0)
}

func mustBook(t *testing.T, svc *Service, start time.Time) domain.Appointment {
	t.Helper()
	a, err := svc.Book(context.Background(), 1, 1, start)
	if err != nil {
		t.Fatalf("setup booking at %v failed: %v", start, err)
	}
	return a
}

func TestBook(t *testing.T) {
	svc := newSvc()
	ctx := context.Background()

	tests := []struct {
		name  string
		start time.Time
		want  error
	}{
		{"ok", at(10, 0), nil},
		{"taken", at(10, 0), domain.ErrSlotTaken}, // same slot again
		{"off grid", at(10, 15), domain.ErrInvalidSlot},
		{"before hours", at(8, 30), domain.ErrOutsideWorkingHours},
		{"at closing", at(17, 0), domain.ErrOutsideWorkingHours},
		{"before opening", at(7, 0), domain.ErrOutsideWorkingHours},
		{"past inside hours", time.Date(2026, 10, 12, 10, 0, 0, 0, nairobi), domain.ErrSlotInPast},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Book(ctx, 1, 1, tt.start)
			if !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestBook_MinLead(t *testing.T) {
	now := at(10, 0) // inside working hours, so only the lead-time rule can reject
	svc := NewService(newFakeStore(), nairobi, func() time.Time { return now }, time.Hour)
	ctx := context.Background()

	if _, err := svc.Book(ctx, 1, 1, at(11, 0)); err != nil {
		t.Errorf("exactly 1h ahead should be allowed, got %v", err)
	}
	if _, err := svc.Book(ctx, 1, 1, at(10, 30)); !errors.Is(err, domain.ErrSlotInPast) {
		t.Errorf("30 min ahead should be rejected, got %v", err)
	}
}

func TestAvailability(t *testing.T) {
	svc := newSvc()
	mustBook(t, svc, at(10, 0))

	slots, err := svc.Availability(context.Background(), 1, at(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 15 {
		t.Fatalf("want 15 slots (16 minus the booked one), got %d", len(slots))
	}
	for _, s := range slots {
		if s.Start.Equal(at(10, 0)) {
			t.Error("booked 10:00 slot still listed")
		}
	}
}

func TestCancel(t *testing.T) {
	ctx := context.Background()

	t.Run("requires a reason", func(t *testing.T) {
		svc := newSvc()
		a := mustBook(t, svc, at(10, 0))
		_, err := svc.Cancel(ctx, a.ID, "   ")
		if !errors.Is(err, domain.ErrReasonRequired) {
			t.Errorf("got %v, want ErrReasonRequired", err)
		}
	})

	t.Run("cancels and frees the slot", func(t *testing.T) {
		svc := newSvc()
		a := mustBook(t, svc, at(10, 0))

		got, err := svc.Cancel(ctx, a.ID, "patient travelling")
		if err != nil || got.Status != "cancelled" {
			t.Fatalf("got (%v, %v), want cancelled", got.Status, err)
		}
		mustBook(t, svc, at(10, 0)) // slot bookable again
	})

	t.Run("cancelling twice fails", func(t *testing.T) {
		svc := newSvc()
		a := mustBook(t, svc, at(10, 0))
		_, _ = svc.Cancel(ctx, a.ID, "reason")
		_, err := svc.Cancel(ctx, a.ID, "reason")
		if !errors.Is(err, domain.ErrAlreadyCancelled) {
			t.Errorf("got %v, want ErrAlreadyCancelled", err)
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		_, err := newSvc().Cancel(ctx, 999, "reason")
		if !errors.Is(err, domain.ErrAppointmentNotFound) {
			t.Errorf("got %v, want ErrAppointmentNotFound", err)
		}
	})
}

func TestReschedule(t *testing.T) {
	ctx := context.Background()

	t.Run("moves and frees the old slot", func(t *testing.T) {
		svc := newSvc()
		a := mustBook(t, svc, at(10, 0))

		got, err := svc.Reschedule(ctx, a.ID, at(11, 0))
		if err != nil {
			t.Fatal(err)
		}
		if !got.StartTime.Equal(at(11, 0)) || got.ID != a.ID {
			t.Errorf("got id=%d start=%v, want same id at 11:00", got.ID, got.StartTime)
		}
		mustBook(t, svc, at(10, 0)) // old slot is free
	})

	t.Run("target slot taken", func(t *testing.T) {
		svc := newSvc()
		a := mustBook(t, svc, at(10, 0))
		mustBook(t, svc, at(11, 0))
		_, err := svc.Reschedule(ctx, a.ID, at(11, 0))
		if !errors.Is(err, domain.ErrSlotTaken) {
			t.Errorf("got %v, want ErrSlotTaken", err)
		}
	})

	t.Run("cancelled appointment", func(t *testing.T) {
		svc := newSvc()
		a := mustBook(t, svc, at(10, 0))
		_, _ = svc.Cancel(ctx, a.ID, "reason")
		_, err := svc.Reschedule(ctx, a.ID, at(11, 0))
		if !errors.Is(err, domain.ErrAlreadyCancelled) {
			t.Errorf("got %v, want ErrAlreadyCancelled", err)
		}
	})

	t.Run("same slot", func(t *testing.T) {
		svc := newSvc()
		a := mustBook(t, svc, at(10, 0))
		_, err := svc.Reschedule(ctx, a.ID, at(10, 0))
		if !errors.Is(err, domain.ErrSameSlot) {
			t.Errorf("got %v, want ErrSameSlot", err)
		}
	})

	t.Run("target validated like a fresh booking", func(t *testing.T) {
		tests := []struct {
			name  string
			start time.Time
			want  error
		}{
			{"off grid", at(10, 15), domain.ErrInvalidSlot},
			{"outside hours", at(18, 0), domain.ErrOutsideWorkingHours},
			{"in past", time.Date(2026, 10, 12, 10, 0, 0, 0, nairobi), domain.ErrSlotInPast},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc := newSvc()
				a := mustBook(t, svc, at(9, 0))
				_, err := svc.Reschedule(ctx, a.ID, tt.start)
				if !errors.Is(err, tt.want) {
					t.Errorf("got %v, want %v", err, tt.want)
				}
			})
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		_, err := newSvc().Reschedule(ctx, 999, at(11, 0))
		if !errors.Is(err, domain.ErrAppointmentNotFound) {
			t.Errorf("got %v, want ErrAppointmentNotFound", err)
		}
	})
}

func TestPatientAppointments(t *testing.T) {
	svc := newSvc()
	mustBook(t, svc, at(15, 0))
	mustBook(t, svc, at(9, 0))
	c := mustBook(t, svc, at(11, 0))
	_, _ = svc.Cancel(context.Background(), c.ID, "reason")

	got, err := svc.PatientAppointments(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].StartTime.Equal(at(9, 0)) || !got[1].StartTime.Equal(at(15, 0)) {
		t.Errorf("want [09:00, 15:00] sorted without the cancelled one, got %+v", got)
	}
}
