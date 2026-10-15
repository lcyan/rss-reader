package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"rss-reader/globals"
	"rss-reader/models"

	"rss-reader/utils"
	"time"

	"github.com/gorilla/websocket"
)

var websocketWriteTimeout = 10 * time.Second

func init() {
	globals.Init()
}

func main() {
	go utils.UpdateFeeds()
	go utils.WatchConfigFileChanges("config.json")
	http.HandleFunc("/feeds", getFeedsHandler)
	http.HandleFunc("/ws", wsHandler)
	// http.HandleFunc("/", serveHome)
	http.HandleFunc("/", tplHandler)

	//加载静态文件
	fs := http.FileServer(http.FS(globals.DirStatic))
	http.Handle("/static/", fs)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html; charset=utf-8")
	w.Write(globals.HtmlContent)
}

func tplHandler(w http.ResponseWriter, r *http.Request) {
	// 创建一个新的模板，并设置自定义分隔符为<< >>，避免与Vue的语法冲突
	tmplInstance := template.New("index.html").Delims("<<", ">>")
	//添加加法函数计数
	funcMap := template.FuncMap{
		"inc": func(i int) int {
			return i + 1
		},
	}
	// 加载模板文件
	tmpl, err := tmplInstance.Funcs(funcMap).ParseFS(globals.DirStatic, "static/index.html")
	if err != nil {
		log.Println("模板加载错误:", err)
		return
	}

	config := globals.GetConfig()
	darkMode := isDarkMode(time.Now(), config.NightStartTime, config.NightEndTime)

	// 定义一个数据对象
	data := struct {
		Keywords       string
		RssDataList    []models.Feed
		DarkMode       bool
		AutoUpdatePush int
	}{
		Keywords:       getKeywords(),
		RssDataList:    utils.GetFeeds(),
		DarkMode:       darkMode,
		AutoUpdatePush: config.AutoUpdatePush,
	}

	// 渲染模板并将结果写入响应
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Println("模板渲染错误:", err)
	}
}

func isDarkMode(now time.Time, start, end string) bool {
	formattedTime := now.Format("15:04:05")
	if start == "" || end == "" {
		return false
	}
	if start > end {
		return formattedTime >= start || formattedTime < end
	}
	return start > formattedTime || formattedTime > end
}

func autoUpdatePushDuration(interval int) (time.Duration, bool) {
	if interval <= 0 {
		return 0, false
	}
	return time.Duration(interval) * time.Minute, true
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := globals.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade failed: %v", err)
		return
	}

	defer conn.Close()
	for {
		config := globals.GetConfig()
		for _, url := range config.Values {
			globals.Lock.RLock()
			cache, ok := globals.DbMap[url]
			globals.Lock.RUnlock()
			if !ok {
				log.Printf("Error getting feed from db is null %v", url)
				continue
			}
			data, err := json.Marshal(cache)
			if err != nil {
				log.Printf("json marshal failure: %s", err.Error())
				continue
			}

			err = writeWebSocketMessage(conn, websocket.TextMessage, data)
			//错误直接关闭更新
			if err != nil {
				log.Printf("Error sending message or Connection closed: %v", err)
				return
			}
		}
		//如果未配置则不自动更新
		pushInterval, enabled := autoUpdatePushDuration(config.AutoUpdatePush)
		if !enabled {
			return
		}
		time.Sleep(pushInterval)
	}
}

func writeWebSocketMessage(conn *websocket.Conn, messageType int, data []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(websocketWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, data)
}

// 获取关键词也就是title
// 获取feeds列表
func getKeywords() string {
	words := ""
	config := globals.GetConfig()
	for _, url := range config.Values {
		globals.Lock.RLock()
		cache, ok := globals.DbMap[url]
		globals.Lock.RUnlock()
		if !ok {
			log.Printf("Error getting feed from db is null %v", url)
			continue
		}
		if cache.Title != "" {
			words += cache.Title + ","
		}
	}
	return words
}

func getFeedsHandler(w http.ResponseWriter, r *http.Request) {
	feeds := utils.GetFeeds()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(feeds)
}
