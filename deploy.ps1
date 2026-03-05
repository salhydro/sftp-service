#!/usr/bin/env pwsh
# Complete build and deployment script for SFTP service
#
# Usage:
#   .\deploy.ps1                  # Full deploy: build + CDK + force restart
#   .\deploy.ps1 -SkipBuild       # CDK deploy + force restart (no Docker build)
#   .\deploy.ps1 -SkipDeploy      # Build + push only (no CDK/ECS update)
#   .\deploy.ps1 -ImageOnly       # Build + push + force ECS restart (skip CDK)

param(
    [string]$Region = "eu-north-1",
    [string]$ClusterName = "sftp-service-cluster",
    [switch]$SkipBuild = $false,
    [switch]$SkipDeploy = $false,
    [switch]$ImageOnly = $false
)

$ErrorActionPreference = "Stop"

Write-Host "FUTUR SFTP Service - Build and Deploy" -ForegroundColor Magenta
Write-Host ("=" * 50) -ForegroundColor Magenta

# Step 1: Build and push Docker image
if (-not $SkipBuild) {
    Write-Host ""
    Write-Host "[1/3] Building and pushing Docker image..." -ForegroundColor Green
    .\build-and-push.ps1 -Region $Region
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Docker build failed!"
        exit 1
    }
    Write-Host "Docker build completed successfully!" -ForegroundColor Green
}

if ($SkipDeploy) {
    Write-Host ""
    Write-Host "Skipping deploy (SkipDeploy flag set)." -ForegroundColor Yellow
    exit 0
}

# Step 2: CDK deploy (unless ImageOnly)
if (-not $ImageOnly) {
    Write-Host ""
    Write-Host "[2/3] Deploying CDK stack..." -ForegroundColor Green
    Push-Location cdk
    try {
        cdk deploy --require-approval never
        if ($LASTEXITCODE -ne 0) {
            Write-Error "CDK deployment failed!"
            exit 1
        }
        Write-Host "CDK deployment completed successfully!" -ForegroundColor Green
    }
    finally {
        Pop-Location
    }
}

# Step 3: Force ECS service to pull the latest image
Write-Host ""
Write-Host "[3/3] Forcing ECS service to deploy latest image..." -ForegroundColor Green

$ServiceArn = aws ecs list-services --cluster $ClusterName --region $Region --query "serviceArns[0]" --output text
if (-not $ServiceArn -or $ServiceArn -eq "None") {
    Write-Error "No ECS service found in cluster $ClusterName"
    exit 1
}
$ServiceName = $ServiceArn.Split("/")[-1]

aws ecs update-service `
    --cluster $ClusterName `
    --service $ServiceName `
    --force-new-deployment `
    --region $Region `
    --query "service.{Status:status,DesiredCount:desiredCount,RunningCount:runningCount}" `
    --output table

if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to force new ECS deployment!"
    exit 1
}

Write-Host ""
Write-Host "Waiting for ECS service to stabilize..." -ForegroundColor Yellow
aws ecs wait services-stable --cluster $ClusterName --services $ServiceName --region $Region
if ($LASTEXITCODE -eq 0) {
    Write-Host "ECS service is stable with new image!" -ForegroundColor Green
} else {
    Write-Host "Warning: Timed out waiting for service to stabilize. Check AWS console." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Deployment completed!" -ForegroundColor Magenta
Write-Host "SFTP endpoint: futur.salhydro.fi (port 22)" -ForegroundColor Cyan