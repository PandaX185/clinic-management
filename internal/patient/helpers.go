package patient

import (
	"time"

	"github.com/PandaX185/clinic-management/internal/platform/apperr"
)

func invalid(msg string) error { return apperr.Invalid(msg) }

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02T15:04:05Z07:00")
	return &s
}
