//go:build race

package stats

// raceEnabled: the race detector slows SQLite down 10–30×, so time budgets are not measured.
const raceEnabled = true
