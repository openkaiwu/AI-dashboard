param(
    [string]$StartUrl = 'https://hub.example.com',
    [string]$SdkRoot = (Join-Path (Split-Path -Parent $PSScriptRoot) '.tools/android-sdk')
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$project = Join-Path $root 'apps/android-web'
$javaHome = $env:JAVA_HOME

function Ensure-Java {
    if ($javaHome -and (Test-Path (Join-Path $javaHome 'bin/java.exe'))) { return }
    $roots = @(
        'C:\Program Files\Microsoft\jdk-*',
        'C:\Program Files\Eclipse Adoptium\jdk-17*'
    )
    foreach ($pattern in $roots) {
        $found = Get-ChildItem -Path $pattern -Directory -ErrorAction SilentlyContinue |
            Sort-Object Name -Descending |
            Select-Object -First 1
        if ($found -and (Test-Path (Join-Path $found.FullName 'bin/java.exe'))) {
            $script:javaHome = $found.FullName
            return
        }
    }
    Write-Host 'Installing Microsoft OpenJDK 17...'
    winget install --id Microsoft.OpenJDK.17 --accept-package-agreements --accept-source-agreements
    foreach ($pattern in $roots) {
        $found = Get-ChildItem -Path $pattern -Directory -ErrorAction SilentlyContinue |
            Sort-Object Name -Descending |
            Select-Object -First 1
        if ($found -and (Test-Path (Join-Path $found.FullName 'bin/java.exe'))) {
            $script:javaHome = $found.FullName
            return
        }
    }
    throw 'Java 17 not found after install'
}

function Ensure-AndroidSdk {
    $cmdline = Join-Path $SdkRoot 'cmdline-tools/latest/bin/sdkmanager.bat'
    if (!(Test-Path $cmdline)) {
        New-Item -ItemType Directory -Force -Path (Join-Path $SdkRoot 'cmdline-tools') | Out-Null
        $zip = Join-Path $env:TEMP 'android-cmdline-tools.zip'
        Invoke-WebRequest -Uri 'https://dl.google.com/android/repository/commandlinetools-win-11076708_latest.zip' -OutFile $zip
        Expand-Archive -Path $zip -DestinationPath (Join-Path $SdkRoot 'cmdline-tools') -Force
        Rename-Item (Join-Path $SdkRoot 'cmdline-tools/cmdline-tools') (Join-Path $SdkRoot 'cmdline-tools/latest')
        Remove-Item $zip -Force
    }
    $env:ANDROID_HOME = $SdkRoot
    $env:ANDROID_SDK_ROOT = $SdkRoot
    $packages = @('platform-tools', 'platforms;android-35', 'build-tools;35.0.0')
    foreach ($pkg in $packages) {
        1..20 | ForEach-Object { 'y' } | & $cmdline "--sdk_root=$SdkRoot" $pkg
        if ($LASTEXITCODE -ne 0) { throw "sdkmanager failed for $pkg" }
    }
}

Ensure-Java
$env:JAVA_HOME = $javaHome
$env:Path = "$javaHome\bin;" + $env:Path
Ensure-AndroidSdk
$env:ANDROID_HOME = $SdkRoot
$env:ANDROID_SDK_ROOT = $SdkRoot
$localProps = Join-Path $project 'local.properties'
"sdk.dir=$($SdkRoot.Replace('\','/'))`nsdk.dir.windows=$SdkRoot" | Set-Content -Path $localProps -Encoding ASCII
Push-Location $project
try {
    & .\gradlew.bat assembleRelease -PaihubStartUrl=$StartUrl --no-daemon
    if ($LASTEXITCODE -ne 0) { throw 'gradle build failed' }
} finally { Pop-Location }

$apk = Join-Path $project 'app/build/outputs/apk/release/app-release.apk'
if (!(Test-Path -LiteralPath $apk)) { throw "APK not found: $apk" }
$outDir = Join-Path $root 'artifacts/android-web'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$target = Join-Path $outDir 'aihub-web-shell.apk'
Copy-Item -LiteralPath $apk -Destination $target -Force
Write-Host "Built $target"
