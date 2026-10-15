package utils

import (
	"github.com/dlclark/regexp2"
	"log"
	"rss-reader/globals"
	"strings"
)

func MatchAllowList(str string) bool {
	matchList, _ := globals.GetMatchLists()
	return MatchStr(str, matchList)
}

func MatchDenyList(str string) bool {
	_, denyMatchList := globals.GetMatchLists()
	return MatchStr(str, denyMatchList)
}

func MatchStr(str string, matchList []string) bool {
	strFinal := strings.TrimSpace(str)
	for _, pattern := range matchList {
		pattern = "(?i:" + pattern + ")"
		re, err := regexp2.Compile(pattern, 0)
		if err != nil {
			log.Printf("⚠️ Invalid regular expression: %s, error: %v", pattern, err)
			continue
		}

		isMatch, _ := re.MatchString(strFinal)
		if isMatch {
			return true
		}
	}
	return false
}
