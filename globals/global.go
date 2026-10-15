package globals

import (
	"bufio"
	"embed"
	"fmt"
	"log"
	"os"
	"rss-reader/models"
	"strings"
	"sync"

	"github.com/mmcdole/gofeed"

	"github.com/gorilla/websocket"
)

var (
	DbMap         map[string]models.Feed
	RssUrls       models.Config
	Upgrader      = websocket.Upgrader{}
	Lock          sync.RWMutex
	ConfigLock    sync.RWMutex
	MatchList     = make([]string, 0)
	DenyMatchList = make([]string, 0)
	Hash          = make(map[string]int)
	HashLock      sync.RWMutex
	Pending       = make(map[string]struct{})
	Fp            = gofeed.NewParser()

	//go:embed static
	DirStatic embed.FS

	HtmlContent []byte
)

func Init() {
	conf, err := models.ParseConf()
	if err != nil {
		log.Printf("config parse failed: %v", err)
		return
	}
	ConfigLock.Lock()
	RssUrls = conf
	MatchList = append([]string(nil), conf.Keywords...)
	DenyMatchList = append([]string(nil), conf.DenyKeywords...)
	ConfigLock.Unlock()
	// 读取 index.html 内容
	HtmlContent, err = DirStatic.ReadFile("static/index.html")
	if err != nil {
		panic(err)
	}

	Lock.Lock()
	if DbMap == nil {
		DbMap = make(map[string]models.Feed)
	}
	Lock.Unlock()

	ReadFile(conf.Archives)
}

func applyConfigKeywords(conf models.Config) {
	ConfigLock.Lock()
	defer ConfigLock.Unlock()
	MatchList = append([]string(nil), conf.Keywords...)
	DenyMatchList = append([]string(nil), conf.DenyKeywords...)
}

func GetConfig() models.Config {
	ConfigLock.RLock()
	defer ConfigLock.RUnlock()
	return RssUrls
}

func GetMatchLists() ([]string, []string) {
	ConfigLock.RLock()
	defer ConfigLock.RUnlock()
	return append([]string(nil), MatchList...), append([]string(nil), DenyMatchList...)
}

func ReadFile(filepath string) {
	file, err := os.Open(filepath)
	if err != nil {
		log.Println("Error opening file:", err)
		return
	}

	scanner := bufio.NewScanner(file)
	loadedHash := make(map[string]int)
	loadedEntries := make([]string, 0)
	for scanner.Scan() { // 逐行扫描
		text := scanner.Text()
		text = strings.TrimSpace(text)
		if text != "" {
			if _, exists := loadedHash[text]; !exists {
				loadedHash[text] = 1
				loadedEntries = append(loadedEntries, text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		_ = file.Close()
		log.Println("Error reading archive file:", err)
		return
	}
	if err := file.Close(); err != nil {
		log.Println("Error closing archive file:", err)
		return
	}
	if info, err := os.Stat(filepath); err == nil {
		if info.Size() != int64(len(strings.Join(loadedEntries, "\n"))+len(loadedEntries)) {
			compact := strings.Join(loadedEntries, "\n") + "\n"
			if err := os.WriteFile(filepath, []byte(compact), info.Mode()); err != nil {
				log.Println("Error compacting archive file:", err)
			}
		}
	}

	HashLock.Lock()
	Hash = loadedHash
	HashLock.Unlock()
}

func ClaimNotification(link string) bool {
	HashLock.Lock()
	defer HashLock.Unlock()

	if _, ok := Hash[link]; ok {
		return false
	}
	if _, ok := Pending[link]; ok {
		return false
	}
	Pending[link] = struct{}{}
	return true
}

func FinishNotification(link string, success bool) bool {
	HashLock.Lock()
	defer HashLock.Unlock()

	delete(Pending, link)
	if !success {
		return false
	}
	if _, ok := Hash[link]; ok {
		return false
	}
	Hash[link] = 1
	return true
}

func HasArchived(link string) bool {
	HashLock.RLock()
	defer HashLock.RUnlock()
	_, ok := Hash[link]
	return ok
}

func WriteFile(filepath string, s string) error {
	// 打开文件，如果文件不存在则创建，如果存在则以追加模式打开
	writeFile, errOpen := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if errOpen != nil {
		log.Printf("open result file err: %+v", errOpen)
		return errOpen
	}

	_, errWrite := writeFile.WriteString(fmt.Sprintf("%s\n", s))
	if errWrite != nil {
		log.Println("writing to file error:", errWrite)
		return errWrite
	}

	defer writeFile.Close()
	return nil
}
