param([string]$ConfigPath = (Join-Path $PSScriptRoot 'bridge.json'), [string]$CodexPath = '')
$ErrorActionPreference = 'Stop'
$configFile = (Resolve-Path -LiteralPath $ConfigPath).Path
$cfg = Get-Content -LiteralPath $configFile -Raw | ConvertFrom-Json
if ($CodexPath) { $cfg.codex_path = $CodexPath }
if (!$cfg.codex_path -or !(Test-Path -LiteralPath $cfg.codex_path -PathType Leaf)) {
    $candidate = Get-Process -Name codex -ErrorAction SilentlyContinue | Where-Object { $_.Path } | Select-Object -First 1
    if ($candidate) { $cfg.codex_path = $candidate.Path }
    else {
        $candidate = Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'OpenAI/Codex/bin') -Filter codex.exe -Recurse -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
        if ($candidate) { $cfg.codex_path = $candidate.FullName }
        else {
            $candidate = Get-Command codex.exe -ErrorAction SilentlyContinue
            if ($candidate) { $cfg.codex_path = $candidate.Source }
        }
    }
}
if (!$cfg.codex_path -or !(Test-Path -LiteralPath $cfg.codex_path -PathType Leaf)) { throw 'Codex executable not found. Pass -CodexPath with the full codex.exe path.' }
$identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
$acl = New-Object System.Security.AccessControl.FileSecurity
$acl.SetAccessRuleProtection($true,$false)
$acl.AddAccessRule([System.Security.AccessControl.FileSystemAccessRule]::new($identity,'FullControl','Allow'))
[System.IO.File]::SetAccessControl($configFile,$acl)
$cfg | ConvertTo-Json | Set-Content -LiteralPath $configFile -Encoding UTF8
# Windows PowerShell 5 emits a BOM; write portable UTF-8 for the Go bridge.
[IO.File]::WriteAllText($configFile,($cfg | ConvertTo-Json),[Text.UTF8Encoding]::new($false))
$root = Split-Path -Parent $PSScriptRoot
$binary = Join-Path $root 'aihub-bridge-windows-amd64.exe'
if (!(Test-Path -LiteralPath $binary)) { $binary = Join-Path $root 'artifacts/aihub-m0/aihub-bridge-windows-amd64.exe' }
if (!(Test-Path -LiteralPath $binary)) { throw 'Build or extract the AI Hub runtime package first.' }
$stateFile = Join-Path (Split-Path -Parent $configFile) 'bridge-process.json'
if (Test-Path -LiteralPath $stateFile) {
    $saved = Get-Content -LiteralPath $stateFile -Raw | ConvertFrom-Json
    $existing = Get-Process -Id $saved.Id -ErrorAction SilentlyContinue
    if ($existing -and $existing.Path -eq $binary -and $existing.StartTime.ToUniversalTime().Ticks.ToString() -eq $saved.StartTicks) { Write-Host 'Codex bridge is already running.'; exit 0 }
}
$logDir = Split-Path -Parent $configFile
$process = Start-Process -FilePath $binary -ArgumentList @('--config',('"'+$configFile+'"')) -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $logDir 'bridge.log') -RedirectStandardError (Join-Path $logDir 'bridge-error.log')
@{Id=$process.Id;StartTicks=$process.StartTime.ToUniversalTime().Ticks.ToString();Binary=$binary} | ConvertTo-Json | Set-Content -LiteralPath $stateFile
Write-Host 'Codex bridge started. First upload may take 40 seconds. Check Codex connections in AI Hub.'
