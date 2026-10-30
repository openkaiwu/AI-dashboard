$ErrorActionPreference = 'Stop'
$compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
if (-not (Test-Path -LiteralPath $compiler)) { $compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework\v4.0.30319\csc.exe' }
$testOutput = Join-Path $PSScriptRoot '..\build\windows-tests'
New-Item -ItemType Directory -Path $testOutput -Force | Out-Null
$testExe = Join-Path $testOutput 'TimeZoneSwitch.Tests.exe'
$sources = @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'src') -Filter '*.cs' | ForEach-Object FullName)
$sources += Join-Path $PSScriptRoot 'tests\Tests.cs'
& $compiler /nologo /target:exe /platform:anycpu /utf8output /codepage:65001 /langversion:5 /main:TimeZoneSwitch.Tests "/out:$testExe" "/win32manifest:$(Join-Path $PSScriptRoot 'app.manifest')" /reference:System.dll /reference:System.Core.dll /reference:System.Drawing.dll /reference:System.Windows.Forms.dll /reference:System.ServiceProcess.dll /reference:System.Web.Extensions.dll $sources
if ($LASTEXITCODE -ne 0) { throw '测试编译失败。' }
& $testExe
if ($LASTEXITCODE -ne 0) { throw 'Windows 检查失败。' }
