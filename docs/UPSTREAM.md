# 上游維護

## Remote

- Fork：`origin` → `https://github.com/SanHsien/codex-auto-retry.git`（預設分支 `main`）
- 原作者：`upstream` → `https://github.com/sybxxx/codex-auto-retry.git`（預設分支 `main`）
- 追蹤分支：`main`

## 檢查新提交

```powershell
git fetch upstream main
python tools\check_upstream_updates.py --strict
```

工具以 `tools/upstream_baseline.json` 的 `reviewed_through` 為起點，列出所有未審查提交、PR 與 Issues。
有新變更或檢查失敗時，`--strict` 回傳非零；排程 workflow 也會因此明確亮紅燈提醒。

CI 沒有 `upstream` remote，所以 baseline 的 `repo` 寫完整 clone URL，不要寫遠端短名。

## 審查清冊

每次只做一次批次審查：

1. 讀 commit 主旨與變更檔案（open PR 必須讀 diff，禁止只憑標題結案）。
2. 判斷是否與繁中 README、Windows gate、發佈閘門或測試衝突。
3. 可直接同步的提交用 merge；只需要部分修正時 cherry-pick 或最小重做。
4. 跑 `pwsh -NoProfile -File tools\dev_check.ps1`。
5. 在 `docs/DECISIONS.md` 記錄採用／略過理由。
6. 驗證完成後才把 baseline 推進到已審查的完整 40 字元 SHA 與更新 PR/Issue 水位。

Baseline 代表「已審查」，不代表「全部已合併」。

## 2026-10-02：fork 起點

本 fork 自上游 `main` `867da682d1863c4d4eb142aab3078aca4df8e0f3`
（`docs(release): publish release notes and changelog for v0.7.12`）建立。
此 SHA 設為第一個 `reviewed_through`（短 SHA 為 `867da68`）。
之後的上游 commit 才需要進入審查清冊。
