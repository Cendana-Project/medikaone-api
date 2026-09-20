[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$EnvFile,

    [ValidateSet('status', 'up')]
    [string]$Action = 'status'
)

$ErrorActionPreference = 'Stop'
$migrationSettings = @{}
$savedSettings = @{}
$allowedSettings = @(
    'ENV', 'DATABASE_DSN', 'DATABASE_ADMIN_DSN', 'REDIS_CACHE_DSN',
    'SERVER_WRITE_TIMEOUT', 'DATABASE_MAX_IDLE_CONNS', 'DATABASE_MAX_OPEN_CONNS',
    'DATABASE_MAX_CONN_LIFETIME', 'LOG_LEVEL'
)

# Parse literal KEY=VALUE entries. This intentionally does not execute or expand
# PowerShell expressions, dollar signs, or backticks in passwords.
$lineNumber = 0
foreach ($migrationLine in [System.IO.File]::ReadAllLines((Resolve-Path -LiteralPath $EnvFile).Path)) {
    $lineNumber++
    $migrationLine = $migrationLine.Trim()
    if ($migrationLine.Length -eq 0 -or $migrationLine.StartsWith('#')) { continue }
    $separator = $migrationLine.IndexOf('=')
    if ($separator -le 0) { throw "Invalid environment entry at line $lineNumber." }
    $settingName = $migrationLine.Substring(0, $separator).Trim()
    $settingValue = $migrationLine.Substring($separator + 1).Trim()
    if ($allowedSettings -cnotcontains $settingName) { throw "Unsupported environment key at line $lineNumber." }
    if ($migrationSettings.ContainsKey($settingName)) { throw "Duplicate environment key at line $lineNumber." }
    if ($settingValue.Length -ge 2) {
        $first = $settingValue[0]
        $last = $settingValue[$settingValue.Length - 1]
        if (($first -eq '"' -and $last -eq '"') -or ($first -eq "'" -and $last -eq "'")) {
            $settingValue = $settingValue.Substring(1, $settingValue.Length - 2)
        }
    }
    $migrationSettings[$settingName] = $settingValue
}

foreach ($requiredSetting in @('ENV', 'DATABASE_DSN', 'DATABASE_ADMIN_DSN')) {
    if ([string]::IsNullOrWhiteSpace($migrationSettings[$requiredSetting])) {
        throw "Missing $requiredSetting in the migration environment file."
    }
}
if ($migrationSettings['ENV'] -notin @('staging', 'production')) {
    throw 'Remote migration environment must be staging or production.'
}
if ($Action -eq 'up' -and [string]::IsNullOrWhiteSpace($migrationSettings['REDIS_CACHE_DSN'])) {
    throw 'REDIS_CACHE_DSN is required for the migration maintenance guard.'
}

$repositoryPath = Split-Path -Parent $PSScriptRoot
try {
    foreach ($settingName in $migrationSettings.Keys) {
        $savedSettings[$settingName] = [Environment]::GetEnvironmentVariable($settingName, 'Process')
        [Environment]::SetEnvironmentVariable($settingName, $migrationSettings[$settingName], 'Process')
    }
    Push-Location -LiteralPath $repositoryPath
    try {
        & go run . migrate --action $Action
        if ($LASTEXITCODE -ne 0) { throw "Database migration command failed (exit code $LASTEXITCODE)." }
    } finally {
        Pop-Location
    }
} finally {
    foreach ($settingName in $savedSettings.Keys) {
        [Environment]::SetEnvironmentVariable($settingName, $savedSettings[$settingName], 'Process')
    }
}
