param(
    [string]$SdkRoot = (Join-Path (Split-Path -Parent $PSScriptRoot) '.tools/android-sdk')
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$mobile = Join-Path $root 'apps/mobile'

function To-WslPath([string]$Path) {
    if ($Path -match '^([A-Za-z]):\\(.*)$') {
        return ('/mnt/{0}/{1}' -f $Matches[1].ToLower(), ($Matches[2] -replace '\\', '/'))
    }
    throw "Unsupported path: $Path"
}

if (!(Test-Path $SdkRoot)) {
    throw 'Missing Android SDK. Run scripts/Build-Android-Web.ps1 once to bootstrap SDK.'
}

$linuxRoot = To-WslPath $root
$linuxSdk = To-WslPath $SdkRoot
$linuxScript = To-WslPath (Join-Path $PSScriptRoot 'build-flutter-android.sh')

$localProps = @(
    "sdk.dir=$linuxSdk",
    "flutter.sdk=/opt/aihub-tools/flutter"
) -join "`n"
Set-Content -Path (Join-Path $mobile 'android/local.properties') -Value $localProps -Encoding ASCII -NoNewline

wsl bash $linuxScript $linuxRoot $linuxSdk
if ($LASTEXITCODE -ne 0) { throw 'Flutter APK build failed' }

$apk = Join-Path $mobile 'build/app/outputs/flutter-apk/app-release.apk'
if (!(Test-Path -LiteralPath $apk)) { throw "APK not found: $apk" }
$outDir = Join-Path $root 'artifacts/android-native'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$target = Join-Path $outDir 'aihub-mobile.apk'
Copy-Item -LiteralPath $apk -Destination $target -Force
Write-Host "Built $target"
