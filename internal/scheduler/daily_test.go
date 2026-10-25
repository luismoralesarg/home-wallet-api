package scheduler

import (
	"testing"
	"time"

	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

func TestNextRun(t *testing.T) {
	at := func(y int, m time.Month, d, h, min int) time.Time {
		return time.Date(y, m, d, h, min, 0, 0, tz.Mendoza)
	}
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"antes de las 9", at(2026, 10, 15, 8, 0), at(2026, 10, 15, 9, 0)},
		{"justo a las 9 va al día siguiente", at(2026, 10, 15, 9, 0), at(2026, 10, 16, 9, 0)},
		{"después de las 9", at(2026, 10, 15, 20, 30), at(2026, 10, 16, 9, 0)},
		{"fin de mes", at(2026, 10, 31, 12, 0), at(2026, 11, 1, 9, 0)},
		{"se interpreta en Mendoza aunque llegue en UTC", at(2026, 10, 15, 8, 0).UTC(), at(2026, 10, 15, 9, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextRun(tc.now, 9); !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
