# scripts/install.ps1
# Usage: .\install.ps1 -ServerUrl "http://192.168.1.10:8000" -Token "abc123..."
# Must be run as Administrator (installing a Windows Service requires it).

param(
    [Parameter(Mandatory=$true)][string]$ServerUrl,
    [Parameter(Mandatory=$true)][string]$Token
)

$ErrorActionPreference = "Stop"
$installDir = "C:\Program Files\khemstrix-agent"
$exePath = Join-Path $installDir "khemstrixAgent.exe"
$statePath = Join-Path $env:ProgramData "khemstrix-agent\state.json"

Write-Host "== Khemstrix EDR Agent installer ==" -ForegroundColor Cyan

# 1. Download
Write-Host "Downloading agent from $ServerUrl..."
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Invoke-WebRequest -Uri "$ServerUrl/download/agent/windows" -OutFile $exePath
Write-Host "Downloaded to $exePath" -ForegroundColor Green

# 2. Install as a Windows Service (bakes --server/--token in as permanent
#    startup arguments — see main.go for why this is safe on every restart)
Write-Host "Installing background service..."
& $exePath install --server=$ServerUrl --token=$Token
if ($LASTEXITCODE -ne 0) {
    Write-Host "Service install failed." -ForegroundColor Red
    exit 1
}

# 3. Start it
Write-Host "Starting service..."
& $exePath start
if ($LASTEXITCODE -ne 0) {
    Write-Host "Service start failed." -ForegroundColor Red
    exit 1
}

# 4. Wait for real proof of connectivity — poll the state file the running
#    service writes to, rather than trusting "OS says it's running."
Write-Host "Waiting for the agent to confirm it can reach the server..."
$maxWaitSeconds = 30
$waited = 0
$connected = $false

while ($waited -lt $maxWaitSeconds) {
    if (Test-Path $statePath) {
        $state = Get-Content $statePath | ConvertFrom-Json
        if ($state.last_success_at -and -not $state.last_error) {
            $connected = $true
            break
        }
        if ($state.last_error) {
            Write-Host "Agent reported an error: $($state.last_error)" -ForegroundColor Yellow
            # Keep waiting briefly in case it's a transient first-attempt issue
        }
    }
    Start-Sleep -Seconds 2
    $waited += 2
}

Write-Host ""
if ($connected) {
    Write-Host "SUCCESS — agent installed, running in the background, and connected to $ServerUrl" -ForegroundColor Green
    & $exePath status
} else {
    Write-Host "The agent installed and started, but hasn't confirmed a successful connection yet." -ForegroundColor Red
    Write-Host "Troubleshooting: run this to see the real error directly (not hidden behind the service):"
    Write-Host "  & `"$exePath`" --server=$ServerUrl --token=$Token" -ForegroundColor Yellow
    exit 1
}