param(
    [Parameter(Mandatory=$true)][string]$ServerUrl,
    [string]$ProfileName='公网 AI Hub',
    [string]$Distribution='Ubuntu-22.04'
)
$ErrorActionPreference='Stop'
$uri=[Uri]$ServerUrl
if (!$uri.IsAbsoluteUri -or $uri.Scheme -ne 'https' -or $uri.UserInfo -or $uri.Query -or $uri.Fragment -or $uri.AbsolutePath -ne '/') {
    throw 'Android release requires an HTTPS server root URL without credentials, query or fragment.'
}
$root=Split-Path -Parent $PSScriptRoot
$runtime=Join-Path $root '.runtime'
New-Item -ItemType Directory -Force -Path $runtime | Out-Null
$defines=@{AIHUB_DEFAULT_SERVER=$uri.GetLeftPart([UriPartial]::Authority);AIHUB_DEFAULT_PROFILE_NAME=$ProfileName} | ConvertTo-Json
[IO.File]::WriteAllText((Join-Path $runtime 'android-release-defines.json'),$defines,[Text.UTF8Encoding]::new($false))
$linuxRoot='/mnt/'+$root.Substring(0,1).ToLowerInvariant()+'/'+($root.Substring(3) -replace '\\','/')
$script=@'
set -euo pipefail
export PATH="/opt/aihub-tools/flutter/bin:/usr/lib/jvm/java-17-openjdk-amd64/bin:$PATH"
export JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64
export ANDROID_HOME="__ROOT__/.tools/android-sdk"
export ANDROID_SDK_ROOT="$ANDROID_HOME"
cd "__ROOT__/apps/mobile"
dart format lib/config.dart
flutter build apk --release --no-pub -t lib/main.dart --dart-define-from-file="__ROOT__/.runtime/android-release-defines.json"
"$ANDROID_HOME/build-tools/36.0.0/apksigner" verify --print-certs build/app/outputs/flutter-apk/app-release.apk
"$ANDROID_HOME/build-tools/36.0.0/aapt2" dump badging build/app/outputs/flutter-apk/app-release.apk | sed -n '1,4p'
'@
[IO.File]::WriteAllText((Join-Path $runtime 'android-release.sh'),$script.Replace('__ROOT__',$linuxRoot).Replace("`r`n","`n"))
wsl -d $Distribution -- bash "$linuxRoot/.runtime/android-release.sh"
if($LASTEXITCODE -ne 0){throw 'Android release build or signature verification failed'}
