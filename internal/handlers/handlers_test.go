package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xMurungi/clinic-booking/internal/booking"
	"github.com/xMurungi/clinic-booking/internal/domain"
)

// fakeSvc returns the configured error from every method.
type fakeSvc struct{ err error }

func (f fakeSvc) Availability(context.Context, int64, time.Time) ([]booking.Slot, error) {
	return []booking.Slot{}, f.err
}
func (f fakeSvc) Book(context.Context, int64, int64, time.Time) (domain.Appointment, error) {
	return domain.Appointment{ID: 1, Status: "booked"}, f.err
}
func (f fakeSvc) Cancel(context.Context, int64, string) (domain.Appointment, error) {
	return domain.Appointment{ID: 1, Status: "cancelled"}, f.err
}
func (f fakeSvc) Reschedule(context.Context, int64, time.Time) (domain.Appointment, error) {
	return domain.Appointment{ID: 1, Status: "booked"}, f.err
}
func (f fakeSvc) PatientAppointments(context.Context, int64) ([]domain.Appointment, error) {
	return []domain.Appointment{}, f.err
}

func newMux(err error) *http.ServeMux {
	api := New(fakeSvc{err: err}, time.UTC)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /appointments", api.Book)
	mux.HandleFunc("PATCH /appointments/{id}/cancel", api.Cancel)
	mux.HandleFunc("PATCH /appointments/{id}/reschedule", api.Reschedule)
	mux.HandleFunc("GET /doctors/{id}/availability", api.Availability)
	mux.HandleFunc("GET /patients/{id}/appointments", api.PatientAppointments)
	return mux
}

func do(mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

const validBook = `{"doctor_id":1,"patient_id":1,"start_time":"2026-10-27T10:00:00+03:00"}`

func TestBook_ErrorMapping(t *testing.T) {
	tests := []struct {
		err  error
		want int
		code string
	}{
		{nil, http.StatusCreated, ""},
		{domain.ErrDoctorNotFound, http.StatusNotFound, "doctor_not_found"},
		{domain.ErrPatientNotFound, http.StatusNotFound, "patient_not_found"},
		{domain.ErrInvalidSlot, http.StatusUnprocessableEntity, "invalid_slot"},
		{domain.ErrOutsideWorkingHours, http.StatusUnprocessableEntity, "outside_working_hours"},
		{domain.ErrSlotInPast, http.StatusUnprocessableEntity, "slot_in_past"},
		{domain.ErrSlotTaken, http.StatusConflict, "slot_taken"},
		{fmt.Errorf("wrapped: %w", domain.ErrSlotTaken), http.StatusConflict, "slot_taken"},
		{errors.New("db exploded"), http.StatusInternalServerError, "internal"},
	}
	for _, tt := range tests {
		name := "nil"
		if tt.err != nil {
			name = tt.err.Error()
		}
		t.Run(name, func(t *testing.T) {
			rec := do(newMux(tt.err), "POST", "/appointments", validBook)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body)
			}
			if tt.code != "" && !strings.Contains(rec.Body.String(), `"`+tt.code+`"`) {
				t.Errorf("body %s missing code %q", rec.Body, tt.code)
			}
		})
	}
}

func TestBook_InvalidBody(t *testing.T) {
	bodies := map[string]string{
		"not json":       `nope`,
		"empty object":   `{}`,
		"unknown field":  `{"doctor_id":1,"patient_id":1,"start_time":"2026-10-27T10:00:00+03:00","x":1}`,
		"bad start_time": `{"doctor_id":1,"patient_id":1,"start_time":"tomorrow"}`,
		"missing doctor": `{"patient_id":1,"start_time":"2026-10-27T10:00:00+03:00"}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			rec := do(newMux(nil), "POST", "/appointments", body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", rec.Code, rec.Body)
			}
		})
	}
}

func TestCancel(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		rec := do(newMux(nil), "PATCH", "/appointments/1/cancel", `{"reason":"travelling"}`)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
	t.Run("already cancelled", func(t *testing.T) {
		rec := do(newMux(domain.ErrAlreadyCancelled), "PATCH", "/appointments/1/cancel", `{"reason":"x"}`)
		if rec.Code != http.StatusConflict {
			t.Errorf("status = %d, want 409", rec.Code)
		}
	})
	t.Run("missing reason", func(t *testing.T) {
		rec := do(newMux(domain.ErrReasonRequired), "PATCH", "/appointments/1/cancel", `{"reason":""}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
	t.Run("not found", func(t *testing.T) {
		rec := do(newMux(domain.ErrAppointmentNotFound), "PATCH", "/appointments/9999/cancel", `{"reason":"x"}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
	t.Run("bad id", func(t *testing.T) {
		rec := do(newMux(nil), "PATCH", "/appointments/abc/cancel", `{"reason":"x"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

func TestReschedule(t *testing.T) {
	body := `{"start_time":"2026-10-27T14:00:00+03:00"}`
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"ok", nil, http.StatusOK},
		{"slot taken", domain.ErrSlotTaken, http.StatusConflict},
		{"cancelled", domain.ErrAlreadyCancelled, http.StatusConflict},
		{"same slot", domain.ErrSameSlot, http.StatusUnprocessableEntity},
		{"invalid slot", domain.ErrInvalidSlot, http.StatusUnprocessableEntity},
		{"not found", domain.ErrAppointmentNotFound, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(newMux(tt.err), "PATCH", "/appointments/1/reschedule", body)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
	t.Run("missing start_time", func(t *testing.T) {
		rec := do(newMux(nil), "PATCH", "/appointments/1/reschedule", `{}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

func TestAvailability(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		rec := do(newMux(nil), "GET", "/doctors/1/availability?date=2026-10-27", "")
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
	t.Run("bad date", func(t *testing.T) {
		rec := do(newMux(nil), "GET", "/doctors/1/availability?date=oops", "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
	t.Run("missing date", func(t *testing.T) {
		rec := do(newMux(nil), "GET", "/doctors/1/availability", "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
	t.Run("unknown doctor", func(t *testing.T) {
		rec := do(newMux(domain.ErrDoctorNotFound), "GET", "/doctors/99/availability?date=2026-10-27", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

func TestPatientAppointments(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		rec := do(newMux(nil), "GET", "/patients/1/appointments", "")
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
	t.Run("unknown patient", func(t *testing.T) {
		rec := do(newMux(domain.ErrPatientNotFound), "GET", "/patients/99/appointments", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}
