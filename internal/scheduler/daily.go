package scheduler

import (
	"context"
	"time"

	"github.com/luismoralesarg/home-wallet-api/internal/tz"
)

// NextRun devuelve el próximo instante > now en que son las `hour`:00 en Mendoza.
func NextRun(now time.Time, hour int) time.Time {
	local := now.In(tz.Mendoza)
	next := time.Date(local.Year(), local.Month(), local.Day(), hour, 0, 0, 0, tz.Mendoza)
	if !next.After(local) {
		next = time.Date(local.Year(), local.Month(), local.Day()+1, hour, 0, 0, 0, tz.Mendoza)
	}
	return next
}

// RunDaily ejecuta fn todos los días a las `hour`:00 (Mendoza) hasta que ctx se cancele.
func RunDaily(ctx context.Context, hour int, fn func(context.Context)) {
	for {
		timer := time.NewTimer(time.Until(NextRun(time.Now(), hour)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			fn(ctx)
		}
	}
}
