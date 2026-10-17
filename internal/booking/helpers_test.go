package booking

import "time"

var nairobi = mustLoc("Africa/Nairobi")

func mustLoc(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// at returns the given clock time on Monday 2026-10-19, Nairobi time.
func at(h, m int) time.Time {
	return time.Date(2026, 10, 19, h, m, 0, 0, nairobi)
}

var workday = Window{StartMin: 9 * 60, EndMin: 17 * 60}
