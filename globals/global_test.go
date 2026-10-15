package globals

import (
	"os"
	"os/exec"
	"testing"

	"rss-reader/models"
)

func TestWriteFileFailureDoesNotExitProcess(t *testing.T) {
	if os.Getenv("RSS_READER_WRITE_FILE_CHILD") == "1" {
		WriteFile(os.Getenv("RSS_READER_WRITE_FILE_TARGET"), "article")
		return
	}

	target := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWriteFileFailureDoesNotExitProcess$")
	cmd.Env = append(os.Environ(),
		"RSS_READER_WRITE_FILE_CHILD=1",
		"RSS_READER_WRITE_FILE_TARGET="+target,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("WriteFile failure exited the process: %v", err)
	}
}

func TestInitInvalidConfigDoesNotExitProcess(t *testing.T) {
	if os.Getenv("RSS_READER_INIT_CHILD") == "1" {
		Init()
		return
	}

	target := t.TempDir()
	if err := os.WriteFile(target+"\\config.json", []byte(`{"values":[`), 0644); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestInitInvalidConfigDoesNotExitProcess$")
	cmd.Dir = target
	cmd.Env = append(os.Environ(), "RSS_READER_INIT_CHILD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Init with invalid config exited the process: %v\n%s", err, output)
	}
}

func TestInitPreservesFeedCacheDuringConfigReload(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(configDir+"\\config.json", []byte(`{"values":[],"archives":"archives.txt"}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Chdir(configDir)

	Lock.Lock()
	DbMap = map[string]models.Feed{
		"https://example.com/feed": {Title: "cached"},
	}
	Lock.Unlock()

	Init()

	Lock.RLock()
	_, ok := DbMap["https://example.com/feed"]
	Lock.RUnlock()
	if !ok {
		t.Fatal("config reload must preserve existing feed cache")
	}
}

func TestReadFileReplacesStaleArchiveHash(t *testing.T) {
	archivePath := t.TempDir() + "\\archives.txt"
	if err := os.WriteFile(archivePath, []byte("new-link\n"), 0644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	HashLock.Lock()
	Hash = map[string]int{"stale-link": 1}
	HashLock.Unlock()

	ReadFile(archivePath)

	HashLock.RLock()
	_, stale := Hash["stale-link"]
	_, current := Hash["new-link"]
	HashLock.RUnlock()
	if stale {
		t.Fatal("reloading archives must remove stale in-memory links")
	}
	if !current {
		t.Fatal("reloading archives must load current links")
	}
}

func TestReadFileCompactsDuplicateArchiveEntries(t *testing.T) {
	archivePath := t.TempDir() + "\\archives.txt"
	content := "same-link\nother-link\nsame-link\n"
	if err := os.WriteFile(archivePath, []byte(content), 0644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	ReadFile(archivePath)

	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read compacted archive: %v", err)
	}
	if string(data) != "same-link\nother-link\n" {
		t.Fatalf("archive = %q, want duplicate-free entries", string(data))
	}
}

func TestApplyConfigKeywordsReplacesPreviousValues(t *testing.T) {
	MatchList = nil
	DenyMatchList = nil

	applyConfigKeywords(models.Config{Keywords: []string{"old"}, DenyKeywords: []string{"old-deny"}})
	applyConfigKeywords(models.Config{Keywords: []string{"new"}, DenyKeywords: []string{"new-deny"}})

	if len(MatchList) != 1 || MatchList[0] != "new" {
		t.Fatalf("match list = %#v, want [\"new\"]", MatchList)
	}
	if len(DenyMatchList) != 1 || DenyMatchList[0] != "new-deny" {
		t.Fatalf("deny match list = %#v, want [\"new-deny\"]", DenyMatchList)
	}
}
