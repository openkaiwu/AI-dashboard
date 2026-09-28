param(
    [string]$ServerIP = '203.0.113.10',
    [string]$KeyPath = (Join-Path $HOME '.ssh/aihub.pem'),
    [string]$SshUser = 'ubuntu',
    [int]$LocalPort = 18080,
    [string]$OutPath = "$env:TEMP\aihub-mobile.apk"
)

$ErrorActionPreference = 'Stop'
Write-Host "Starting SSH tunnel localhost:${LocalPort} -> server:80 ..."
$tunnel = Start-Process -FilePath 'ssh.exe' -ArgumentList @(
    '-i', $KeyPath, '-o', 'BatchMode=yes', '-N',
    '-L', "${LocalPort}:127.0.0.1:80",
    "${SshUser}@${ServerIP}"
) -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 2
try {
    curl.exe --noproxy "*" -f -L "http://127.0.0.1:${LocalPort}/downloads/aihub-mobile.apk" -o $OutPath
    Write-Host "Downloaded: $OutPath ($((Get-Item $OutPath).Length) bytes)"
} finally {
    Stop-Process -Id $tunnel.Id -Force -ErrorAction SilentlyContinue
}
