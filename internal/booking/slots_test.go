package booking

import (
	"testing"
	"time"
)

func TestGenerateSlots(t *testing.T) {
	slots := GenerateSlots(at(0, 0), workday, nairobi)

	if len(slots) != 16 { // 8 hours * 2
		t.Fatalf("want 16 slots, got %d", len(slots))
	}
	if !slots[0].Start.Equal(at(9, 0)) {
		t.Errorf("first slot = %v, want 09:00", slots[0].Start)
	}
	if !slots[15].End.Equal(at(17, 0)) {
		t.Errorf("last slot ends %v, want 17:00", slots[15].End)
	}
}

func TestGenerateSlots_PartialTrailingSlotDropped(t *testing.T) {
	w := Window{StartMin: 9 * 60, EndMin: 9*60 + 45} // 09:00-09:45
	slots := GenerateSlots(at(0, 0), w, nairobi)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
}

func TestFilterAvailable(t *testing.T) {
	slots := GenerateSlots(at(0, 0), workday, nairobi)
	booked := []time.Time{at(10, 0), at(14, 30)}
	now := at(8, 0)

	got := FilterAvailable(slots, booked, now, 0)
	if len(got) != 14 {
		t.Fatalf("want 14 available, got %d", len(got))
	}
	for _, s := range got {
		if s.Start.Equal(at(10, 0)) || s.Start.Equal(at(14, 30)) {
			t.Errorf("booked slot %v still listed", s.Start)
		}
	}
}

func TestFilterAvailable_PastAndLeadTime(t *testing.T) {
	slots := GenerateSlots(at(0, 0), workday, nairobi)
	now := at(12, 10)

	got := FilterAvailable(slots, nil, now, 0)
	if !got[0].Start.Equal(at(12, 30)) {
		t.Errorf("first = %v, want 12:30", got[0].Start)
	}

	got = FilterAvailable(slots, nil, now, time.Hour) // cutoff 13:10
	if !got[0].Start.Equal(at(13, 30)) {
		t.Errorf("first = %v, want 13:30", got[0].Start)
	}
}

func TestFilterAvailable_CompareByInstantNotZone(t *testing.T) {
	slots := GenerateSlots(at(0, 0), workday, nairobi)
	// 10:00 Nairobi == 07:00 UTC; the DB returns times in UTC.
	booked := []time.Time{at(10, 0).UTC()}
	got := FilterAvailable(slots, booked, at(8, 0), 0)
	if len(got) != 15 {
		t.Fatalf("want 15, got %d (UTC booked time not matched)", len(got))
	}
}

func TestIsValidSlot(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		want  bool
	}{
		{"first slot", at(9, 0), true},
		{"last slot", at(16, 30), true},
		{"half past", at(11, 30), true},
		{"before opening", at(8, 30), false},
		{"at closing", at(17, 0), false},
		{"off grid", at(10, 15), false},
		{"has seconds", at(10, 0).Add(5 * time.Second), false},
		{"valid given in UTC", at(10, 0).UTC(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidSlot(tt.start, workday, nairobi); got != tt.want {
				t.Errorf("IsValidSlot(%v) = %v, want %v", tt.start, got, tt.want)
			}
		})
	}
}
