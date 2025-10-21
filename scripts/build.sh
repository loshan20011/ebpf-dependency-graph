#!/bin/bash
set -e

# eBPF Dependency Tracker Build Script
echo "🏗️  Building eBPF Dependency Tracker..."

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Project root directory
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

echo -e "${BLUE}📁 Project root: $PROJECT_ROOT${NC}"

# Create necessary directories
echo -e "${BLUE}📁 Creating directories...${NC}"
mkdir -p bin
mkdir -p logs

# Check dependencies
echo -e "${BLUE}🔍 Checking dependencies...${NC}"

# Check if protoc is available
if ! command -v protoc &> /dev/null; then
    echo -e "${RED}❌ protoc is not installed. Please install Protocol Buffers compiler.${NC}"
    echo "   Ubuntu/Debian: sudo apt-get install protobuf-compiler"
    echo "   macOS: brew install protobuf"
    exit 1
fi

# Check if Go is available
if ! command -v go &> /dev/null; then
    echo -e "${RED}❌ Go is not installed. Please install Go 1.23 or later.${NC}"
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
REQUIRED_VERSION="1.23"
if ! printf '%s\n%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V -C; then
    echo -e "${RED}❌ Go version $GO_VERSION is too old. Requires Go $REQUIRED_VERSION or later.${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Go version $GO_VERSION${NC}"

# Install Go dependencies
echo -e "${BLUE}📦 Installing Go dependencies...${NC}"
go mod tidy


echo -e "${BLUE}🔧 Installing Go plugins for protoc...${NC}"
# Install plugins into the current GOPATH (under sudo this is typically /root/go)
GO_BIN_DIR="$(go env GOPATH)/bin"
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
# Ensure plugin binaries are on PATH for this script execution
export PATH="${GO_BIN_DIR}:${PATH}"

# Verify plugins are available
if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo -e "${RED}❌ protoc-gen-go not found on PATH (${GO_BIN_DIR}).${NC}"
  echo "   Add to PATH: export PATH=\"${GO_BIN_DIR}:\$PATH\" and rerun, or run build without sudo."
  exit 1
fi
if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo -e "${RED}❌ protoc-gen-go-grpc not found on PATH (${GO_BIN_DIR}).${NC}"
  echo "   Add to PATH: export PATH=\"${GO_BIN_DIR}:\$PATH\" and rerun, or run build without sudo."
  exit 1
fi

# Generate protobuf files
echo -e "${BLUE}🔧 Generating protobuf files...${NC}"
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       pkg/proto/metrics.proto

echo -e "${GREEN}✅ Protobuf files generated${NC}"

# Generate eBPF bindings (bpf2go)
echo -e "${BLUE}🔧 Generating eBPF bindings...${NC}"
go generate ./internal/ebpf/...

echo -e "${GREEN}✅ eBPF bindings generated${NC}"

# Set up Python virtual environment for tests (optional)
echo -e "${BLUE}🐍 Setting up Python environment for tests...${NC}"
if command -v python3 &> /dev/null; then
    if [ ! -d ".venv" ]; then
        echo -e "${BLUE}   Creating Python virtual environment...${NC}"
        python3 -m venv .venv
    fi
    
    echo -e "${BLUE}   Installing Python test dependencies...${NC}"
    .venv/bin/pip install --quiet requests
    echo -e "${GREEN}✅ Python environment ready${NC}"
else
    echo -e "${YELLOW}⚠️  Python3 not found. Test scripts may not work.${NC}"
fi

# Build operator
echo -e "${BLUE}🔨 Building operator...${NC}"
CGO_ENABLED=1 go build -o bin/operator cmd/operator/*.go
echo -e "${GREEN}✅ Operator built successfully${NC}"

# Build agent
echo -e "${BLUE}🔨 Building agent...${NC}"
CGO_ENABLED=1 go build -o bin/agent cmd/agent/*.go
echo -e "${GREEN}✅ Agent built successfully${NC}"

# Make binaries executable
chmod +x bin/operator bin/agent

echo -e "${GREEN}🎉 Build completed successfully!${NC}"
echo
echo -e "${BLUE}📋 Next steps:${NC}"
echo "   • Run operator: sudo ./bin/operator"
echo "   • Run agent: sudo ./bin/agent"
echo "   • View logs: tail -f logs/*.log"
echo "   • Test API: curl http://localhost:8080/health"
echo
echo -e "${BLUE}📖 Documentation:${NC}"
echo "   • Installation: docs/installation.md"
echo "   • API Reference: docs/api.md"
echo "   • README: README.md"