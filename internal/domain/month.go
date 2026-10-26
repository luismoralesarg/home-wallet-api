package domain

import (
	"fmt"
	"time"

	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

// Month es un mes calendario (formato "YYYY-MM" en la API).
type Month struct {
	Year int
	Mon  time.Month
}

// MonthOf devuelve el mes de un instante, en hora Mendoza.
func MonthOf(t time.Time) Month {
	d := tz.DateOf(t)
	return Month{d.Year(), d.Month()}
}

// MonthOfDate devuelve el mes de una fecha ya normalizada (p. ej. una columna DATE).
func MonthOfDate(d time.Time) Month { return Month{d.Year(), d.Month()} }

func ParseMonth(s string) (Month, error) {
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return Month{}, fmt.Errorf("mes inválido %q (formato YYYY-MM)", s)
	}
	return Month{t.Year(), t.Month()}, nil
}

func (m Month) String() string { return fmt.Sprintf("%04d-%02d", m.Year, int(m.Mon)) }

// Start es el primer día del mes (00:00 UTC), como se guarda en columnas DATE.
func (m Month) Start() time.Time { return time.Date(m.Year, m.Mon, 1, 0, 0, 0, 0, time.UTC) }

func (m Month) AddMonths(n int) Month {
	t := m.Start().AddDate(0, n, 0)
	return Month{t.Year(), t.Month()}
}

func (m Month) Before(o Month) bool { return m.Year < o.Year || (m.Year == o.Year && m.Mon < o.Mon) }
func (m Month) After(o Month) bool  { return o.Before(m) }

// Day devuelve el día `d` del mes; si el mes es más corto, el último día (cierre el 31 en febrero → 28/29).
func (m Month) Day(d int) time.Time {
	last := m.AddMonths(1).Start().AddDate(0, 0, -1).Day()
	if d > last {
		d = last
	}
	return time.Date(m.Year, m.Mon, d, 0, 0, 0, 0, time.UTC)
}
