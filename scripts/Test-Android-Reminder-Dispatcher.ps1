param(
    [string]$Adb = (Join-Path (Split-Path -Parent $PSScriptRoot) '.tools/android-sdk/platform-tools/adb.exe'),
    [string]$Serial = 'emulator-5554',
    [string[]]$Modes = @('normal', 'quiet', 'release_retry', 'join', 'auth', 'foreground', 'foreground_quiet', 'foreground_paused', 'channel_race')
)
$ErrorActionPreference = 'Stop'
if ($Serial -notmatch '^emulator-\d+$') { throw 'Use a disposable emulator, never a real phone.' }
$isEmulator = (& $Adb -s $Serial shell getprop ro.kernel.qemu).Trim()
if ($LASTEXITCODE -ne 0 -or $isEmulator -ne '1') { throw 'Emulator verification failed.' }
$root = Split-Path -Parent $PSScriptRoot
$probe = Join-Path $root '.runtime/android-reminder-probe/probe.apk'
& $Adb -s $Serial install -r $probe
if ($LASTEXITCODE -ne 0) { throw 'Probe installation failed.' }
foreach ($mode in $Modes) {
    if ($mode -notin @('normal', 'quiet', 'release_retry', 'join', 'auth', 'channel_race', 'denied', 'foreground', 'foreground_preview', 'foreground_quiet', 'foreground_paused')) { throw "Unknown mode: $mode" }
    $output = & $Adb -s $Serial shell am instrument -w -e mode $mode dev.aihub.reminderqa/qa.ReminderProbe 2>&1
    $output | Set-Content (Join-Path $root ".runtime/android-reminder-probe/$mode.txt") -Encoding utf8
    $output | Write-Output
    if ($LASTEXITCODE -ne 0 -or ($output -join "`n") -notmatch "result=PASS $mode") { throw "Reminder regression failed: $mode" }
}
