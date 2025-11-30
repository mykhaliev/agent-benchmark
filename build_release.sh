go build -ldflags "\
    -X 'github.com/mykhaliev/agent-benchmark/version.Version=1.0.0' \
    -X 'github.com/mykhaliev/agent-benchmark/version.Commit=$(git rev-parse --short HEAD)' \
    -X 'github.com/mykhaliev/agent-benchmark/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)' \
" -o agent-benchmark