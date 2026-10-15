package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"rss-reader/globals"
	"rss-reader/models"
	"strings"
	"sync"
	"time"
)

const (
	FeiShuRoute   = "feishu"
	TelegramRoute = "telegram"
	ContentType   = "application/json"
	TokenReplace  = "${token}"
)

type Message struct {
	Routes  []string `json:"routes"`
	Content string   `json:"content"`
}

type FeiShuMessage struct {
	MsgType string            `json:"msg_type"`
	Content FeiShuMessageText `json:"content"`
}

type FeiShuMessageText struct {
	Text string `json:"text"`
}

type TelegramMessage struct {
	ChatId string `json:"chat_id"`
	Text   string `json:"text"`
}

var notificationRequestTimeout = 30 * time.Second
var notificationMaxResponseBodySize int64 = 1 << 20
var telegramRateLimitDelay = 1500 * time.Millisecond

var notificationProgress = struct {
	sync.Mutex
	completed map[string]map[string]struct{}
}{completed: make(map[string]map[string]struct{})}

func Notify(msg Message) error {
	if msg.Routes == nil || len(msg.Routes) == 0 {
		return nil
	}
	return notifyRoutes(msg, msg.Routes, nil)
}

func notifyRoutes(msg Message, routes []string, onSuccess func(string)) error {
	config := globals.GetConfig()
	sent := false
	var firstErr error
	for _, route := range routes {
		var err error
		configured := false
		switch route {
		case FeiShuRoute:
			if config.Notify.FeiShu.API != "" {
				configured = true
				sent = true
				err = sendToFeiShu(msg, config)
			}
		case TelegramRoute:
			if config.Notify.Telegram.Token != "" && config.Notify.Telegram.ChatId != "" {
				configured = true
				time.Sleep(telegramRateLimitDelay)
				sent = true
				err = sendToTelegram(msg, config)
			}
		default:
			log.Println("without route")
		}
		if configured && err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if configured && onSuccess != nil {
			onSuccess(route)
		}
	}
	if !sent {
		return fmt.Errorf("no notification route configured")
	}
	return firstErr
}
func sendToTelegram(msg Message, config models.Config) error {
	finalMsg, err := json.Marshal(
		TelegramMessage{
			ChatId: config.Notify.Telegram.ChatId,
			Text:   msg.Content,
		})
	if err != nil {
		log.Printf("json marshal err: %+v\n", err)
		return err
	}
	api := strings.ReplaceAll(config.Notify.Telegram.API, TokenReplace, config.Notify.Telegram.Token)
	return requestPost(api, finalMsg)
}

func sendToFeiShu(msg Message, config models.Config) error {
	finalMsg, err := json.Marshal(
		FeiShuMessage{
			MsgType: "text",
			Content: FeiShuMessageText{
				Text: msg.Content,
			},
		})
	if err != nil {
		log.Printf("json marshal err: %+v\n", err)
		return err
	}
	return requestPost(config.Notify.FeiShu.API, finalMsg)
}

func requestPost(url string, param []byte) error {
	requestBody := bytes.NewBuffer(param)
	client := &http.Client{Timeout: notificationRequestTimeout}
	resp, err := client.Post(url, ContentType, requestBody)

	if err != nil {
		log.Printf("http post err: %+v\n", err)
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("http body close err: %+v\n", err)
		}
	}(resp.Body)
	body, err := io.ReadAll(io.LimitReader(resp.Body, notificationMaxResponseBodySize+1)) // 读取响应内容
	if err != nil {
		log.Printf("http post read body err: %+v\n", err)
		return err
	}
	if int64(len(body)) > notificationMaxResponseBodySize {
		return fmt.Errorf("notification response exceeds %d bytes", notificationMaxResponseBodySize)
	}
	log.Printf("response status: %s,response body:%s", string(body), resp.Status)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("notification request returned %s", resp.Status)
	}
	return nil
}
