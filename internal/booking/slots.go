package booking

import "time"

const (
	SlotMinutes = 30
	SlotLength  = SlotMinutes * time.Minute
)

// Window is a doctor's working hours for one weekday,
// as minutes since local midnight (09:00 = 540, 17:00 = 1020).
type Window struct {
	StartMin int
	EndMin   int
}

type Slot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// GenerateSlots returns every 30-minute slot in the window for the given
// calendar day, interpreted in loc (the clinic's timezone).
// A slot must fit entirely inside the window.
func GenerateSlots(day time.Time, w Window, loc *time.Location) []Slot {
	y, m, d := day.In(loc).Date()

	var slots []Slot
	for min := w.StartMin; min+SlotMinutes <= w.EndMin; min += SlotMinutes {
		// time.Date normalises minute overflow (540 -> 09:00).
		start := time.Date(y, m, d, 0, min, 0, 0, loc)
		slots = append(slots, Slot{Start: start, End: start.Add(SlotLength)})
	}
	return slots
}

// FilterAvailable removes booked slots and slots that start before
// now+minLead (use 0 for "just not in the past", 1h for the bonus rule).
func FilterAvailable(slots []Slot, booked []time.Time, now time.Time, minLead time.Duration) []Slot {
	taken := make(map[int64]struct{}, len(booked))
	for _, b := range booked {
		taken[b.Unix()] = struct{}{}
	}

	cutoff := now.Add(minLead)
	out := make([]Slot, 0, len(slots))
	for _, s := range slots {
		if _, isTaken := taken[s.Start.Unix()]; isTaken {
			continue
		}
		if s.Start.Before(cutoff) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// IsValidSlot reports whether start is aligned to the 30-minute grid and
// lies fully inside the doctor's window. Booking and rescheduling both use it.

func OnGrid(start time.Time, loc *time.Location) bool {
	l := start.In(loc)
	return l.Second() == 0 && l.Nanosecond() == 0 && l.Minute()%SlotMinutes == 0
}

func InWindow(start time.Time, w Window, loc *time.Location) bool {
	l := start.In(loc)
	min := l.Hour()*60 + l.Minute()
	return min >= w.StartMin && min+SlotMinutes <= w.EndMin
}

func IsValidSlot(start time.Time, w Window, loc *time.Location) bool {
	return OnGrid(start, loc) && InWindow(start, w, loc)
}
