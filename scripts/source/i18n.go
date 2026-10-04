package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The interface language is one per-user choice shared by the settings
// window, the startup manager, the tray, and the embedded panel. It lives in
// ui-language.json in the data directory; Traditional Chinese is the default.
const (
	languageChinese = "zh"
	languageEnglish = "en"
	uiLanguageFile  = "ui-language.json"
)

func uiLanguage(dataDir string) string {
	data, err := os.ReadFile(filepath.Join(dataDir, uiLanguageFile))
	if err != nil {
		return languageChinese
	}
	var record struct {
		Language string `json:"language"`
	}
	// Windows PowerShell may write the file with a UTF-8 BOM.
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if json.Unmarshal(data, &record) == nil && record.Language == languageEnglish {
		return languageEnglish
	}
	return languageChinese
}

func setUILanguage(dataDir, language string) error {
	if language != languageChinese && language != languageEnglish {
		return fmt.Errorf("unsupported interface language %q", language)
	}
	return writeJSONAtomic(filepath.Join(dataDir, uiLanguageFile), map[string]string{"language": language})
}

// text picks the string for the chosen language.
func text(language, zh, en string) string {
	if language == languageEnglish {
		return en
	}
	return zh
}
