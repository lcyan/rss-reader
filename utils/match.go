package utils

import (
	"github.com/dlclark/regexp2"
	"log"
	"rss-reader/globals"
	"strings"
)

func MatchAllowList(str string) bool {
	return MatchStr(str, globals.MatchList)
}

func MatchDenyList(str string) bool {
	return MatchStr(str, globals.DenyMatchList)
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
