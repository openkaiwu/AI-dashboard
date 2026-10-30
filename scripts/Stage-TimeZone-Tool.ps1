$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$source = Join-Path $root 'apps/desktop/tools/timezone'
$destination = Join-Path $root 'apps/desktop/resources/timezone'
$compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
if (!(Test-Path -LiteralPath $compiler)) { $compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework/v4.0.30319/csc.exe' }
if (!(Test-Path -LiteralPath $compiler)) { throw 'Windows .NET Framework compiler required for the timezone tool' }
New-Item -ItemType Directory -Force -Path $destination | Out-Null
$exe = Join-Path $destination 'TimeZoneSwitch.exe'
$sources = @(Get-ChildItem -LiteralPath (Join-Path $source 'src') -Filter '*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:winexe /platform:anycpu /optimize+ /utf8output /codepage:65001 /langversion:5 "/out:$exe" "/win32icon:$(Join-Path $source 'assets/timezone.ico')" "/win32manifest:$(Join-Path $source 'app.manifest')" /reference:System.dll /reference:System.Core.dll /reference:System.Drawing.dll /reference:System.Windows.Forms.dll /reference:System.ServiceProcess.dll /reference:System.Web.Extensions.dll $sources
if ($LASTEXITCODE -ne 0) { throw 'Timezone tool compilation failed' }
[IO.File]::WriteAllText("$exe.config", '<?xml version="1.0" encoding="utf-8"?><configuration><startup useLegacyV2RuntimeActivationPolicy="true"><supportedRuntime version="v4.0" sku=".NETFramework,Version=v4.8" /></startup></configuration>', [Text.UTF8Encoding]::new($false))
Copy-Item -LiteralPath (Join-Path $source '使用说明.txt') -Destination $destination -Force
Write-Host 'Windows timezone tool staged from bundled source'
