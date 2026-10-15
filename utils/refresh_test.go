package utils

import (
	"testing"
	"time"

	"rss-reader/globals"
	"rss-reader/models"
)

func TestUpdateFeedsDoesNotDelayBetweenURLs(t *testing.T) {
	globals.RssUrls = models.Config{
		ReFresh: 1,
		Values:  []string{"://invalid-one", "://invalid-two"},
	}

	started := time.Now()
	updateFeeds(func(time.Duration) bool { return false })
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("feed cycle took %v, URL launch delay must not dominate the refresh interval", elapsed)
	}
}

func TestUpdateFeedsUsesReloadedRefreshInterval(t *testing.T) {
	globals.RssUrls = models.Config{ReFresh: 60}
	var waits []time.Duration

	updateFeeds(func(duration time.Duration) bool {
		waits = append(waits, duration)
		if len(waits) == 1 {
			globals.RssUrls.ReFresh = 1
			return true
		}
		return false
	})

	if len(waits) != 2 || waits[0] != time.Minute || waits[1] != time.Second {
		t.Fatalf("refresh waits = %#v, want [1m0s 1s]", waits)
	}
}
