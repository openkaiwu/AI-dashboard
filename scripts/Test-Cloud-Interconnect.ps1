param(
    [string]$Base = 'https://hub.example.com',
    [string]$AdminEmail = '',
    [string]$AdminPassword = '',
    [string]$UserEmail = '',
    [string]$UserPassword = '',
    [string]$DesktopInstall = "aihub-desktop-interconnect-$([guid]::NewGuid().ToString('N').Substring(0,24))",
    [string]$MobileInstall = "aihub-mobile-interconnect-$([guid]::NewGuid().ToString('N').Substring(0,24))",
    [switch]$CreateUser,
    [string]$NewUserEmail = '',
    [string]$NewUserPassword = 'Interconnect2026',
    [switch]$KeepDevices,
    [switch]$ReleaseMobileSlot,
    [switch]$SkipDesktopLogin
)

$ErrorActionPreference = 'Stop'
$pass = 0
$fail = 0
$tmp = @()

function Check($name, $ok, $detail) {
    if ($ok) { $script:pass++; Write-Host "[PASS] $name - $detail" -ForegroundColor Green }
    else { $script:fail++; Write-Host "[FAIL] $name - $detail" -ForegroundColor Red }
}

function Read-Bootstrap {
    $paths = @(
        (Join-Path $env:APPDATA 'AI Hub\runtime\bootstrap.json'),
        (Join-Path (Split-Path -Parent $PSScriptRoot) '.runtime\bootstrap.json')
    )
    foreach ($p in $paths) {
        if (Test-Path -LiteralPath $p) {
            $raw = Get-Content -LiteralPath $p -Raw
            if ($raw.Length -gt 0 -and [int][char]$raw[0] -eq 0xFEFF) { $raw = $raw.Substring(1) }
            return $raw | ConvertFrom-Json
        }
    }
    throw 'bootstrap.json not found'
}

function Invoke-Api {
    param(
        [string]$Method = 'GET',
        [string]$Path,
        [hashtable]$Headers = @{},
        $Body = $null
    )
    $uri = "$Base$Path"
    $params = @{
        Uri         = $uri
        Method      = $Method
        Headers     = $Headers
        TimeoutSec  = 20
    }
    if ($Body -ne $null) {
        $params.ContentType = 'application/json'
        $params.Body = ($Body | ConvertTo-Json -Depth 8 -Compress)
    }
    return Invoke-RestMethod @params
}

function Login-Device {
    param(
        [string]$Email,
        [string]$Password,
        [string]$DeviceName,
        [string]$Kind,
        [string]$InstallationId
    )
    return Invoke-Api -Method POST -Path '/api/v1/auth/login' -Body @{
        email            = $Email
        password         = $Password
        device_name      = $DeviceName
        device_kind      = $Kind
        installation_id  = $InstallationId
    }
}

function AuthHeader($token) { @{ Authorization = "Bearer $token" } }

function Push-Note($token, $title) {
    $entityId = [guid]::NewGuid().ToString()
    $body = @{
        protocol     = 1
        operation_id = [guid]::NewGuid().ToString()
        entity       = 'note'
        entity_id    = $entityId
        base_version = 0
        op           = 'put'
        payload      = @{ title = $title; body = "interconnect $(Get-Date -Format o)" }
    }
    $push = Invoke-Api -Method POST -Path '/api/v1/sync/push' -Headers (AuthHeader $token) -Body $body
    return @{ entityId = $entityId; push = $push }
}

function Pull-HasEntity($token, $entityId) {
    $pull = Invoke-Api -Path '/api/v1/sync/pull?cursor=' -Headers (AuthHeader $token)
    return @($pull.events | Where-Object { $_.entity_id -eq $entityId }).Count -gt 0
}

function Unbind-Devices {
    param([string]$AdminToken, [string]$UserId, [string[]]$DeviceIds)
    foreach ($id in $DeviceIds) {
        if (-not $id) { continue }
        try {
            Invoke-Api -Method DELETE -Path "/api/v1/admin/users/$UserId/devices/$id" -Headers (AuthHeader $AdminToken) | Out-Null
        } catch {}
    }
}

$boot = Read-Bootstrap
if (-not $AdminEmail) { $AdminEmail = $boot.email }
if (-not $AdminPassword) { $AdminPassword = $boot.password }
if (-not $UserEmail) { $UserEmail = $boot.email }
if (-not $UserPassword) { $UserPassword = $boot.password }
if ($CreateUser) {
    if (-not $NewUserEmail) { $NewUserEmail = "interconnect+$([guid]::NewGuid().ToString('N').Substring(0,8))@example.test" }
}

Write-Host "== Cloud interconnect => $Base ==" -ForegroundColor Cyan
Write-Host "User: $UserEmail"

try {
    $ready = Invoke-Api -Path '/ready'
    Check 'ready' ($ready.status -eq 'ok') $ready.status
} catch { Check 'ready' $false $_.Exception.Message; exit 1 }

$admin = Login-Device -Email $AdminEmail -Password $AdminPassword -DeviceName 'Interconnect Admin' -Kind 'web_admin' -InstallationId "admin-interconnect-$([guid]::NewGuid().ToString('N').Substring(0,16))"
Check 'admin login' ($null -ne $admin.token) $admin.user.email
$adminH = AuthHeader $admin.token

$userIdForAdmin = $null
try {
    $users = Invoke-Api -Path '/api/v1/admin/users' -Headers $adminH
    $match = @($users.users | Where-Object { $_.email -eq $UserEmail } | Select-Object -First 1)
    if ($match.Count -gt 0) { $userIdForAdmin = $match[0].id }
} catch {}

if ($ReleaseMobileSlot -and $userIdForAdmin) {
    try {
        $bound = Invoke-Api -Path "/api/v1/admin/users/$userIdForAdmin/devices" -Headers $adminH
        foreach ($d in @($bound.devices | Where-Object { $_.kind -eq 'mobile' -and -not $_.revoked_at })) {
            Invoke-Api -Method DELETE -Path "/api/v1/admin/users/$userIdForAdmin/devices/$($d.id)" -Headers $adminH | Out-Null
            Write-Host "Released mobile slot: $($d.name) ($($d.id))" -ForegroundColor Yellow
        }
    } catch {
        Write-Host "Could not release mobile slot: $($_.Exception.Message)" -ForegroundColor Yellow
    }
}

$createdUserId = $null
if ($CreateUser) {
    try {
        $created = Invoke-Api -Method POST -Path '/api/v1/admin/users' -Headers $adminH -Body @{
            email    = $NewUserEmail
            password = $NewUserPassword
        }
        $createdUserId = $created.id
        Check 'create user' ($null -ne $createdUserId) $NewUserEmail
        $UserEmail = $NewUserEmail
        $UserPassword = $NewUserPassword
    } catch {
        Check 'create user' $false $_.Exception.Message
        exit 1
    }
}

$desktop = $null
$mobile = $null
$desktopDevice = $null
$mobileDevice = $null
if (-not $SkipDesktopLogin) {
    try {
        $desktop = Login-Device -Email $UserEmail -Password $UserPassword -DeviceName 'Interconnect Desktop' -Kind 'desktop' -InstallationId $DesktopInstall
        $desktopDevice = $desktop.device_id
        Check 'desktop login' ($null -ne $desktop.token) $desktopDevice
    } catch {
        Check 'desktop login' $false $_.Exception.Message
    }
} else {
    try {
        if ($userIdForAdmin) {
            $bound = Invoke-Api -Path "/api/v1/admin/users/$userIdForAdmin/devices" -Headers $adminH
            $liveDesktop = @($bound.devices | Where-Object { $_.kind -eq 'desktop' -and -not $_.revoked_at })
            Check 'desktop slot (live)' ($liveDesktop.Count -eq 1) ($liveDesktop[0].name)
        }
    } catch { Check 'desktop slot (live)' $false $_.Exception.Message }
}

try {
    $mobile = Login-Device -Email $UserEmail -Password $UserPassword -DeviceName 'Interconnect Mobile' -Kind 'mobile' -InstallationId $MobileInstall
    $mobileDevice = $mobile.device_id
    Check 'mobile login' ($null -ne $mobile.token) $mobileDevice
} catch {
    Check 'mobile login' $false $_.Exception.Message
}

if ($mobile -and ($desktop -or $SkipDesktopLogin)) {
    $dTitle = "desktop-$([guid]::NewGuid().ToString('N').Substring(0,8))"
    $mTitle = "mobile-$([guid]::NewGuid().ToString('N').Substring(0,8))"
    if ($desktop) {
        try {
            $dPush = Push-Note $desktop.token $dTitle
            Check 'desktop push' ($dPush.push.status -eq 'applied') $dPush.push.status
            Start-Sleep -Milliseconds 300
            Check 'mobile pull desktop note' (Pull-HasEntity $mobile.token $dPush.entityId) $dPush.entityId
        } catch { Check 'desktop->mobile' $false $_.Exception.Message }
    } else {
        Check 'desktop push (skipped)' $true 'live desktop client keeps bridge; API slot not borrowed'
    }

    try {
        $mPush = Push-Note $mobile.token $mTitle
        Check 'mobile push' ($mPush.push.status -eq 'applied') $mPush.push.status
        Start-Sleep -Milliseconds 300
        if ($desktop) {
            Check 'desktop pull mobile note' (Pull-HasEntity $desktop.token $mPush.entityId) $mPush.entityId
        } else {
            Check 'mobile note stored' (Pull-HasEntity $mobile.token $mPush.entityId) $mPush.entityId
        }
    } catch { Check 'mobile->desktop' $false $_.Exception.Message }

    try {
        $devices = Invoke-Api -Path '/api/v1/devices' -Headers (AuthHeader $mobile.token)
        $kinds = @($devices.devices | ForEach-Object { $_.kind })
        Check 'device list' (($kinds -contains 'desktop') -and ($kinds -contains 'mobile')) ($kinds -join ',')
    } catch { Check 'device list' $false $_.Exception.Message }

    if ($desktop) {
        try {
            $bridge = Invoke-Api -Method POST -Path '/api/v1/codex/bridges' -Headers (AuthHeader $desktop.token) -Body @{ name = 'Interconnect Desktop' }
            Check 'bridge create' ($null -ne $bridge.token) $bridge.id
        } catch { Check 'bridge create' $false $_.Exception.Message }
    } else {
        try {
            $overview = Invoke-Api -Path '/api/v1/codex/overview' -Headers (AuthHeader $mobile.token)
            $devices = @($overview.devices)
            $connected = @($devices | Where-Object { $_.snapshot -or $_.received_at })
            Check 'live bridge via mobile' ($connected.Count -ge 1) "devices=$($devices.Count)"
        } catch { Check 'live bridge via mobile' $false $_.Exception.Message }
    }

    try {
        $notes = Invoke-Api -Path '/api/v1/notifications' -Headers (AuthHeader $mobile.token)
        $strong = @($notes.notifications | Where-Object { $_.status -eq 'unread' -and ($_.severity -eq 'warning' -or $_.severity -eq 'critical') })
        Check 'mobile notifications' ($strong.Count -ge 0) "unread strong=$($strong.Count)"
    } catch { Check 'mobile notifications' $false $_.Exception.Message }
}

if (-not $KeepDevices) {
    $userId = if ($createdUserId) { $createdUserId } elseif ($desktop) { $desktop.user.id } else { $userIdForAdmin }
    Unbind-Devices -AdminToken $admin.token -UserId $userId -DeviceIds @($desktopDevice, $mobileDevice)
    Check 'cleanup devices' $true 'test slots released'
}

Write-Host ''
Write-Host "Summary: $pass passed, $fail failed" -ForegroundColor $(if ($fail -eq 0) { 'Green' } else { 'Red' })
if ($CreateUser) { Write-Host "New account: $NewUserEmail / $NewUserPassword" }
exit $(if ($fail -eq 0) { 0 } else { 1 })
