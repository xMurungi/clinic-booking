package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/xMurungi/clinic-booking/internal/domain"
	"github.com/xMurungi/clinic-booking/internal/httpx"
)

type bookRequest struct {
	DoctorID  int64     `json:"doctor_id"`
	PatientID int64     `json:"patient_id"`
	StartTime time.Time `json:"start_time"` // RFC3339, e.g. 2026-10-19T10:00:00+03:00
}

func (a *API) Book(w http.ResponseWriter, r *http.Request) {

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req bookRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.DoctorID <= 0 || req.PatientID <= 0 || req.StartTime.IsZero() {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body",
			"doctor_id, patient_id and start_time are required")
		return
	}

	appt, err := a.svc.Book(r.Context(), req.DoctorID, req.PatientID, req.StartTime)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}

	w.Header().Set("Location", "/appointments/"+strconv.FormatInt(appt.ID, 10))
	httpx.WriteJSON(w, http.StatusCreated, appt)
}

// One place that maps domain errors to HTTP; cancel/reschedule will reuse it.
func (a *API) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {

	switch {
	case errors.Is(err, domain.ErrDoctorNotFound):
		httpx.WriteError(w, http.StatusNotFound, "doctor_not_found", err.Error())
	case errors.Is(err, domain.ErrPatientNotFound):
		httpx.WriteError(w, http.StatusNotFound, "patient_not_found", err.Error())
	case errors.Is(err, domain.ErrInvalidSlot):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "invalid_slot", err.Error())
	case errors.Is(err, domain.ErrOutsideWorkingHours):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "outside_working_hours", err.Error())
	case errors.Is(err, domain.ErrSlotInPast):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "slot_in_past", err.Error())
	case errors.Is(err, domain.ErrSlotTaken):
		httpx.WriteError(w, http.StatusConflict, "slot_taken", err.Error())
	case errors.Is(err, domain.ErrAppointmentNotFound):
		httpx.WriteError(w, http.StatusNotFound, "appointment_not_found", err.Error())
	case errors.Is(err, domain.ErrAlreadyCancelled):
		httpx.WriteError(w, http.StatusConflict, "appointment_cancelled", err.Error())
	case errors.Is(err, domain.ErrReasonRequired):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", err.Error())
	case errors.Is(err, domain.ErrSameSlot):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "same_slot", err.Error())
	default:
		slog.ErrorContext(r.Context(), "request failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

// new handlers

func (a *API) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	appt, err := a.svc.Cancel(r.Context(), id, req.Reason)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, appt)
}

func (a *API) Reschedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		StartTime time.Time `json:"start_time"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.StartTime.IsZero() {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "start_time is required (RFC3339)")
		return
	}
	appt, err := a.svc.Reschedule(r.Context(), id, req.StartTime)
	if err != nil {
		a.writeDomainError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, appt)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
		return 0, false
	}
	return id, true
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		slog.WarnContext(r.Context(), "bad request body", "err", err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON for this endpoint")
		return false
	}
	return true
}
