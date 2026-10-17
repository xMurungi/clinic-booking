package domain

import "errors"

var (
	ErrDoctorNotFound      = errors.New("doctors not found")
	ErrPatientNotFound     = errors.New("patient not found")
	ErrInvalidSlot         = errors.New("start_time must be on a 30-minute boundary (:00 or :30)")
	ErrOutsideWorkingHours = errors.New("slot is outside the doctor's working hours")
	ErrSlotInPast          = errors.New("slot is in the past or too soon to book")
	ErrSlotTaken           = errors.New("slot is already booked")

	// Cancel and reschedule
	ErrAppointmentNotFound = errors.New("appointment not found")
	ErrAlreadyCancelled    = errors.New("appointment is already cancelled")
	ErrSameSlot            = errors.New("appointment is already at that time")
	ErrReasonRequired      = errors.New("a cancellation reason is required")
)
