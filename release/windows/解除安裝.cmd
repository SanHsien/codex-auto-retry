@echo off
setlocal
chcp 65001 >nul
title Codex Auto Retry 解除安裝 / Uninstaller
echo.
echo Codex Auto Retry - 一鍵解除安裝 / one-click uninstaller
echo 既有的重試設定與狀態會保留。 / Existing retry settings and state will be preserved.
echo.
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0uninstall-release.ps1"
set "EXIT_CODE=%ERRORLEVEL%"
echo.
if not "%EXIT_CODE%"=="0" (
  echo 解除安裝失敗，請看上方的錯誤訊息。 / Uninstall failed. Review the error above.
) else (
  echo 解除安裝完成。 / Uninstall succeeded.
)
echo.
pause
exit /b %EXIT_CODE%
