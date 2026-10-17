package domain

import (
	"time"
)

type Appointment struct {
	ID        int64     `json:"id"`
	DoctorID  int64     `json:"doctor_id"`
	PatientID int64     `json:"patient_id"`
	StartTime time.Time `json:"start_time"`
	Status    string    `json:"status"`
}
