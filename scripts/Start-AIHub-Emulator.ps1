param(
    [string]$SdkRoot = 'D:\wearing\.android-sdk',
    [string]$AvdHome = 'D:\wearing\.android-avd',
    [string]$AvdName = 'OOTD_API34',
    [string]$ApkPath = (Join-Path (Split-Path -Parent $PSScriptRoot) 'artifacts/android-native/aihub-mobile.apk'),
    [string]$Package = 'dev.aihub.aihub_mobile'
)

$ErrorActionPreference = 'Stop'
$adb = Join-Path $SdkRoot 'platform-tools\adb.exe'
$emulator = Join-Path $SdkRoot 'emulator\emulator.exe'
if (!(Test-Path -LiteralPath $adb)) { throw "adb not found: $adb" }
if (!(Test-Path -LiteralPath $emulator)) { throw "emulator not found: $emulator" }
if (!(Test-Path -LiteralPath $ApkPath)) { throw "APK not found: $ApkPath" }

$env:ANDROID_SDK_ROOT = $SdkRoot
$env:ANDROID_AVD_HOME = $AvdHome
$env:Path = "$(Join-Path $SdkRoot 'platform-tools');$env:Path"

function Get-EmulatorSerial {
    $line = & $adb devices | Select-String '^emulator-\d+\s+device$' | Select-Object -First 1
    if ($line) { return ($line.Line -split '\s+')[0] }
    return $null
}

$serial = Get-EmulatorSerial
if (-not $serial) {
    Write-Host "Starting emulator $AvdName ..."
    Start-Process -FilePath $emulator -ArgumentList @('-avd', $AvdName, '-gpu', 'auto', '-no-boot-anim') -WindowStyle Normal | Out-Null
    for ($i = 0; $i -lt 60; $i++) {
        Start-Sleep -Seconds 5
        $serial = Get-EmulatorSerial
        if ($serial) { break }
    }
}
if (-not $serial) { throw 'Emulator did not connect to adb within 5 minutes.' }

Write-Host "Waiting for boot ($serial) ..."
$booted = ''
for ($i = 0; $i -lt 60; $i++) {
    $booted = (& $adb -s $serial shell getprop sys.boot_completed 2>$null).Trim()
    if ($booted -eq '1') { break }
    Start-Sleep -Seconds 5
}
if ($booted -ne '1') { throw 'Emulator boot timeout.' }

Write-Host "Installing $ApkPath ..."
& $adb -s $serial install -r $ApkPath | Out-Host
if ($LASTEXITCODE -ne 0) { throw 'APK install failed.' }

& $adb -s $serial shell pm grant $Package android.permission.POST_NOTIFICATIONS 2>$null | Out-Null
& $adb -s $serial shell cmd notification allow_listener $Package 2>$null | Out-Null

& $adb -s $serial shell am force-stop $Package | Out-Null
& $adb -s $serial shell monkey -p $Package -c android.intent.category.LAUNCHER 1 | Out-Null

$shot = Join-Path (Split-Path -Parent $PSScriptRoot) '.runtime/emulator-launch.png'
$cap = Start-Process -FilePath $adb -ArgumentList @('-s', $serial, 'exec-out', 'screencap', '-p') -RedirectStandardOutput $shot -NoNewWindow -Wait -PassThru
if ($cap.ExitCode -ne 0) { Write-Host "Screenshot capture failed." -ForegroundColor Yellow }
Write-Host "SERIAL=$serial"
Write-Host "SCREENSHOT=$shot"
Write-Host 'Emulator ready with AI Hub APK.'
