# Codex Auto Retry

[![CI](https://github.com/SanHsien/codex-auto-retry/actions/workflows/ci.yml/badge.svg)](https://github.com/SanHsien/codex-auto-retry/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/sybxxx/codex-auto-retry?label=upstream%20release)](https://github.com/sybxxx/codex-auto-retry/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Platform: Windows](https://img.shields.io/badge/platform-Windows%2010%20%7C%2011-blue.svg)](#)

[English](README.en.md) | 繁體中文

> 本專案為 [`sybxxx/codex-auto-retry`](https://github.com/sybxxx/codex-auto-retry) 的繁體中文維護 fork，遵循 MIT 授權條款。
> 維護差異記錄於 [`FORK.md`](FORK.md) 與 [`docs/DECISIONS.md`](docs/DECISIONS.md)；上游審查清冊位於 [`docs/UPSTREAM.md`](docs/UPSTREAM.md)。

**Codex Auto Retry** 是一款專為 Windows 平台 Codex 打造的開源可靠性守護與原地自動接續工具。它在背景靜默監控 Codex 任務生命週期，當遭遇網路中斷、服務限流（Rate Limit / 5 小時額度重設）、請求逾時、伺服器 5xx 異常或空回覆時，**在原聊天中安全、自動地原地接續執行**，完整保留任務工作區、模型設定與推理參數。

本工具作為 Windows 本地背景輕量守護行程（Watchdog）運作，不需在每個 Codex 會話中手動輸入提示詞。同時提供 Windows 系統匣常駐托盤控制器，以及 Codex 原生內嵌管理面板（基於 MCP 協定），介面開閉皆不影響背景自動復原。

---

## 目錄

- [為什麼需要 Codex Auto Retry？](#為什麼需要-codex-auto-retry)
- [核心接續機制](#核心接續機制)
- [使用者介面](#使用者介面)
  - [Windows 系統匣托盤控制器](#windows-系統匣托盤控制器)
  - [Codex 內嵌管理面板（MCP）](#codex-內嵌管理面板mcp)
- [快速上手（終端使用者）](#快速上手終端使用者)
- [隱私與安全保證](#隱私與安全保證)
- [故障安全設計（Fail-Open）](#故障安全設計fail-open)
- [本機開發與驗收](#本機開發與驗收)
- [維護者與授權條款](#維護者與授權條款)

---

## 為什麼需要 Codex Auto Retry？

長時間運行的 Codex 任務常在工具已執行或模型已產生部分輸出後，遭遇中斷（例如 5 小時用量重設、網路瞬斷或伺服器超載）。

傳統手動重試往往會開新會話，遺失中間狀態或重放已執行的副作用。Codex Auto Retry 確保：

1. **原地續跑**：延續同一個任務，不開替換對話，不重放已完成的工具操作。
2. **Turn 關聯檢驗**：精確比對 `task_started` 與 `task_complete` 的 Turn ID，防止無關成功回合誤標記為復原。
3. **退避曲線**：支援固定（Fixed）、線性遞增（Linear）與指數倍增（Exponential）等待延遲，可自訂上限。
4. **行程守護**：Codex 關閉時自動停止重試倒數，不浪費請求額度。

```text
+--------------------------------------------------------------+
| Windows 11 Desktop (Codex App)                               |
|                                                              |
|   Codex 任務遭遇 Rate Limit、連線中斷或 5xx 伺服器異常       |
|            |                                                 |
|            v (\\.\pipe\codex-ipc 原生命名管道)               |
|   +----------------------------------------------------+     |
|   | codex-auto-retry Watchdog (常駐系統匣背景行程)     |     |
|   | - 零內容紀錄：只看生命週期狀態與旗標               |     |
|   | - 支援退避策略與 Turn ID 精確對齊                  |     |
|   | - 內建 MCP 管理面板（在 Codex 內打字即可開啟）     |     |
|   +----------------------------------------------------+     |
|            |                                                 |
|            v (額度恢復或服務可用)                            |
|   自動接續原聊天續跑（不開新工作階段、不重放已完成動作）     |
+--------------------------------------------------------------+
```

---

## 核心接續機制

### 可重試與不可重試邊界

* **可自動重試的故障**：
  * 網路連線中斷、連線重設與逾時
  * HTTP 5xx 伺服器錯誤
  * Rate Limit 與暫時性容量耗盡（如 5 小時額度重設）
  * 串流傳輸中斷
  * 空回覆（Empty Response：HTTP 200 但無輸出）
  * 暫時性驗證服務不可用（在設定上限內）
* **不可重試（Fail-Closed 即刻停止）**：
  * 使用者主動取消或中止
  * 客戶端無效請求與 HTTP 400 / 404
  * 缺少模型宣告
  * 上下文長度／Token 限制超出
  * 安全政策、權限與審批拒絕

---

## 使用者介面

### Windows 系統匣托盤控制器

守護行程以單一輕量背景行程執行，於 Windows 通知區域顯示圖示：

* **懸停提示（Tooltip）**：即時顯示運作狀態（running、paused、waiting、active、stopped）與下次重試倒數。
* **Explorer 重啟自癒**：Windows 檔案總管重啟時自動重新註冊托盤圖示並恢復狀態。
* **雙擊**：開啟圖形化設定視窗。
* **右鍵選單**：快速暫停／繼續派發、開啟設定或結束守護行程。
* **設定視窗**：支援設定復原上限、延遲曲線、備援提示文字、通知偏好與繁簡英介面切換。

### Codex 內嵌管理面板（MCP）

可在 Codex 對話中直接呼叫：

> `開啟 Codex Auto Retry 管理面板` 或 `Open Codex Auto Retry Management Panel`

採用純 TypeScript 撰寫並透過 Go `embed` 內嵌於 MCP 二進位檔中，零外部網路請求且不需 Node.js 執行期環境。提供：

* 守護行程健康度、監控會話目錄與最後掃描時間。
* 作用中與等待中佇列，附即時倒數計時器。
* 即時手動重試（`Retry Now`）與取消控制項。
* 全域暫停／繼續切換。

---

## 快速上手（終端使用者）

1. 從本 fork 的 [GitHub Releases](https://github.com/SanHsien/codex-auto-retry/releases/latest) 下載繁體中文版，二擇一：
   * **單檔安裝程式** `Codex-Auto-Retry-<版本>-windows-x64-setup.exe`：完全關閉 Codex App 後直接雙擊，不必解壓縮。
   * **壓縮檔** `Codex-Auto-Retry-<版本>-windows-x64.zip`：解壓縮後，在資料夾內雙擊 `安裝.cmd`。
2. 兩者內容相同；單檔安裝程式只是把壓縮檔解到暫存資料夾再執行同一支 `deploy.ps1`。尚未購買程式碼簽章，Windows SmartScreen 可能提示「未知的發行者」，請對照 Release 頁的 SHA-256。
3. 安裝程式自動校驗 SHA-256、設定目前使用者開機啟動，並將守護行程部署於 `%LOCALAPPDATA%\CodexAutoRetry`，同時完成 Codex 外掛註冊。
4. **不需系統管理員權限**，亦不需安裝 Go 或 Node.js。

> 原始碼內的 `release\windows\` 只是安裝範本，沒有 `release-manifest.json` 與 `payload\`，直接執行會出現 `release-manifest.json is missing`。要從原始碼安裝，先執行 `pwsh -NoProfile -File scripts\build-release.ps1`（需要 Go 與 Node.js），它會在 `%USERPROFILE%\releases\codex-auto-retry\` 產生壓縮檔與單檔安裝程式。

### 管理與維護腳本

位於壓縮檔根目錄（原始碼範本在 `release\windows\`）。使用單檔安裝程式時，改用 `setup.exe -uninstall`（加 `-remove-data` 連設定與日誌一併清除）、`setup.exe -safe-disable`，或 `setup.exe -extract <資料夾>` 取出整包：

* `啟動管理員.cmd`：開啟啟動管理員視窗，顯示啟動指令、監護狀態、心跳與 Windows `StartupApproved` 狀態。
* `安全停用.cmd`：一鍵緊急停止腳本，停用共用後端、清理外掛登錄值並恢復 Codex 官方直接執行模式。
* `解除安裝.cmd`：乾淨解除安裝守護行程與外掛，預設保留使用者設定與日誌。執行 `.\uninstall-release.ps1 -RemoveData` 可執行完全清除。

---

## 隱私與安全保證

* **零對話內容紀錄（Zero Content Logging）**：掃描器僅處理生命週期事件與布林進度旗標。使用者提問、助手回覆、工具呼叫、參數、憑證與回應內文絕不解碼、記錄或儲存。
* **嚴格讀取白名單**：僅在復原派發前解碼必要設定（工作目錄、模型、provider、reasoning_effort、審批策略等），其餘欄位全數捨棄。
* **不可逆覆寫保護**：狀態檔案採用原子寫入（Atomic Write），平滑重試檔案衝突。
* **資源硬性上限**：日誌定期滾動（5 MB，最多保留 3 份），單次自動復原鏈設有 30 分鐘硬性中斷保護。

---

## 故障安全設計（Fail-Open）

* **官方 IPC 優先**：支援 Windows Desktop 官方命名管道 `\\.\pipe\codex-ipc`，不啟動本機伺服器、不修改全域網路路由。
* **安全降級**：若守護行程當機或遭遇異常狀態，保證 Fail-Open，Codex 仍能以官方後端正常啟動，絕不卡死應用程式。

---

## 本機開發與驗收

本 fork 採 **Windows 11 原生** 開發環境：

```powershell
# 執行 Windows 原生一鍵門禁檢查
pwsh -NoProfile -File tools\dev_check.ps1

# 執行產品冒煙測試
pwsh -NoProfile -File scripts\smoke-test.ps1
```

---

## 維護者與授權條款

* 原作者：`sybxxx` (TQY Local Tools)，遵循 [MIT License](LICENSE)。
* 本維護 Fork：由 `SanHsien` 維護，詳細差異與取捨見 [`FORK.md`](FORK.md)。
