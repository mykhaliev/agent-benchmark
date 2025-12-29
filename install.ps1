# PowerShell install script for agent-benchmark
# Usage: irm https://raw.githubusercontent.com/mykhaliev/agent-benchmark/master/install.ps1 | iex

$ErrorActionPreference = "Stop"

# Configuration
$ToolName = "agent-benchmark"
$GitHubRepo = "mykhaliev/agent-benchmark"
$InstallDir = "$env:LOCALAPPDATA\Programs\$ToolName"
$UseUPX = $false

# Helper functions
function Write-Info { param($Message) Write-Host "==> " -ForegroundColor Green -NoNewline; Write-Host $Message }
function Write-Warn { param($Message) Write-Host "Warning: " -ForegroundColor Yellow -NoNewline; Write-Host $Message }
function Write-Err { param($Message) Write-Host "Error: " -ForegroundColor Red -NoNewline; Write-Host $Message; exit 1 }

# Detect architecture
function Get-Platform {
    $arch = if ([Environment]::Is64BitOperatingSystem) {
        if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64" -or $env:PROCESSOR_IDENTIFIER -match "ARM") {
            "arm64"
        } else {
            "amd64"
        }
    } else {
        Write-Err "32-bit Windows is not supported"
    }
    return "windows_$arch"
}

# Get latest release version from GitHub
function Get-LatestVersion {
    try {
        $releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$GitHubRepo/releases/latest" -ErrorAction Stop
        return $releases.tag_name
    } catch {
        try {
            $releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$GitHubRepo/releases"
            if ($releases.Count -gt 0) {
                return $releases[0].tag_name
            }
        } catch { }
        Write-Err "Failed to fetch latest version. Please check your internet connection."
    }
}

# Download and install binary
function Install-Binary {
    $platform = Get-Platform
    $version = Get-LatestVersion

    if (Test-Path "$InstallDir\$ToolName.exe") {
        Write-Info "Updating $ToolName to $version..."
    } else {
        Write-Info "Installing $ToolName $version..."
    }

    # Construct download URL
    $binaryName = "${ToolName}_${version}_${platform}"
    if ($UseUPX) {
        $binaryName = "${binaryName}_upx"
        Write-Info "Downloading UPX compressed version (smaller size)..."
    }

    $downloadUrl = "https://github.com/$GitHubRepo/releases/download/$version/$binaryName.zip"

    # Create temporary directory
    $tmpDir = New-Item -ItemType Directory -Path "$env:TEMP\$ToolName-install-$(Get-Random)" -Force

    try {
        # Download archive
        Write-Info "Downloading from $downloadUrl..."
        $zipPath = "$tmpDir\$ToolName.zip"
        Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath -UseBasicParsing

        # Extract
        Write-Info "Extracting..."
        Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force

        # Create install directory if it doesn't exist
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }

        # Find and move the executable
        $exeFile = Get-ChildItem -Path $tmpDir -Filter "*.exe" -Recurse | Select-Object -First 1
        if (-not $exeFile) {
            Write-Err "Executable not found in archive"
        }

        Copy-Item -Path $exeFile.FullName -Destination "$InstallDir\$ToolName.exe" -Force

        if ($UseUPX) {
            Write-Info "Successfully installed $ToolName $version (UPX compressed)!"
        } else {
            Write-Info "Successfully installed $ToolName $version!"
        }
    } finally {
        # Cleanup
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Add to PATH
function Add-ToPath {
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    
    if ($currentPath -split ";" | Where-Object { $_ -eq $InstallDir }) {
        return
    }

    Write-Info "Adding $InstallDir to PATH..."
    $newPath = "$currentPath;$InstallDir"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    $env:Path = "$env:Path;$InstallDir"
    Write-Warn "PATH updated. You may need to restart your terminal for changes to take effect."
}

# Main installation process
function Main {
    Write-Host ""
    Write-Host "Installing $ToolName for Windows" -ForegroundColor Cyan
    Write-Host ""

    Install-Binary
    Add-ToPath

    Write-Host ""
    Write-Info "Installation complete!"
    Write-Info "Run '$ToolName -v' to verify"
    Write-Host ""
}

Main
