package utils

import "testing"

func TestRefreshDurationRejectsNonPositiveValues(t *testing.T) {
	for _, refresh := range []int{0, -1} {
		if _, err := refreshDuration(refresh); err == nil {
			t.Fatalf("refresh=%d should be rejected", refresh)
		}
	}
}
