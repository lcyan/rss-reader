package utils

import (
	"fmt"
	"github.com/imroc/req/v3"
	"log"
	"rss-reader/globals"
	"rss-reader/models"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/fsnotify/fsnotify"
)

func UpdateFeeds() {
	var (
		tick = time.Tick(time.Duration(globals.RssUrls.ReFresh) * time.Second)
	)
	for {
		formattedTime := time.Now().Format("2006-01-02 15:04:05")
		for _, url := range globals.RssUrls.Values {
			go UpdateFeed(url, formattedTime)
			time.Sleep(3 * time.Second)
		}
		<-tick
	}
}

func UpdateFeed(url, formattedTime string) {
	log.Printf("timer exec get: %s\n", url)
	result, err := loadRssData(url)
	if err != nil {
		log.Printf("Error fetching feed: %v | %v", url, err)
		return
	}
	//feed内容无更新时无需更新缓存
	if cache, ok := globals.DbMap[url]; ok &&
		len(result.Items) > 0 &&
		len(cache.Items) > 0 &&
		result.Items[0].Link == cache.Items[0].Link {
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

func loadRssData(url string) (*gofeed.Feed, error) {
	fp := gofeed.NewParser()
	reqClient := req.C().ImpersonateChrome()
	resp, err := reqClient.R().Get(url)
	if err != nil {
		return nil, err
	}

	return fp.ParseString(resp.String())
}

// GetFeeds 获取feeds列表
func GetFeeds() []models.Feed {
	feeds := make([]models.Feed, 0, len(globals.RssUrls.Values))
	for _, url := range globals.RssUrls.Values {
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
	// 创建一个新的监控器
	watcher, err := fsnotify.NewWatcher()
	if (err != nil) {
		log.Fatal(err)
	}
	defer watcher.Close()

	// 添加要监控的文件
	err = watcher.Add(filePath)
	if (err != nil) {
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
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Println("文件已修改")
					globals.Init()
					formattedTime := time.Now().Format("2006-01-02 15:04:05")
					for _, url := range globals.RssUrls.Values {
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

func Check(url string, result *gofeed.Feed, v *gofeed.Item) {
	cache, cacheOk := globals.DbMap[url]
	if !cacheOk || cache.Items[0].Link != result.Items[0].Link {

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

		_, fileCacheOk := globals.Hash[link]
		if fileCacheOk {
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

func TryNotify(msg string, link string) {
	fileCacheOk := false
	_, fileCacheOk = globals.Hash[link]
	if fileCacheOk {
		return
	} else {
		globals.Hash[link] = 1
		// 发送通知
		go Notify(Message{
			Routes:  []string{FeiShuRoute, TelegramRoute},
			Content: fmt.Sprintf("%s\n%s", msg, link),
		})
		globals.WriteFile(globals.RssUrls.Archives, link)
	}
}
