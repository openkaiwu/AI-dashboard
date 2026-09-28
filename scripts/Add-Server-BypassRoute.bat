@echo off
:: Right-click -> Run as administrator, or approve UAC when prompted.
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo Requesting administrator privileges...
    powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

set SERVER=203.0.113.10
set GW=192.0.2.1

route delete %SERVER% >nul 2>&1
route -p add %SERVER% mask 255.255.255.255 %GW% metric 1
if %errorLevel% neq 0 (
    echo Failed to add route.
    pause
    exit /b 1
)

echo.
echo Route added: %SERVER% -^> %GW%
route print %SERVER%
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0Verify-Server-Ports.ps1"
echo.
pause
