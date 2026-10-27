package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }
func ptr(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

func TestMonthsBetweenIsInclusiveAndCrossesYears(t *testing.T) {
	got := MonthsBetween(Month{2026, 11}, Month{2027, 2})
	want := []string{"2026-11", "2026-12", "2027-01", "2027-02"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i].String() != want[i] {
			t.Fatalf("%d: %s != %s", i, got[i], want[i])
		}
	}
	if n := len(MonthsBetween(Month{2026, 5}, Month{2026, 5})); n != 1 {
		t.Fatalf("un solo mes: %d", n)
	}
	if n := len(MonthsBetween(Month{2026, 6}, Month{2026, 5})); n != 0 {
		t.Fatalf("rango invertido: %d", n)
	}
}

func TestAdjustByPercent(t *testing.T) {
	cases := []struct{ amount, percent, want string }{
		{"1000.00", "10", "1100.00"},
		{"1000.00", "7.5", "1075.00"},
		{"333.33", "10", "366.66"}, // 366.663 → 366.66
		{"100.05", "10", "110.06"}, // 110.055 → half-up → 110.06
		{"1000.00", "0", "1000.00"},
	}
	for _, c := range cases {
		if got := Adjust(d(c.amount), ptr(c.percent), nil); !got.Equal(d(c.want)) {
			t.Errorf("%s + %s%% = %s, quería %s", c.amount, c.percent, got, c.want)
		}
	}
}

func TestAdjustByAmount(t *testing.T) {
	if got := Adjust(d("1000"), nil, ptr("250.50")); !got.Equal(d("1250.50")) {
		t.Fatalf("suma: %s", got)
	}
	if got := Adjust(d("1000"), nil, ptr("-100")); !got.Equal(d("900")) {
		t.Fatalf("resta: %s", got)
	}
}

func TestNextChargeDate(t *testing.T) {
	ar := func(y int, m time.Month, day, h int) time.Time {
		return time.Date(y, m, day, h, 0, 0, 0, time.FixedZone("AR", -3*3600))
	}
	cases := []struct {
		name string
		day  int
		now  time.Time
		want string
	}{
		{"todavía no cobró este mes", 20, ar(2026, 10, 10, 12), "2026-10-20"},
		{"cobra hoy", 20, ar(2026, 10, 20, 23), "2026-10-20"},
		{"ya cobró este mes", 20, ar(2026, 10, 21, 0), "2026-11-20"},
		{"cruza de año", 15, ar(2026, 12, 16, 12), "2027-01-15"},
		{"día 31 en febrero cobra a fin de mes", 31, ar(2026, 2, 10, 12), "2026-02-28"},
		{"día 31 pasado el 28 de febrero pasa a marzo", 31, ar(2026, 3, 1, 12), "2026-03-31"},
		// 01:00 UTC del 21 son las 22:00 del 20 en Mendoza: hoy todavía es 20.
		{"borde UTC/Mendoza", 20, time.Date(2026, 10, 21, 1, 0, 0, 0, time.UTC), "2026-10-20"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := (Subscription{ChargeDay: c.day}).NextChargeDate(c.now).Format("2006-01-02"); got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}
