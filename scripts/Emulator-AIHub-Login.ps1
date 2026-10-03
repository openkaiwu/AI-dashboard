param(
    [string]$SdkRoot = 'D:\wearing\.android-sdk',
    [string]$Email = '',
    [string]$Password = '',
    [string]$DeviceName = 'EmulatorPhone',
    [switch]$SkipClear
)

$ErrorActionPreference = 'Stop'
$adb = Join-Path $SdkRoot 'platform-tools\adb.exe'
$serial = (& $adb devices | Select-String '^emulator-\d+\s+device$' | Select-Object -First 1).Line.Split()[0]
if (-not $serial) { throw 'No emulator connected.' }

$bootPaths = @(
    (Join-Path $env:APPDATA 'AI Hub\runtime\bootstrap.json'),
    (Join-Path (Split-Path -Parent $PSScriptRoot) '.runtime\bootstrap.json')
)
foreach ($p in $bootPaths) {
    if (Test-Path -LiteralPath $p) {
        $raw = Get-Content -LiteralPath $p -Raw
        if ($raw.Length -gt 0 -and [int][char]$raw[0] -eq 0xFEFF) { $raw = $raw.Substring(1) }
        $boot = $raw | ConvertFrom-Json
        if (-not $Email) { $Email = $boot.email }
        if (-not $Password) { $Password = $boot.password }
        break
    }
}
if (-not $Email -or -not $Password) { throw 'Missing credentials in bootstrap.json' }

function Tap([int]$x, [int]$y) { & $adb -s $serial shell input tap $x $y | Out-Null }
function TypeText([string]$text) {
    $escaped = $text.Replace(' ', '%s').Replace('@', '\@').Replace(':', '\:').Replace('/', '\/')
    & $adb -s $serial shell input text $escaped | Out-Null
}
function ClearField() {
    for ($i = 0; $i -lt 24; $i++) { & $adb -s $serial shell input keyevent 67 | Out-Null }
}

if (-not $SkipClear) {
    & $adb -s $serial shell pm clear dev.aihub.aihub_mobile | Out-Null
}
& $adb -s $serial shell pm grant dev.aihub.aihub_mobile android.permission.POST_NOTIFICATIONS 2>$null | Out-Null
& $adb -s $serial shell monkey -p dev.aihub.aihub_mobile -c android.intent.category.LAUNCHER 1 | Out-Null
Start-Sleep -Seconds 6

& $adb -s $serial shell uiautomator dump /sdcard/ui.xml | Out-Null
$ui = & $adb -s $serial shell cat /sdcard/ui.xml
if ($ui -match '进入 Codex 模式' -or $ui -match '进入 Cursor 模式') {
    Write-Host 'ALREADY_LOGGED_IN'
    exit 0
}

Tap 108 2164   # quota tab (login form)
Start-Sleep -Seconds 1
Tap 540 815    # email
TypeText $Email
Tap 540 1013   # password
TypeText $Password
Tap 540 1211   # device name
ClearField
TypeText $DeviceName
Tap 308 1410   # enter workspace
Start-Sleep -Seconds 14

$shot = Join-Path (Split-Path -Parent $PSScriptRoot) '.runtime/emulator-after-login.png'
Start-Process -FilePath $adb -ArgumentList @('-s', $serial, 'exec-out', 'screencap', '-p') -RedirectStandardOutput $shot -NoNewWindow -Wait | Out-Null
Write-Host "LOGIN_ATTEMPT_DONE screenshot=$shot"
