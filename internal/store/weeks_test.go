package store

import (
	"reflect"
	"testing"
	"time"
)

func TestFillEmptyWeeks(t *testing.T) {
	now := time.Date(2026, time.January, 14, 12, 0, 0, 0, time.UTC) // 2026-W03
	rows := []WeekRow{
		{Week: "2026-W01", Km: 10, Hours: 1},
		{Week: "2025-W51", Km: 5, Hours: 0.5},
	}
	got := fillEmptyWeeks(rows, now)

	var labels []string
	for _, r := range got {
		labels = append(labels, r.Week)
	}
	want := []string{"2026-W03", "2026-W02", "2026-W01", "2025-W52", "2025-W51"}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("weeks = %v, want %v", labels, want)
	}
	if got[2].Km != 10 || got[1].Km != 0 {
		t.Fatalf("unexpected values: %+v", got)
	}
}

func TestFillEmptyWeeksYear53(t *testing.T) {
	// 2020 has 53 ISO weeks.
	now := time.Date(2021, time.January, 5, 0, 0, 0, 0, time.UTC) // 2021-W01
	rows := []WeekRow{{Week: "2020-W52", Km: 1}}
	got := fillEmptyWeeks(rows, now)
	if len(got) != 3 || got[1].Week != "2020-W53" {
		t.Fatalf("got %+v", got)
	}
}

func TestFillEmptyWeeksEmpty(t *testing.T) {
	if got := fillEmptyWeeks(nil, time.Now()); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}
