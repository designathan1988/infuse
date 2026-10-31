package scheduler

import (
	"testing"
	"time"
)

func TestNextRun(t *testing.T) {
	loc := time.Local
	mon := time.Date(2026, 11, 2, 10, 0, 0, 0, loc) // a Monday
	daily := Task{Active: true, Preset: "daily", Hour: 9, Minute: 30}
	if got := time.UnixMilli(NextRun(daily, mon)); got.Day() != 3 || got.Hour() != 9 || got.Minute() != 30 {
		t.Fatalf("daily %v", got)
	}
	fri := time.Date(2026, 11, 6, 18, 0, 0, 0, loc)
	wk := Task{Active: true, Preset: "weekdays", Hour: 8}
	if got := time.UnixMilli(NextRun(wk, fri)); got.Weekday() != time.Monday {
		t.Fatalf("weekdays %v", got)
	}
	hourly := Task{Active: true, Preset: "hourly", Minute: 5}
	if got := time.UnixMilli(NextRun(hourly, mon)); got.Hour() != 10 || got.Minute() != 5 {
		t.Fatalf("hourly %v", got)
	}
	weekly := Task{Active: true, Preset: "weekly", Weekday: 3, Hour: 7}
	if got := time.UnixMilli(NextRun(weekly, mon)); got.Weekday() != time.Wednesday {
		t.Fatalf("weekly %v", got)
	}
	if NextRun(Task{Active: false, Preset: "daily"}, mon) != 0 || NextRun(Task{Active: true, Preset: "manual"}, mon) != 0 {
		t.Fatal("paused/manual must not schedule")
	}
}
