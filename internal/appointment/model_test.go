package appointment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanTransition_ValidPaths(t *testing.T) {
	valid := []struct {
		from, to Status
	}{
		{StatusScheduled, StatusConfirmed},
		{StatusScheduled, StatusCancelled},
		{StatusScheduled, StatusNoShow},
		{StatusConfirmed, StatusCompleted},
		{StatusConfirmed, StatusCancelled},
		{StatusConfirmed, StatusNoShow},
	}
	for _, tc := range valid {
		assert.Truef(t, CanTransition(tc.from, tc.to), "expected %s -> %s to be allowed", tc.from, tc.to)
	}
}

func TestCanTransition_TerminalStatesAreFrozen(t *testing.T) {
	for _, from := range []Status{StatusCompleted, StatusCancelled, StatusNoShow} {
		for _, to := range []Status{StatusScheduled, StatusConfirmed, StatusCompleted, StatusCancelled, StatusNoShow} {
			assert.Falsef(t, CanTransition(from, to), "terminal status %s must not transition to %s", from, to)
		}
	}
}

func TestCanTransition_InvalidSkips(t *testing.T) {
	invalid := []struct {
		from, to Status
	}{
		{StatusCompleted, StatusConfirmed},
		{StatusCancelled, StatusScheduled},
		{StatusNoShow, StatusConfirmed},
		{StatusScheduled, StatusCompleted},
	}
	for _, tc := range invalid {
		assert.Falsef(t, CanTransition(tc.from, tc.to), "expected %s -> %s to be rejected", tc.from, tc.to)
	}
}

func TestIsActive(t *testing.T) {
	assert.True(t, IsActive(StatusScheduled))
	assert.True(t, IsActive(StatusConfirmed))
	assert.False(t, IsActive(StatusCompleted))
	assert.False(t, IsActive(StatusCancelled))
	assert.False(t, IsActive(StatusNoShow))
}
