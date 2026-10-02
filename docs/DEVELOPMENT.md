# 開發環境

維護者與 AI 接手用的開發文件。產品使用方式在 [`README.md`](../README.md)；上游同步在 [`UPSTREAM.md`](UPSTREAM.md)；決策在 [`DECISIONS.md`](DECISIONS.md)。

## 架構

```text
.codex-plugin/                         Codex 外掛定義 (plugin.json)
release/windows/                       Windows 發佈與安裝控制腳本 (deploy.ps1, startup-manager 等)
scripts/
  ├── bin/                             可執行二進位檔 (codex-auto-retry.exe, codex-auto-retry-mcp.exe)
  ├── source/                          Go 核心原始碼與 RPC 協定實作
  │   └── ui/                          TypeScript 前端管理面板 (可嵌入 MCP)
  ├── installer/                       單檔安裝程式原始碼（Go，無外部相依）
  ├── build.ps1                        二進位檔與面板建置腳本
  ├── build-release.ps1                打包壓縮檔與單檔安裝程式
  └── smoke-test.ps1                   完整 PowerShell 冒煙測試集
tools/                                 fork 維護工具（Windows gate、上游檢查、依賴新鮮度）
  └── tests/                           維護契約測試
docs/                                  fork 維護與治理文件
```

## 本機開發（Windows 11 原生）

### 維護骨架（必跑）

```powershell
python -m venv .venv
.venv\Scripts\python -m pip install --upgrade pip
.venv\Scripts\python -m pip install -r requirements-dev.txt
```

### 驗證門禁

```powershell
# 1. 執行 Windows 本機一鍵門禁（格式、靜態檢查、契約測試、相對連結檢查）
pwsh -NoProfile -File tools\dev_check.ps1

# 2. 執行產品冒煙測試
pwsh -NoProfile -File scripts\smoke-test.ps1
```

### 建置與發佈

```powershell
# 1. 改過 scripts\source 就要重建，否則打包會被來源雜湊檢查擋下
pwsh -NoProfile -File scripts\build.ps1

# 2. 打包（輸出壓縮檔、單檔安裝程式與各自的 SHA-256）
pwsh -NoProfile -File scripts\build-release.ps1 -SkipBuild -OutputDirectory $env:TEMP\car-release

# 3. 驗證發佈檔
pwsh -NoProfile -File scripts\release-test.ps1 -ArchivePath $env:TEMP\car-release\Codex-Auto-Retry-<版本>-windows-x64.zip
```

推送到 `main` 後由 CI 跑完整驗證（競態測試需要 gcc，本機通常沒有）。

### 繁體中文化

```powershell
# 同步上游後執行；需要 OpenCC（pip install opencc），不在日常 gate 的依賴內
python tools\convert_zh_hant.py --check   # 只列出會變動的檔案
python tools\convert_zh_hant.py           # 轉換並寫回，之後重跑 scripts\build.ps1
```

PowerShell 測試裡用 `[char]0xXXXX` 拼出的中文不會被工具處理，需人工對照。

### 上游追蹤

```powershell
# 檢查上游最新變更
python tools\check_upstream_updates.py --strict
```
