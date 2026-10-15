package utils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mmcdole/gofeed"
	"rss-reader/models"
)

func TestLoadRssDataTimesOutWhenServerStalls(t *testing.T) {
	originalTimeout := rssRequestTimeout
	rssRequestTimeout = 50 * time.Millisecond
	defer func() { rssRequestTimeout = originalTimeout }()

	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		_, err := loadRssData(server.URL)
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("test server did not receive the request")
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a stalled RSS request must return an error")
		}
	case <-time.After(750 * time.Millisecond):
		t.Fatal("stalled RSS request did not time out")
	}
}

func TestLoadRssDataRejectsOversizedResponse(t *testing.T) {
	originalLimit := rssMaxResponseBodySize
	rssMaxResponseBodySize = 64
	defer func() { rssMaxResponseBodySize = originalLimit }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := "<rss><channel><title>test</title><description>" + strings.Repeat("x", 128) + "</description></channel></rss>"
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	if _, err := loadRssData(server.URL); err == nil {
		t.Fatal("an oversized RSS response must be rejected")
	}
}

func TestFeedUpdateClaimPreventsOverlappingRequests(t *testing.T) {
	activeFeedUpdates = sync.Map{}
	if !claimFeedUpdate("https://example.com/feed") {
		t.Fatal("first feed update must be allowed")
	}
	defer activeFeedUpdates.Delete("https://example.com/feed")

	if claimFeedUpdate("https://example.com/feed") {
		t.Fatal("overlapping feed update must be skipped")
	}
}

func TestShouldReloadConfigForAtomicReplacement(t *testing.T) {
	if !shouldReloadConfig(fsnotify.Event{Name: "config.json", Op: fsnotify.Rename}) {
		t.Fatal("rename events must trigger config reloads")
	}
	if !shouldReloadConfig(fsnotify.Event{Name: "config.json", Op: fsnotify.Create}) {
		t.Fatal("create events must trigger config reloads")
	}
}

func TestFeedUnchangedDetectsItemsAfterFirstEntry(t *testing.T) {
	cache := models.Feed{Items: []models.Item{
		{Link: "https://example.com/a"},
		{Link: "https://example.com/b"},
	}}
	result := &gofeed.Feed{Items: []*gofeed.Item{
		{Link: "https://example.com/a"},
		{Link: "https://example.com/b"},
		{Link: "https://example.com/c"},
	}}

	if feedUnchanged(cache, result) {
		t.Fatal("a new item after the first entry must invalidate the cache")
	}
}

func TestShouldNotifyItemDetectsAppendedEntry(t *testing.T) {
	cache := models.Feed{Items: []models.Item{{Link: "https://example.com/a"}}}
	result := &gofeed.Feed{Items: []*gofeed.Item{
		{Link: "https://example.com/a"},
		{Link: "https://example.com/b"},
	}}

	if !shouldNotifyItem(cache, true, result.Items[1]) {
		t.Fatal("an appended RSS entry must be eligible for notification")
	}
}
