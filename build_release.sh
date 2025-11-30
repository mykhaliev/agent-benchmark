#!/bin/sh

VERSION="1.0.0"
COMMIT=$(git rev-parse --short HEAD)
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

LD_FLAGS="-X 'github.com/mykhaliev/agent-benchmark/version.Version=$VERSION' \
-X 'github.com/mykhaliev/agent-benchmark/version.Commit=$COMMIT' \
-X 'github.com/mykhaliev/agent-benchmark/version.BuildDate=$BUILD_DATE'"

# Target platforms
PLATFORMS=("windows/amd64" "linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64")

for PLATFORM in "${PLATFORMS[@]}"; do
    OS=$(echo $PLATFORM | cut -d'/' -f1)
    ARCH=$(echo $PLATFORM | cut -d'/' -f2)

    OUTPUT="agent-benchmark-$VERSION-$OS-$ARCH"
    [ "$OS" = "windows" ] && OUTPUT="$OUTPUT.exe"

    echo "Building $OUTPUT..."
    GOOS=$OS GOARCH=$ARCH CGO_ENABLED=0 go build -ldflags "$LD_FLAGS" -o $OUTPUT
done

echo "All builds completed."
