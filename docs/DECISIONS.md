# 維護決策

## 2026-10-26：額度用完時等到重置再試，控制端暫時連不上不放棄

**事件**：Codex 額度用完（`rate_limit`），重試程式 00:53（UTC）對 `01a0b950` 送出續跑並被接受；之後 01:03～01:05 的生命週期探測連續 `controller_unavailable`，到 `controller_failure_limit`（3）就 `retry stopped`，同時三個子代理 thread 因 `codex_thread_owner_unavailable` 停掉。額度恢復後沒有人再叫醒 Codex。

**根因**：`controller_unavailable` 是「沒有歸類的錯誤」的預設名稱。實際來源是判斷桌面版傳輸方式的 PowerShell 檢查（`desktop_transport_windows.go`）在機器忙碌時超過 8 秒被砍；生命週期探測自己的預算只有 5 秒，比它包住的檢查還短。`shared_app_server_enabled=false` 是官方 IPC 模式的正常狀態，與此無關。

**決定**：
- 傳輸檢查逾時放寬為 20 秒（`desktopTransportCheckTimeout`），生命週期探測預算為其加 5 秒；指令因自己的期限被砍時回報 `controller_timeout`，不再混成 `controller_unavailable`。
- 錯誤文字含「try again at …」時（`quota_wait.go`），下一次重試排在重置時間加 30 秒，30 分鐘自動恢復視窗從重置時間起算（`ThreadState.QuotaResetAt`）。解析不到時間（例如只有 429）維持原本的短間隔。
- `usage limit`、`usage_limit` 與帶 plan／billing 字樣的 `quota exceeded` 歸為 `rate_limit`，原本落到 `unknown` 只能試 3 次。
- `rate_limit` 類別的控制端失敗分三種：確定沒送出的（`codex_thread_owner_unavailable`、`codex_background_channel_unavailable`）不受 `controller_failure_limit` 限制，一律退避重試；可能已送出的（`controller_timeout`、`controller_unavailable`）上限放大為 4 倍但仍有限，避免重複開回合；其餘（例如 `start_not_acknowledged`）維持原上限。唯一的總期限是 30 分鐘恢復視窗。派送閘門遇到暫時性控制端狀態時，`rate_limit` 任務留在佇列而不是被停止；Codex 已關閉（`codex_not_running`）、需要重啟或設定衝突仍照既有約定停止並等使用者處理。
- 生命週期探測失敗改為退避（最長 5 分鐘）而非停止，超過視窗才停；探測與派送前的就緒檢查預算皆為 45 秒（傳輸檢查在共用模式會做兩次）。
- 「try again at」只有時分時，以錯誤發生時間為基準判斷是哪一天；重試失敗後若沒有新的重置時間，保留較晚的重置時間作為恢復視窗起點。`quota exceeded` 只在同時含 plan／billing／usage 字樣時才歸為 `rate_limit`。

**理由**：額度重置往往是數小時後，短間隔重試只會用掉次數；而重試已被接受時，探測失敗不代表任務結束。

**限制／延後**：子代理 owner 不在時只做等待，沒有改走父 thread（官方 IPC 對父 thread 開新回合的行為無法在不碰真 Codex 的情況下驗證）；已停止的舊紀錄需在新版安裝後由使用者手動按「重試」。

**觸發條件**：改動 `scheduleFailureLocked`、`rescheduleAwaitingWithPolicyLocked` 或生命週期探測時需保留 `quota_wait_test.go` 的行為；若 Codex 改變用量錯誤文字格式，更新 `usageLimitResetPattern`。

## 2026-10-26：面板打包移除 Zod 動態 Function

**決定**：`scripts/source/ui/vite.config.ts` 新增建置外掛，在打包時把 `zod/v4/core` 的 `doc.js`、`util.js`、`compile.js` 三處 `new Function` 換成停用版本（Zod 退回解譯式驗證），並在產出含 `new Function(` 時讓建置失敗。目的是消除 CodeQL 警示 `js/bad-code-sanitization`（`panel.html`）。來源比對失敗也會讓建置失敗，升級 Zod 時需重新確認。

**來源**：Codex 在 `codex-auto-retry-remediation-20261026` 的未提交改動，已在新 worktree 重現並確認產出的 `panel.html` 逐位元相同。

## 2026-10-25：修 CodeQL 首輪兩個警示

**決定**：`logger.go` 的 `Printf` 把訊息中的 CR／LF 轉成可見的 `\r`／`\n`，一筆紀錄永遠只占一行（`go/log-injection`）；`codex_ipc_windows.go` 的 `writeFrame` 比照 `readFrame` 以同一常數 `maxCodexIPCFrameBytes`（256 MiB）限制長度，再配置緩衝區（`go/allocation-size-overflow`）。

**理由**：兩處都是對上游程式的最小硬化，不改行為（正常訊息不含換行、IPC 框架遠小於上限）。已補 `TestLoggerKeepsEachRecordOnOneLine`。

**觸發條件**：上游改動這兩個檔案時，cherry-pick 需保留這兩處。

## 2026-10-25：補齊維護自動化，並分流上游 issue #1

**決定**：
- 新增 `.github/dependabot.yml`（github-actions、兩個 Go module、面板 npm、pip）、`codeql.yml`（Go 與 JavaScript/TypeScript，Go 在 Windows runner 建置）、`dependency-freshness.yml`（每月）與 `upstream-check.yml`（每週）。
- CI 的 Windows job 加跑 `tools\dev_check.ps1`，讓維護工具測試與連結檢查與本機 gate 同一組。
- 上游 issue #1（Wails 桌面客戶端的 ADR 規格）：**略過**。發文者在同一 issue 留言說明發錯 repo、屬於其自己 fork 的路線圖，且已關閉；內容是他人的產品規劃，不是缺陷回報。`reviewed_issue_through` 升為 1。
- 重寫歷史：2026-10-24 的 `Initial commit` 把本 repo 歷史壓成單一根提交，與上游不再有共同祖先。`reviewed_through` 水位比對的是 upstream remote 的 SHA，不受影響；但上游變更只能 cherry-pick 或最小重做，不能直接 merge。

**理由**：稽核缺口 `dependabot`、`codeql`、`freshness`、`upstream_check` 都是缺檔；上游檢查器若不推進 issue 水位，嚴格模式會對已分流的項目永遠紅燈。

**觸發條件**：上游 `main` 有新 commit、PR 或 issue 時，由週檢查通知後逐筆審查。

## 2026-10-02：建立 Windows-first 維護型 fork

**決定**：fork `sybxxx/codex-auto-retry`，保留 MIT License 與完整歷史。本線預設分支用 `main`。本線聚焦繁中文件、Windows 開發 gate、Windows 原生驗收，以及逐筆審查的上游追蹤。

**理由**：`codex-auto-retry` 是一套為 Windows Codex 打造的原地自動續跑工具。本 fork 補足 Windows 11 原生開發／驗收骨架、繁體中文維護入口，以及可審計的上游追蹤機制。

**限制**：
- 不把 fork 包裝成原創專案，不移除原作者 sybxxx (TQY Local Tools) 與官方連結。
- 不修改核心 Go 二進位檔架構或破壞官方 IPC 命名管道安全設計（介面字串在地化不在此限，見 2026-10-03 決定）。
- 維護 gate 不預設安裝重型套件，僅需 Python 測試與標準 PowerShell。
- 上游更新必須逐筆審查。

## 2026-10-02：繁中與英文雙語架構，移除簡體中文與非 Windows 程式碼

**決定**：公開說明僅維護繁體中文主檔 `README.md` 與英文鏡像 `README.en.md`，移除簡體中文檔案。同時移除非 Windows 平台的 stub 程式碼（如 `*_nonwindows.go`），專注純 Windows 11 原生架構。

**理由**：維護者首選繁體中文與國際英文雙語維護，且本 fork 定位為純 Windows 原生維護線，排除非 Windows 程式碼與說明可減少維護面雜訊。

## 2026-10-05：設定視窗顯示任務標題，取消與中斷的任務保留給使用者決定

**決定**：設定視窗從 Codex 的 `session_index.jsonl` 讀任務標題顯示在佇列；取消等待中的重試改為「已取消」保留 24 小時；新增「重新偵測中斷任務」，把最近 24 小時最後一輪可重試失敗、之後沒再繼續的任務列為「偵測到中斷」。兩者都只能由使用者按重新開始接回。

**理由**：只顯示 ID 認不出是哪個任務；取消即刪除、又因事件已處理而不會再被掃到，使用者沒有補救方法。

**限制**：
- 標題是 Codex 依對話產生的摘要，屬於使用者內容：只在本機設定視窗讀取顯示，不寫入狀態檔或日誌，也不放進 MCP 回傳（那會進到模型的對話內容）。
- 重新偵測不處理目標模式進行中或被暫停的任務，略過已封存的任務、超過 64 MB 的會話檔與超過 24 小時的失敗；一次最多讀 512 MB。已達上限而停止、且超過一小時不再顯示的任務，也可能被重新列為「偵測到中斷」，仍要使用者按重新開始。
- 系統匣的「已停止」計數不含已取消與偵測到中斷的項目。

## 2026-10-04：版本號改為本 fork 的語意化版本線 1.0.0

**決定**：外掛、背景服務與面板版本改為 `1.0.0`，之後依 semver 遞增：介面或功能新增升 minor、只修錯升 patch、使用者需要手動處理的不相容變更升 major。上游對應版本寫在 CHANGELOG 與 release 說明；`0.7.12-fork.N` 的 release 與 tag 已刪除。

**理由**：`-fork.N` 尾碼不好讀；沿用上游 0.7.x 或 0.8.0 又會和上游未來的版本撞號。

**限制**：同步上游時版本號不跟著上游走，`scripts/build-release.ps1` 仍要求 `model.go` 的 `appVersion` 與外掛基礎版本一致；改版本要一起改 `.codex-plugin/plugin.json`、`scripts/source/model.go`、`scripts/source/ui/package.json`（含 lock）與 `panel.ts` 裡的兩處。

## 2026-10-04：整套繁中／英文雙語

**決定**：有語言設定可讀的介面（設定視窗、啟動管理員、面板、系統匣、面板回覆訊息）依 `ui-language.json` 切換；沒有機會先讀設定的地方（安裝程式、執行檔選單、`.cmd`、命令列進度、錯誤訊息、記憶體警示、關閉 Codex 提示、MCP 工具說明）一律「中文 / English」並列。

**理由**：安裝當下還沒有語言設定，並列才能保證兩種使用者都看得懂；錯誤訊息保留原英文句子，既有測試與日後搜尋英文錯誤都不受影響。

**限制**：
- 含中文的 PowerShell 腳本必須有 UTF-8 BOM，否則 Windows PowerShell 5.1 讀成亂碼；契約測試會檢查。
- 背景服務寫進日誌的訊息維持英文（給維護者看）。
- 新增使用者看得到的訊息時要同時寫中英文，契約測試會擋下只有單一語言的 `throw`／`Write-Step`。

## 2026-10-03：產品介面與入口檔名改為繁體中文

**決定**：系統匣、設定視窗、內嵌面板、MCP 工具說明、安裝提示與 `release/windows/` 入口檔名全部改為臺灣繁體中文（`安裝.cmd`、`解除安裝.cmd`、`啟動管理員.cmd`、`安全啟動Codex.vbs`、`README-安裝說明.txt`）；預設後備重試文字由「继续」改為「繼續」。轉換由 `tools/convert_zh_hant.py`（OpenCC `s2twp` 加用語對照表）完成，`test_product_strings_are_traditional_chinese` 防止回歸。

**理由**：維護者要求只留繁體中文與英文。逐字手改無法跟上上游，所以用可重跑的工具；同步上游後重跑即可。

**限制**：
- 既有使用者設定檔裡的後備重試文字不會被改寫，只有新安裝採用新預設。
- 測試裡「UTF-8 被當成 GBK」的亂碼字串必須原樣保留，工具依標記跳過。
- `assets/` 截圖由 `tools/capture_screenshots.ps1` 重拍（2026-10-03）；啟動管理員也已加上繁中介面並重拍。

## 2026-10-03：提供單檔安裝程式，不做免安裝版

**決定**：`scripts/build-release.ps1` 除了壓縮檔，另外產生單一執行檔 `Codex-Auto-Retry-<版本>-windows-x64.exe`（2026-10-03 起不帶 `-setup`，直接雙擊顯示操作選單）。它是 `scripts/installer/` 的小程式後面直接接上同一份壓縮檔；執行時把壓縮檔解到暫存資料夾，再呼叫其中的 `deploy.ps1`。

**理由**：本工具必須註冊開機啟動、Codex 外掛與背景服務，沒有「免安裝直接執行」的形態；能省的是解壓縮與找入口檔這一步。安裝邏輯仍只有 `deploy.ps1` 一份，完整性檢查與失敗回復不會出現第二套實作。

**限制**：未做程式碼簽章，SmartScreen 會提示未知發行者；自解壓執行檔也較容易被防毒軟體誤判，所以壓縮檔照常提供。

## 2026-10-02：上游檢查涵蓋 Commit、PR 與 Issue 三面向

**決定**：`check_upstream_updates.py` 以 `--state all` 收集上游 PR 與 Issue，並追蹤 Commit SHA。`gh` 失敗時 fail closed（exit 2）。

**理由**：未合併即關閉的 PR 與待處理的 Issue 同樣可能揭露重要缺陷或需求。排程報告必須確保「未檢查」與「沒有新變更」截然分明。

## 2026-10-02：日常直接推 main

**決定**：日常維護修改在本機跑 `tools\dev_check.ps1` 後直接推 `origin/main`。Dependabot 與外部貢獻仍走 PR，合併前讀 diff。

**理由**：對齊 SanHsien 體系其他維護 fork 的治理規範。
