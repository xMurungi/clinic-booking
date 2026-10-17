package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/xMurungi/clinic-booking/internal/booking"
	"github.com/xMurungi/clinic-booking/internal/domain"
	"github.com/xMurungi/clinic-booking/internal/httpx"
)

type Service interface {
	Availability(ctx context.Context, doctorID int64, date time.Time) ([]booking.Slot, error)
	Book(ctx context.Context, doctorID, patientID int64, start time.Time) (domain.Appointment, error)
	Cancel(ctx context.Context, id int64, reason string) (domain.Appointment, error)
	Reschedule(ctx context.Context, id int64, start time.Time) (domain.Appointment, error)
	PatientAppointments(ctx context.Context, patientID int64) ([]domain.Appointment, error)
}

type API struct {
	svc Service
	loc *time.Location
}

func New(svc Service, loc *time.Location) *API {
	return &API{
		svc: svc,
		loc: loc,
	}
}

func (a *API) Availability(w http.ResponseWriter, r *http.Request) {

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil || id <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "doctor id must be a positive integer")
		return
	}

	dateStr := r.URL.Query().Get("date")
	date, err := time.ParseInLocation("2006-01-02", dateStr, a.loc)

	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_date", "date is required in YYYY-MM-DD format")
		return
	}

	slots, err := a.svc.Availability(r.Context(), id, date)

	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"doctor_id": id,
		"date":      dateStr,
		"timezone":  a.loc.String(),
		"slots":     slots,
	})
}

func (a *API) PatientAppointments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	appts, err := a.svc.PatientAppointments(r.Context(), id)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"patient_id":   id,
		"appointments": appts,
	})
}
