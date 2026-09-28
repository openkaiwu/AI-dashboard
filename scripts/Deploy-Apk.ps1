param(
    [string]$ServerIP = '203.0.113.10',
    [string]$SshUser = 'ubuntu',
    [string]$KeyPath = (Join-Path $HOME '.ssh/aihub.pem'),
    [string]$ApkPath = (Join-Path (Split-Path -Parent $PSScriptRoot) 'artifacts/android-native/aihub-mobile.apk')
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$nginxConfig = Join-Path $root 'deploy/nginx-aihub.conf'
$remoteScript = Join-Path $PSScriptRoot 'deploy-apk-remote.sh'

if (!(Test-Path -LiteralPath $ApkPath)) {
    throw "APK not found: $ApkPath. Run scripts/Build-Flutter-Android.ps1 first."
}
if (!(Test-Path -LiteralPath $KeyPath)) {
    throw "SSH key not found: $KeyPath"
}

Write-Host "Uploading APK and nginx config..."
scp -i $KeyPath -o BatchMode=yes -o ConnectTimeout=30 `
    $ApkPath `
    $nginxConfig `
    $remoteScript `
    "${SshUser}@${ServerIP}:/home/ubuntu/"
if ($LASTEXITCODE -ne 0) {
    throw "SCP failed (exit $LASTEXITCODE). Open Tencent Cloud TCP 22 or upload via WebShell, then rerun."
}

Write-Host "Installing download route..."
ssh -i $KeyPath -o BatchMode=yes -o ConnectTimeout=30 "${SshUser}@${ServerIP}" "bash /home/ubuntu/deploy-apk-remote.sh"
if ($LASTEXITCODE -ne 0) { throw "Remote install failed (exit $LASTEXITCODE)" }

Write-Host ""
Write-Host "Done. Download: https://$ServerIP/downloads/aihub-mobile.apk"
