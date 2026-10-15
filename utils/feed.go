package utils

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"rss-reader/globals"
	"rss-reader/models"
	"strings"
	"sync"
	"time"

	"github.com/imroc/req/v3"

	"github.com/mmcdole/gofeed"

	"github.com/fsnotify/fsnotify"
)

var rssRequestTimeout = 30 * time.Second
var rssMaxResponseBodySize int64 = 4 << 20
var activeFeedUpdates sync.Map

func UpdateFeeds() {
	updateFeeds(func(duration time.Duration) bool {
		time.Sleep(duration)
		return true
	})
}

func updateFeeds(wait func(time.Duration) bool) {
	for {
		config := globals.GetConfig()
		refresh, err := refreshDuration(config.ReFresh)
		if err != nil {
			log.Printf("invalid refresh interval: %v", err)
			return
		}
		formattedTime := time.Now().Format("2006-01-02 15:04:05")
		for _, url := range config.Values {
			go UpdateFeed(url, formattedTime)
		}
		if !wait(refresh) {
			return
		}
	}
}

func refreshDuration(refresh int) (time.Duration, error) {
	if refresh <= 0 {
		return 0, fmt.Errorf("refresh must be greater than zero")
	}
	return time.Duration(refresh) * time.Second, nil
}

func UpdateFeed(url, formattedTime string) {
	if !claimFeedUpdate(url) {
		return
	}
	defer activeFeedUpdates.Delete(url)

	log.Printf("timer exec get: %s\n", url)
	result, err := loadRssData(url)
	if err != nil {
		log.Printf("Error fetching feed: %v | %v", url, err)
		return
	}
	//feed内容无更新时无需更新缓存
	globals.Lock.RLock()
	cache, cacheOk := globals.DbMap[url]
	globals.Lock.RUnlock()
	if cacheOk && feedUnchanged(cache, result) {
		return
	}
	customFeed := models.Feed{
		Title:  result.Title,
		Link:   url,
		Custom: map[string]string{"lastupdate": formattedTime},
		Items:  make([]models.Item, 0, len(result.Items)),
	}
	for _, v := range result.Items {
		customFeed.Items = append(customFeed.Items, models.Item{
			Link:        v.Link,
			Title:       v.Title,
			Description: v.Description,
		})
		Check(url, result, v)
	}
	globals.Lock.Lock()
	defer globals.Lock.Unlock()
	globals.DbMap[url] = customFeed
}

func claimFeedUpdate(url string) bool {
	_, loaded := activeFeedUpdates.LoadOrStore(url, struct{}{})
	return !loaded
}

func feedUnchanged(cache models.Feed, result *gofeed.Feed) bool {
	if len(cache.Items) != len(result.Items) {
		return false
	}
	for i, item := range result.Items {
		if item == nil {
			return false
		}
		cached := cache.Items[i]
		if cached.Link != item.Link || cached.Title != item.Title || cached.Description != item.Description {
			return false
		}
	}
	return true
}

func loadRssData(url string) (*gofeed.Feed, error) {
	fp := gofeed.NewParser()
	reqClient := req.C().ImpersonateChrome().SetTimeout(rssRequestTimeout)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := reqClient.GetClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("RSS request returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, rssMaxResponseBodySize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > rssMaxResponseBodySize {
		return nil, fmt.Errorf("RSS response exceeds %d bytes", rssMaxResponseBodySize)
	}

	return fp.ParseString(string(body))
}

// GetFeeds 获取feeds列表
func GetFeeds() []models.Feed {
	config := globals.GetConfig()
	feeds := make([]models.Feed, 0, len(config.Values))
	for _, url := range config.Values {
		globals.Lock.RLock()
		cache, ok := globals.DbMap[url]
		globals.Lock.RUnlock()
		if !ok {
			log.Printf("Error getting feed from db is null %v", url)
			continue
		}

		feeds = append(feeds, cache)
	}
	return feeds
}

func WatchConfigFileChanges(filePath string) {
	targetPath, err := filepath.Abs(filePath)
	if err != nil {
		log.Fatal(err)
	}
	targetPath = filepath.Clean(targetPath)

	// 创建一个新的监控器
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	// 添加要监控的文件
	err = watcher.Add(filepath.Dir(targetPath))
	if err != nil {
		log.Fatal(err)
	}

	// 启动一个 goroutine 来处理文件变化事件
	go func() {
		for {
			time.Sleep(7 * time.Second)
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					log.Println("通道关闭1")
					return
				}
				eventPath, err := filepath.Abs(event.Name)
				if err != nil || filepath.Clean(eventPath) != targetPath || !shouldReloadConfig(event) {
					continue
				}
				if _, err := os.Stat(targetPath); err == nil {
					log.Println("文件已修改")
					globals.Init()
					formattedTime := time.Now().Format("2006-01-02 15:04:05")
					for _, url := range globals.GetConfig().Values {
						go UpdateFeed(url, formattedTime)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					log.Println("通道关闭2")
					return
				}
				log.Println("错误:", err)
				return
			}
		}
	}()

	select {}
}

func shouldReloadConfig(event fsnotify.Event) bool {
	return event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0
}

func Check(url string, result *gofeed.Feed, v *gofeed.Item) {
	globals.Lock.RLock()
	cache, cacheOk := globals.DbMap[url]
	globals.Lock.RUnlock()
	if shouldNotifyItem(cache, cacheOk, v) {

		link := v.Link
		link = strings.TrimSpace(link)
		linkStrSplitForParam := strings.Split(link, "?")
		if linkStrSplitForParam != nil && len(linkStrSplitForParam) != 0 {
			link = linkStrSplitForParam[0]
		}
		linkStrSplitForRoute := strings.Split(link, "#")
		if linkStrSplitForRoute != nil && len(linkStrSplitForRoute) != 0 {
			link = linkStrSplitForRoute[0]
		}

		// 临时修改
		if strings.Contains(link, "nodeloc.com") {
			nodelocLinkArr := strings.Split(link, "/")
			if nodelocLinkArr != nil && len(nodelocLinkArr) != 0 {
				link = strings.TrimSuffix(link, "/"+nodelocLinkArr[len(nodelocLinkArr)-1])
			}
		}

		if globals.HasArchived(link) {
			return
		}

		// 匹配关键词
		allow := MatchAllowList(v.Title)

		if allow {
			deny := MatchDenyList(v.Title)
			if !deny {
				TryNotify(v.Title, link)
			}
		}
	}
}

func shouldNotifyItem(cache models.Feed, cacheOk bool, item *gofeed.Item) bool {
	if !cacheOk || item == nil {
		return !cacheOk
	}
	for _, cached := range cache.Items {
		if cached.Link == item.Link {
			return false
		}
	}
	return true
}

func TryNotify(msg string, link string) {
	if !globals.ClaimNotification(link) {
		return
	}

	routes := []string{FeiShuRoute, TelegramRoute}
	notificationProgress.Lock()
	completed := notificationProgress.completed[link]
	pendingRoutes := make([]string, 0, len(routes))
	for _, route := range routes {
		if _, ok := completed[route]; !ok {
			pendingRoutes = append(pendingRoutes, route)
		}
	}
	notificationProgress.Unlock()

	err := notifyRoutes(Message{
		Routes:  pendingRoutes,
		Content: fmt.Sprintf("%s\n%s", msg, link),
	}, pendingRoutes, func(route string) {
		notificationProgress.Lock()
		if notificationProgress.completed[link] == nil {
			notificationProgress.completed[link] = make(map[string]struct{})
		}
		notificationProgress.completed[link][route] = struct{}{}
		notificationProgress.Unlock()
	})
	if err != nil {
		globals.FinishNotification(link, false)
		log.Printf("notification failed for %s: %v", link, err)
		return
	}

	if err := globals.WriteFile(globals.GetConfig().Archives, link); err != nil {
		globals.FinishNotification(link, false)
		notificationProgress.Lock()
		delete(notificationProgress.completed, link)
		notificationProgress.Unlock()
		log.Printf("archive write failed for %s: %v", link, err)
		return
	}

	if globals.FinishNotification(link, true) {
		notificationProgress.Lock()
		delete(notificationProgress.completed, link)
		notificationProgress.Unlock()
	}
}
