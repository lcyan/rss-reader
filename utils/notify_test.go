package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"rss-reader/globals"
	"rss-reader/models"
)

func TestTryNotifyRetriesAfterNotificationFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	archivePath := filepath.Join(t.TempDir(), "archives.txt")
	globals.Hash = make(map[string]int)
	globals.RssUrls = models.Config{
		Archives: archivePath,
		Notify: models.Notify{
			Telegram: models.Telegram{
				API:    server.URL,
				ChatId: "chat",
				Token:  "token",
			},
		},
	}

	link := "https://example.com/article"
	TryNotify("article", link)

	if _, ok := globals.Hash[link]; ok {
		t.Fatal("failed notification must not be recorded as delivered")
	}

	TryNotify("article", link)

	if _, ok := globals.Hash[link]; !ok {
		t.Fatal("successful retry must be recorded as delivered")
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if string(data) != fmt.Sprintf("%s\n", link) {
		t.Fatalf("archive = %q, want %q", string(data), link+"\n")
	}
}

func TestTryNotifyDoesNotCommitWhenArchiveWriteFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	globals.Hash = make(map[string]int)
	globals.Pending = make(map[string]struct{})
	globals.RssUrls = models.Config{
		Archives: t.TempDir(),
		Notify: models.Notify{
			FeiShu: models.FeiShu{API: server.URL},
		},
	}

	link := "https://example.com/archive-failure"
	TryNotify("article", link)

	if _, ok := globals.Hash[link]; ok {
		t.Fatal("a notification with a failed archive write must not be recorded as delivered")
	}
}

func TestRequestPostTimesOutWhenNotificationServerStalls(t *testing.T) {
	originalTimeout := notificationRequestTimeout
	notificationRequestTimeout = 50 * time.Millisecond
	defer func() { notificationRequestTimeout = originalTimeout }()

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer func() {
		releaseOnce.Do(func() { close(release) })
		server.CloseClientConnections()
		server.Close()
	}()

	done := make(chan error, 1)
	go func() {
		done <- requestPost(server.URL, []byte(`{"message":"test"}`))
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("test server did not receive the notification request")
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a stalled notification request must return an error")
		}
	case <-time.After(750 * time.Millisecond):
		releaseOnce.Do(func() { close(release) })
		<-done
		t.Fatal("stalled notification request did not time out")
	}
}

func TestRequestPostRejectsOversizedResponse(t *testing.T) {
	originalLimit := notificationMaxResponseBodySize
	notificationMaxResponseBodySize = 64
	defer func() { notificationMaxResponseBodySize = originalLimit }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", 128)))
	}))
	defer server.Close()

	if err := requestPost(server.URL, []byte(`{"message":"test"}`)); err == nil {
		t.Fatal("an oversized notification response must be rejected")
	}
}

func TestNotifyWaitsBeforeTelegramRequest(t *testing.T) {
	originalDelay := telegramRateLimitDelay
	telegramRateLimitDelay = 50 * time.Millisecond
	defer func() { telegramRateLimitDelay = originalDelay }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	globals.RssUrls = models.Config{
		Notify: models.Notify{
			Telegram: models.Telegram{API: server.URL, ChatId: "chat", Token: "token"},
		},
	}

	started := time.Now()
	if err := Notify(Message{Routes: []string{TelegramRoute}, Content: "article"}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed < telegramRateLimitDelay {
		t.Fatalf("Telegram request waited %v, want at least %v", elapsed, telegramRateLimitDelay)
	}
}

func TestTryNotifyDoesNotRepeatSuccessfulRouteOnRetry(t *testing.T) {
	var feishuCalls atomic.Int32
	feishu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		feishuCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer feishu.Close()

	var telegramCalls atomic.Int32
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		telegramCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer telegram.Close()

	globals.Hash = make(map[string]int)
	globals.Pending = make(map[string]struct{})
	notificationProgress.Lock()
	notificationProgress.completed = make(map[string]map[string]struct{})
	notificationProgress.Unlock()
	globals.RssUrls = models.Config{
		Archives: filepath.Join(t.TempDir(), "archives.txt"),
		Notify: models.Notify{
			FeiShu:   models.FeiShu{API: feishu.URL},
			Telegram: models.Telegram{API: telegram.URL, ChatId: "chat", Token: "token"},
		},
	}

	link := "https://example.com/partial-failure"
	TryNotify("article", link)
	TryNotify("article", link)

	if got := feishuCalls.Load(); got != 1 {
		t.Fatalf("successful route calls = %d, want 1", got)
	}
	if got := telegramCalls.Load(); got != 2 {
		t.Fatalf("failed route calls = %d, want 2", got)
	}
}

func TestTryNotifyConcurrentLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	globals.Hash = make(map[string]int)
	globals.RssUrls = models.Config{
		Archives: filepath.Join(t.TempDir(), "archives.txt"),
		Notify: models.Notify{
			FeiShu: models.FeiShu{API: server.URL},
		},
	}

	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			TryNotify("article", fmt.Sprintf("https://example.com/%d", i))
		}(i)
	}
	group.Wait()
}

func TestTryNotifyDoesNotSendSameLinkConcurrently(t *testing.T) {
	var calls atomic.Int32
	firstStarted := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-release
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	globals.Hash = make(map[string]int)
	globals.RssUrls = models.Config{
		Archives: filepath.Join(t.TempDir(), "archives.txt"),
		Notify: models.Notify{
			FeiShu: models.FeiShu{API: server.URL},
		},
	}

	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			TryNotify("article", "https://example.com/same")
		}()
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first notification did not start")
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	group.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("notification calls = %d, want 1", got)
	}
}
