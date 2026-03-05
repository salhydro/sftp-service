#!/usr/bin/env pwsh
# Test script: Send an order file via SFTP to the service
#
# Usage:
#   .\test-order.ps1                           # Send to production (futur.salhydro.fi)
#   .\test-order.ps1 -Host localhost -Port 2222 # Send to local dev
#   .\test-order.ps1 -SampleFile .\samples\5229-2026_03_05-13_56_09.113.done

param(
    [string]$SftpHost = "futur.salhydro.fi",
    [int]$Port = 22,
    [string]$Username = "customer_5229",
    [string]$Password = "klsakldaklskldklasjd",
    [string]$SampleFile = ""
)

$ErrorActionPreference = "Stop"

# Pick a sample file if not specified
if (-not $SampleFile) {
    $SampleFile = Get-ChildItem -Path ".\samples\*.done" | Select-Object -First 1 -ExpandProperty FullName
    if (-not $SampleFile) {
        Write-Error "No sample files found in .\samples\"
        exit 1
    }
}

$FileName = [System.IO.Path]::GetFileName($SampleFile)
Write-Host "SFTP Order Upload Test" -ForegroundColor Magenta
Write-Host ("=" * 40) -ForegroundColor Magenta
Write-Host "Host:     $SftpHost`:$Port"
Write-Host "User:     $Username"
Write-Host "File:     $FileName"
Write-Host "Size:     $((Get-Item $SampleFile).Length) bytes"
Write-Host ""

# Check if sftp command is available
if (-not (Get-Command sftp -ErrorAction SilentlyContinue)) {
    Write-Host "sftp command not found. Trying with ssh (OpenSSH)..." -ForegroundColor Yellow
}

# Create a batch file for sftp commands
$BatchFile = [System.IO.Path]::GetTempFileName()
@"
cd /in
put $SampleFile $FileName
ls
bye
"@ | Set-Content -Path $BatchFile -Encoding UTF8

Write-Host "Connecting to $SftpHost`:$Port as $Username..." -ForegroundColor Green
Write-Host "Password will need to be entered manually." -ForegroundColor Yellow
Write-Host ""

try {
    # Use sftp with batch mode
    # Note: sftp doesn't support password via command line for security reasons
    # Use sshpass on Linux or enter password manually
    sftp -P $Port -o StrictHostKeyChecking=no -b $BatchFile "${Username}@${SftpHost}"
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host ""
        Write-Host "Order uploaded successfully!" -ForegroundColor Green
    } else {
        Write-Host ""
        Write-Host "Upload failed with exit code $LASTEXITCODE" -ForegroundColor Red
    }
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
} finally {
    Remove-Item -Path $BatchFile -ErrorAction SilentlyContinue
}
