# Release build script for Windows
# Creates binaries for all supported platforms

$VERSION = "1.0.0"
$COMMIT = git rev-parse --short HEAD
$BUILD_DATE = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

$LD_FLAGS = "-X 'github.com/mykhaliev/agent-benchmark/version.Version=$VERSION' " +
            "-X 'github.com/mykhaliev/agent-benchmark/version.Commit=$COMMIT' " +
            "-X 'github.com/mykhaliev/agent-benchmark/version.BuildDate=$BUILD_DATE'"

# Target platforms
$PLATFORMS = @(
    @{ OS = "windows"; ARCH = "amd64" },
    @{ OS = "windows"; ARCH = "arm64" },
    @{ OS = "linux"; ARCH = "amd64" },
    @{ OS = "linux"; ARCH = "arm64" },
    @{ OS = "darwin"; ARCH = "amd64" },
    @{ OS = "darwin"; ARCH = "arm64" }
)

foreach ($platform in $PLATFORMS) {
    $os = $platform.OS
    $arch = $platform.ARCH
    
    $output = "agent-benchmark-$VERSION-$os-$arch"
    if ($os -eq "windows") {
        $output = "$output.exe"
    }
    
    Write-Host "Building $output..."
    
    $env:GOOS = $os
    $env:GOARCH = $arch
    $env:CGO_ENABLED = "0"
    
    go build -ldflags $LD_FLAGS -o $output
    
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to build $output"
        exit 1
    }
}

# Clean up environment variables
Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue

Write-Host "All builds completed."
