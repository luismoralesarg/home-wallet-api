// Package tz centraliza la zona horaria que define "día" y "mes".
package tz

import (
	"time"
	_ "time/tzdata" // la imagen distroless no trae zoneinfo
)

var Mendoza = mustLoad("America/Argentina/Mendoza")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// Today devuelve la fecha (a medianoche UTC, sin hora) del día en Mendoza.
func Today(now time.Time) time.Time { return DateOf(now) }

// DateOf devuelve el día calendario de t en Mendoza, como fecha UTC a las 00:00.
func DateOf(t time.Time) time.Time {
	y, m, d := t.In(Mendoza).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
