param(
    [string]$ServerIP = '203.0.113.10',
    [string]$KeyPath = (Join-Path $HOME '.ssh/aihub.pem'),
    [string]$SshUser = 'ubuntu',
    [string]$OutPath = (Join-Path (Split-Path -Parent $PSScriptRoot) 'artifacts\android-native\aihub-mobile.apk')
)

$ErrorActionPreference = 'Stop'
$dir = Split-Path -Parent $OutPath
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Write-Host "Downloading via SSH (port 22)..."
scp -i $KeyPath -o BatchMode=yes "${SshUser}@${ServerIP}:/var/www/aihub-downloads/aihub-mobile.apk" $OutPath
Write-Host "Saved: $OutPath ($((Get-Item $OutPath).Length) bytes)"
