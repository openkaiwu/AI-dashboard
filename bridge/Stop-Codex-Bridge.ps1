param([string]$ConfigPath = (Join-Path $PSScriptRoot 'bridge.json'))
$ErrorActionPreference='Stop'
$stateFile=Join-Path (Split-Path -Parent ([IO.Path]::GetFullPath($ConfigPath))) 'bridge-process.json'
if(Test-Path -LiteralPath $stateFile){
 $saved=Get-Content -LiteralPath $stateFile -Raw|ConvertFrom-Json
 $process=Get-Process -Id $saved.Id -ErrorAction SilentlyContinue
 if($process -and $process.Path -eq $saved.Binary -and $process.StartTime.ToUniversalTime().Ticks.ToString() -eq $saved.StartTicks){
  Get-CimInstance Win32_Process -Filter "ParentProcessId = $($process.Id)" | Where-Object { $_.Name -eq 'codex.exe' } | ForEach-Object { Stop-Process -Id $_.ProcessId -ErrorAction SilentlyContinue }
  Stop-Process -Id $process.Id
 }
}
Write-Host 'Codex bridge stopped. Connection can be revoked in AI Hub.'
