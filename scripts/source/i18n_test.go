package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUILanguageDefaultsToChineseAndReadsSavedChoice(t *testing.T) {
	dir := t.TempDir()
	if got := uiLanguage(dir); got != languageChinese {
		t.Fatalf("missing file: got %q", got)
	}
	for _, content := range []string{`{"language":"en"}`, "\xef\xbb\xbf{\"language\":\"en\"}"} {
		if err := os.WriteFile(filepath.Join(dir, uiLanguageFile), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := uiLanguage(dir); got != languageEnglish {
			t.Fatalf("%q: got %q", content, got)
		}
	}
	for _, content := range []string{`{"language":"fr"}`, `not json`} {
		if err := os.WriteFile(filepath.Join(dir, uiLanguageFile), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := uiLanguage(dir); got != languageChinese {
			t.Fatalf("%q: got %q", content, got)
		}
	}
}

func TestSetUILanguageRejectsUnknownLanguages(t *testing.T) {
	dir := t.TempDir()
	if err := setUILanguage(dir, "fr"); err == nil {
		t.Fatal("unknown language was accepted")
	}
	if err := setUILanguage(dir, languageEnglish); err != nil || uiLanguage(dir) != languageEnglish {
		t.Fatalf("english was not saved: %v", err)
	}
}

func TestManagementNoticeFollowsInterfaceLanguage(t *testing.T) {
	dir := t.TempDir()
	service := newManagementService(dir)
	now := time.Now().UTC()
	snapshot, err := service.setUILanguage(languageEnglish, now)
	if err != nil || snapshot.UILanguage != languageEnglish || snapshot.Notice != "Interface language set to English" {
		t.Fatalf("english notice: %+v, %v", snapshot, err)
	}
	snapshot, err = service.setPaused(true, now)
	if err != nil || snapshot.Notice != "Automatic retry paused" {
		t.Fatalf("english pause notice: %q, %v", snapshot.Notice, err)
	}
	snapshot, err = service.setUILanguage(languageChinese, now)
	if err != nil || snapshot.UILanguage != languageChinese || snapshot.Notice != "介面語言已改為繁體中文" {
		t.Fatalf("chinese notice: %+v, %v", snapshot, err)
	}
}
