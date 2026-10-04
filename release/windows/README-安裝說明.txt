Codex Auto Retry 1.0.0 - Windows x64 安裝說明 / Installation Guide
==================================================================

（English version follows the Chinese section.）

適用環境
--------
- Windows 10 / 11 64 位元系統
- 電腦上已安裝並至少啟動過一次 Codex App
- 只寫入目前使用者的資料夾，不需要系統管理員權限
- 不需要預先安裝 Go、Node.js 或其他開發執行環境

使用單一執行檔（Codex-Auto-Retry-<版本>-windows-x64.exe）時
------------------------------
- 不需要解壓縮。雙擊後會出現選單：安裝或更新、開啟啟動管理員、緊急停用、解除安裝、取出整包檔案。
- 請保留下載的 .exe，日後要管理或解除安裝時再雙擊它，從選單選擇即可；下文提到「解壓縮目錄」裡的入口檔也都在選單裡。
- 也可以直接帶參數執行，不顯示選單：-install、-startup-manager、-safe-disable、-uninstall（加 -remove-data 連資料一併清除）、-extract <資料夾>。

安裝步驟
--------
1. 先把整個 ZIP 壓縮檔解壓縮到一個普通資料夾中（切勿直接在壓縮檔預覽視窗裡雙擊執行）。
2. 先完整結束 Codex App，再雙擊「安裝.cmd」。安裝器會自動驗證包完整性並啟動後端守護服務。
   如果 Codex 仍在執行，會彈出中文提醒；儲存工作並結束後按一下「重試」即可繼續。取消或等待超過五分鐘會安全結束，不會強制關閉 Codex。
3. 等待黑色主控台視窗顯示「安裝完成。 / Installation succeeded.」後按任意鍵關閉。
4. 安裝完成後，Windows 右下角通知區域會出現獨立的 Codex Auto Retry 系統匣圖示。
5. 檢查連線狀態與恢復通道：
   - 雙擊系統匣圖示開啟「設定」視窗檢視目前狀態：
     * 【推薦 / 新版 Codex】：若狀態顯示「Codex 已接入官方恢復通道」，說明您的 Codex 支援原生 IPC 通道。無需進行任何額外設定，直接正常開啟 Codex 即可享受自動靜默恢復！
     * 【相容 / 舊版 Codex】：若狀態顯示「Codex 未接入共用後端」，如需開啟靜默恢復，請在系統匣設定中勾選「啟用共用後端」並通過健康檢查；之後完整結束 Codex，透過解壓縮目錄下的「安全啟動Codex.vbs」（或啟動管理員中的「安全啟動 Codex」）啟動 Codex 即可。
6. 驗證是否安裝成功：
   開啟 Codex App 新建一個任務，傳送提示詞：「開啟 Codex Auto Retry 管理面板」。若能正常彈出內嵌管理面板並顯示心跳健康，即代表就緒！

版本特性說明（1.0.0）
--------------------
1.0.0 是本 fork 自己的版本線（產品邏輯對應上游 sybxxx/codex-auto-retry 0.7.12）：
- 整套繁體中文／英文雙語：設定視窗、啟動管理員、內嵌面板與系統匣可切換語言，安裝與錯誤訊息中英並列。
- 單一執行檔 Codex-Auto-Retry-<版本>-windows-x64.exe：雙擊顯示選單，不必解壓縮。
- 修復：Codex 設定裡有失效的外掛清單時，全新安裝不再中止。
上游 0.7.12 的修復全部保留（官方 IPC 恢復、登入異常恢復上限、PowerShell 5.1 安裝誤判、關閉提醒與重試按鈕等）。

啟動管理與緊急停用
------------------
- 雙擊「啟動管理員.cmd」：
  開啟獨立的圖形管理視窗（無多餘黑框），可檢視真實的開機啟動命令、supervise 監督行程、心跳、共用後端與端點狀態；支援一鍵啟閉開機自啟、啟動/停止守護服務。
- 「安全啟動Codex.vbs」：
  僅在後端身份和健康檢查通過後，將本地端點安全傳遞給目前啟動的 Codex；如果後端異常或關閉，會自動回退到官方直連模式，絕不修改 Windows 全域永久環境變數。
- 雙擊「安全停用.cmd」：
  一鍵緊急回退指令碼。如果 Codex 啟動異常，雙擊此指令碼會立即停用共用後端、清理外掛自身的啟動項與本地端點，讓 Codex 徹底回到純淨官方環境。
  （注：本指令碼僅清理 Codex Auto Retry 自身的登錄檔與行程，絕不會刪除任何聊天記錄或 API 金鑰）。

系統匣設定指南
------------
雙擊右下角系統匣圖示即可開啟設定視窗：
- 任務佇列監控：即時檢視目前處於等待重試或活躍狀態的任務，以及最近一次倒數計時。
- 雙重安全防線（防無窮迴圈與防配額耗盡）：
  * 「本次故障恢復」上限（預設 15 次，最高可調至 1000 次）：限制同一段故障期間的最大重試總次數。
  * 「連續無進展」上限（預設 5 次，最高可調至 100 次）：若重試未產出任何可見助手回覆或工具呼叫，達到上限即主動停止並標記該任務，防止無限空耗 Token。
- 等待退避策略：
  支援「固定間隔」、「等差線性遞增」以及「翻倍指數遞增」，可自訂首次等待、步長與最大等待封頂時間。產生可見回覆後，連續計數自動清零並重置等待時間。
- 介面語言：設定視窗、啟動管理員與管理面板都能一鍵切換繁體中文與英文，三者共用同一個語言設定。
- 提醒說明：系統匣中的通知開關僅控制外掛達到重試上限時的系統通知。Codex 自帶的「ChatGPT finished a turn」彈窗屬於主程式行為，如需關閉可在 Codex「設定 > 一般 > 通知 > 輪次完成通知」中選「從不」。

Codex 內嵌管理面板
------------------
在 Codex 任意任務中輸入「開啟 Codex Auto Retry 管理面板」，即可直接叫出基於 MCP 建置的原生內嵌面板：
- 可檢視目前任務佇列、倒數計時、一鍵立即重試（Retry Now）或取消重試。
- 支援全域暫停自動重試。
- 可修改後備重試文字（預設「繼續」）。在普通對話中，外掛優先執行「原地靜默續接（無新增使用者泡泡、不重發舊訊息以防重複工具副作用）」；只有當本地 Codex 不支援靜默續接時，才會傳送該後備文字。
- 目標模式（Goal Mode）原生恢復：支援 Codex 目標模式自動恢復，且嚴格遵循使用者/AI 的主動暫停狀態，暫停期間絕不擅自恢復。

解除安裝與徹底清理
--------------
1. 標準解除安裝：
   雙擊解壓縮目錄下的「解除安裝.cmd」，即可自動停止背景服務、移除開機啟動項與 Codex 外掛註冊。預設會保留您的重試設定、歷史狀態和執行日誌，以便日後重新安裝繼續生效。
2. 徹底刪除資料：
   如需連同所有執行資料與日誌一併徹底清除，請在解壓縮目錄下開啟 PowerShell 執行：
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\uninstall-release.ps1 -RemoveData

關鍵檔案與目錄說明
------------------
- 外掛原始碼目錄：%USERPROFILE%\plugins\codex-auto-retry
- 守護行程與執行狀態：%LOCALAPPDATA%\CodexAutoRetry
- 執行日誌路徑：%LOCALAPPDATA%\CodexAutoRetry\logs\daemon.log
- Codex 外掛註冊快取：%USERPROFILE%\.codex\plugins\cache
- 開機自啟登錄檔項：HKCU\Software\Microsoft\Windows\CurrentVersion\Run\CodexAutoRetry

安全與隱私說明
--------------
- 安裝程式會先對照 SHA256SUMS.txt 驗證安裝包中所有二進位與指令碼的雜湊值。
- 本工具為完全本地執行的開源工具，僅解析任務生命週期狀態碼，絕不讀取、儲存或上傳任何使用者訊息、程式碼內容、工具參數或 API 金鑰。
- 目前尚未購買昂貴的商業程式碼簽章憑證，因此安裝時 Windows SmartScreen 可能提示「未知發行者」，請認準官方 GitHub Releases 發佈的 SHA-256 雜湊值。

常見問題排查
------------
- 安裝器提示找不到 Codex：請先正常啟動一次官方 Codex App 並進入主介面，關閉後再執行「安裝.cmd」。
- 安裝後在 Codex 中看不到外掛：必須在 Codex 中「新建一個任務/會話」才能載入新外掛；已經在進行的舊會話不會自動重新讀取外掛清單。
- 提示「請重新啟動一次 Codex」：完整結束 Codex 軟體後重新開啟即可。
- 永久失效的帳號登入：若帳號被封禁或登入已徹底過期，仍需在 Codex 中重新掃碼/登入，外掛無法繞過帳號身份驗證。


==================================================================
English
==================================================================

Requirements
------------
- Windows 10 / 11, 64-bit
- Codex App installed and started at least once
- Writes only to the current user's folders; no administrator rights needed
- No Go, Node.js, or other developer runtime needed

Single executable (Codex-Auto-Retry-<version>-windows-x64.exe)
--------------------------------------------------------------
- No extraction needed. Double-click it to get a menu: install or update, open the startup manager, safe-disable, uninstall, or extract the package.
- Keep the downloaded .exe and double-click it again later to manage or uninstall; every launcher mentioned below is also in its menu.
- Flags skip the menu: -install, -startup-manager, -safe-disable, -uninstall (add -remove-data to delete all data), -extract <folder>.

Install (ZIP)
-------------
1. Extract the whole ZIP into a normal folder (do not run files from the archive preview).
2. Fully exit Codex App, then double-click 安裝.cmd. The installer verifies the package and starts the background service.
   If Codex is still running, a bilingual reminder appears; save your work, exit Codex, and choose Retry. Cancel, or waiting more than five minutes, ends safely and never force-closes Codex.
3. When the console shows "Installation succeeded.", press any key to close it.
4. A Codex Auto Retry icon appears in the notification area.
5. Check the recovery channel: double-click the tray icon to open Settings.
   * Newer Codex: "Codex is on the official recovery channel" means native IPC works; nothing else to set up.
   * Older Codex: "Codex is not on the shared backend" means silent recovery needs the shared backend. Enable it in Settings (it runs a health check), fully exit Codex, then start Codex with 安全啟動Codex.vbs or the startup manager's "Launch Codex safely".
6. In a new Codex task, send "Open Codex Auto Retry management panel". The embedded panel with a healthy heartbeat means you are ready.

Startup manager and emergency tools
-----------------------------------
- 啟動管理員.cmd: a window showing the startup command, supervisor, heartbeat, shared backend, and endpoint state; turn sign-in startup on or off and start or stop the service.
- 安全啟動Codex.vbs: passes the local endpoint to Codex only after the backend passes its identity and health checks; otherwise Codex falls back to the official direct mode. Global environment variables are never changed.
- 安全停用.cmd: emergency rollback. Disables the shared backend and removes the plugin's own startup entries and endpoint so Codex returns to the official setup. It never deletes chats or API keys.

Settings window
---------------
- Live task queue with countdowns.
- Two safety limits: per-outage recovery limit (default 15, up to 1000) and no-progress limit (default 5, up to 100).
- Fixed, linear, or exponential waits with configurable first wait, step, and maximum.
- Language toggle shared with the startup manager and the panel.
- The notification switch only covers the plugin's retry-limit alerts. Codex's own "ChatGPT finished a turn" pop-up is set in Codex under Settings > General > Notifications.

Embedded management panel
-------------------------
Send "Open Codex Auto Retry management panel" in any Codex task to view the queue and countdowns, retry or cancel, pause globally, edit the fallback retry prompt (default "繼續"), and switch the language. Normal conversations use silent in-place continuation; the fallback text is sent only when Codex cannot continue silently. Goal mode recovers natively and respects pauses.

Uninstall
---------
1. Double-click 解除安裝.cmd (or choose Uninstall in the executable's menu). Settings, state, and logs are kept.
2. To delete everything, open PowerShell in the extracted folder and run:
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\uninstall-release.ps1 -RemoveData

Files and folders
-----------------
- Plugin: %USERPROFILE%\plugins\codex-auto-retry
- Service and state: %LOCALAPPDATA%\CodexAutoRetry
- Log: %LOCALAPPDATA%\CodexAutoRetry\logs\daemon.log
- Codex plugin cache: %USERPROFILE%\.codex\plugins\cache
- Startup entry: HKCU\Software\Microsoft\Windows\CurrentVersion\Run\CodexAutoRetry

Security and privacy
--------------------
- The installer checks every file against SHA256SUMS.txt first.
- Runs locally only; reads task lifecycle markers, never messages, code, tool arguments, or API keys.
- Not code-signed, so SmartScreen may say "Unknown publisher"; compare the SHA-256 on the GitHub release page.

Troubleshooting
---------------
- "Codex CLI was not found": start Codex App once, close it, then install again.
- Plugin missing in Codex: open a new task; running sessions do not reload the plugin list.
- "Restart Codex once": fully exit Codex and open it again.
- A permanently expired sign-in still needs a fresh login in Codex; the plugin cannot bypass authentication.
