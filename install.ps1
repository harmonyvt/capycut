# CapyCut Installation Script for Windows
# Supports: Windows PowerShell 5.x and PowerShell 7+
#
# Usage:
#   irm https://raw.githubusercontent.com/harmonyvt/capycut/main/install.ps1 | iex
#   
# Or download and run:
#   Invoke-WebRequest -Uri https://raw.githubusercontent.com/harmonyvt/capycut/main/install.ps1 -OutFile install.ps1
#   .\install.ps1

#Requires -Version 5.0

[CmdletBinding()]
param(
    [string]$InstallDir = "",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

# Configuration
$RepoOwner = "harmonyvt"
$RepoName = "capycut"
$BinaryName = "capycut"
$GitHubAPI = "https://api.github.com"

# Colors
function Write-Info { param([string]$Message) Write-Host "[INFO] " -ForegroundColor Cyan -NoNewline; Write-Host $Message }
function Write-Success { param([string]$Message) Write-Host "[SUCCESS] " -ForegroundColor Green -NoNewline; Write-Host $Message }
function Write-Warn { param([string]$Message) Write-Host "[WARN] " -ForegroundColor Yellow -NoNewline; Write-Host $Message }
function Write-Err { param([string]$Message) Write-Host "[ERROR] " -ForegroundColor Red -NoNewline; Write-Host $Message }

function Get-Architecture {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64" { return "amd64" }
        "Arm64" { return "arm64" }
        "X86" { return "386" }
        default { throw "Unsupported architecture: $arch" }
    }
}

function Get-LatestVersion {
    Write-Info "Fetching latest version..."
    
    try {
        $releases = Invoke-RestMethod -Uri "$GitHubAPI/repos/$RepoOwner/$RepoName/releases/latest" -Headers @{
            "Accept" = "application/vnd.github.v3+json"
            "User-Agent" = "capycut-installer"
        }
        return $releases.tag_name
    }
    catch {
        throw "Failed to get latest version: $_"
    }
}

function Get-InstallDirectory {
    param([string]$CustomDir)
    
    if ($CustomDir) {
        return $CustomDir
    }
    
    # Default installation directory
    $localAppData = [Environment]::GetFolderPath("LocalApplicationData")
    $installDir = Join-Path $localAppData "Programs\capycut"
    
    return $installDir
}

function Add-ToPath {
    param([string]$Directory)
    
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    
    if ($currentPath -split ";" -contains $Directory) {
        Write-Info "Directory already in PATH"
        return $false
    }
    
    $newPath = "$currentPath;$Directory"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    
    # Also update current session
    $env:Path = "$env:Path;$Directory"
    
    return $true
}

function Test-InPath {
    param([string]$Directory)
    
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $systemPath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $fullPath = "$currentPath;$systemPath"
    
    return ($fullPath -split ";" -contains $Directory)
}

function Install-CapyCut {
    Write-Host ""
    Write-Host "  " -NoNewline
    Write-Host "CapyCut Installer" -ForegroundColor Cyan
    Write-Host "  --------------------"
    Write-Host ""
    
    # Detect architecture
    $arch = Get-Architecture
    Write-Info "Detected architecture: windows/$arch"
    
    # Get latest version
    $version = Get-LatestVersion
    $versionNum = $version.TrimStart("v")
    Write-Info "Latest version: $version"
    
    # Construct download URL
    $archiveName = "${BinaryName}_${versionNum}_windows_${arch}.zip"
    $downloadUrl = "https://github.com/$RepoOwner/$RepoName/releases/download/$version/$archiveName"
    
    Write-Info "Downloading $archiveName..."
    
    # Create temp directory
    $tempDir = Join-Path $env:TEMP "capycut-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
    
    try {
        # Download archive
        $archivePath = Join-Path $tempDir $archiveName
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing
        
        Write-Info "Extracting..."
        
        # Extract archive
        Expand-Archive -Path $archivePath -DestinationPath $tempDir -Force
        
        # Get install directory
        $installDir = Get-InstallDirectory -CustomDir $InstallDir
        Write-Info "Installing to $installDir..."
        
        # Create install directory if it doesn't exist
        if (-not (Test-Path $installDir)) {
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        }
        
        # Find the binary
        $binaryPath = Get-ChildItem -Path $tempDir -Recurse -Filter "$BinaryName.exe" | Select-Object -First 1
        
        if (-not $binaryPath) {
            throw "Could not find $BinaryName.exe in archive"
        }
        
        # Check if capycut is already installed
        $targetPath = Join-Path $installDir "$BinaryName.exe"
        if ((Test-Path $targetPath) -and -not $Force) {
            $existingVersion = & $targetPath --version 2>$null | Select-Object -First 1
            Write-Warn "CapyCut is already installed: $existingVersion"
            Write-Info "Use -Force to overwrite or run 'capycut --update' to update"
        }
        
        # Copy binary to install directory
        Copy-Item -Path $binaryPath.FullName -Destination $targetPath -Force
        
        Write-Host ""
        Write-Success "CapyCut $version installed successfully!"
        Write-Host ""
        
        # Add to PATH
        if (-not (Test-InPath $installDir)) {
            Write-Info "Adding $installDir to PATH..."
            $pathAdded = Add-ToPath -Directory $installDir
            
            if ($pathAdded) {
                Write-Success "Added to PATH"
                Write-Host ""
                Write-Warn "NOTE: You may need to restart your terminal for PATH changes to take effect"
            }
        }
        
        Write-Host ""
        Write-Host "  Run " -NoNewline
        Write-Host "capycut --help" -ForegroundColor Cyan -NoNewline
        Write-Host " to get started"
        Write-Host "  Run " -NoNewline
        Write-Host "capycut --setup" -ForegroundColor Cyan -NoNewline
        Write-Host " to configure your LLM provider"
        Write-Host ""
        Write-Host "  To update in the future, run: " -NoNewline
        Write-Host "capycut --update" -ForegroundColor Cyan
        Write-Host ""
    }
    finally {
        # Cleanup temp directory
        if (Test-Path $tempDir) {
            Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

# Run installation
try {
    Install-CapyCut
}
catch {
    Write-Err $_.Exception.Message
    exit 1
}
