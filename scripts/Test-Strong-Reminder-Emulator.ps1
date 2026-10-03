param(
    [string]$SshKey = 'C:\Users\<you>\.ssh\server-key.pem',
    [string]$SshHost = 'ubuntu@203.0.113.10',
    [string]$Email = 'owner@example.com',
    [string]$SdkRoot = 'D:\wearing\.android-sdk',
    [ValidateSet('foreground', 'background')]
    [string]$Mode = 'foreground',
    [int]$RemainingPercent = 35,
    [int]$ResetHours = 24,
    [string]$PlanLabel = 'Pro'
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$adb = Join-Path $SdkRoot 'platform-tools\adb.exe'
$loginScript = Join-Path $PSScriptRoot 'Emulator-AIHub-Login.ps1'
$sqlPath = Join-Path $repoRoot '.runtime\insert-strong-reminder.sql'

$serial = (& $adb devices | Select-String '^emulator-\d+\s+device$' | Select-Object -First 1).Line.Split()[0]
if (-not $serial) { throw 'No emulator connected. Run Start-AIHub-Emulator.ps1 first.' }

$id = "ntf_emulator_$([guid]::NewGuid().ToString('N').Substring(0,12))"

Write-Host 'Releasing occupied mobile slot if needed...' -ForegroundColor Cyan
& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'Test-Cloud-Interconnect.ps1') -ReleaseMobileSlot -SkipDesktopLogin | Out-Null

Write-Host 'Ensuring emulator session...' -ForegroundColor Cyan
& powershell -NoProfile -ExecutionPolicy Bypass -File $loginScript -DeviceName 'EmulatorPhone' | Out-Host

$pyScript = Join-Path $PSScriptRoot 'write-strong-reminder-sql.py'
python $pyScript $id $Email $RemainingPercent $ResetHours $PlanLabel $sqlPath | Out-Host

Write-Host "Inserting strong reminder $id on server..." -ForegroundColor Cyan
scp -i $SshKey -o StrictHostKeyChecking=no $sqlPath "${SshHost}:/tmp/insert-strong-reminder.sql" | Out-Null
ssh -i $SshKey -o StrictHostKeyChecking=no $SshHost "sudo -u postgres psql -d aihub -v ON_ERROR_STOP=1 -f /tmp/insert-strong-reminder.sql" | Out-Host
Write-Host "Inserted notification: $id" -ForegroundColor Green

if ($Mode -eq 'background') {
    Write-Host 'Sending app to background for heads-up notification...' -ForegroundColor Yellow
    & $adb -s $serial shell input keyevent 3 | Out-Null
    Start-Sleep -Seconds 2
    Write-Host 'Waiting up to 35s for background poll...'
    Start-Sleep -Seconds 35
    & $adb -s $serial shell cmd statusbar expand-notifications | Out-Null
    Start-Sleep -Seconds 1
    $shot = Join-Path $repoRoot '.runtime/emulator-strong-reminder-bg.png'
} else {
    Write-Host 'Relaunching app for foreground dialog...' -ForegroundColor Yellow
    & $adb -s $serial shell am force-stop dev.aihub.aihub_mobile | Out-Null
    Start-Sleep -Seconds 1
    & $adb -s $serial shell monkey -p dev.aihub.aihub_mobile -c android.intent.category.LAUNCHER 1 | Out-Null
    Start-Sleep -Seconds 12
    $shot = Join-Path $repoRoot '.runtime/emulator-strong-reminder-fg.png'
}

Start-Process -FilePath $adb -ArgumentList @('-s', $serial, 'exec-out', 'screencap', '-p') -RedirectStandardOutput $shot -NoNewWindow -Wait | Out-Null
Write-Host "SCREENSHOT=$shot"
Write-Host "NOTIFICATION_ID=$id"
