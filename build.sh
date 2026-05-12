#!/bin/bash

# Cross-platform build script for 3x-ui
# Usage: ./build.sh [arch]
# Supported architectures: amd64, arm64,386, armv7

set -e

# Colors
red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

# Default version
VERSION=${1:-"v3.0.0"}

# Supported architectures
ARCHS=("amd64" "arm64")

echo -e "${green}======================================"
echo -e "  3x-ui Build Script v${VERSION}"
echo -e "======================================${plain}"

# Download dependencies
echo -e "${yellow}Downloading Go dependencies...${plain}"
go mod download

# Create dist directory
mkdir -p dist

for ARCH in "${ARCHS[@]}"; do
    echo -e "\n${green}Building for ${ARCH}...${plain}"

    OUTPUT_NAME="x-ui"
    if [ "$ARCH" == "amd64" ]; then
        GOARCH="amd64"
    elif [ "$ARCH" == "arm64" ]; then
        GOARCH="arm64"
    fi

    # Build binary
    GOOS=linux GOARCH=${GOARCH} go build -o ${OUTPUT_NAME} -ldflags "-s -w" .

    # Create package directory
    PKG_DIR="dist/x-ui-${ARCH}"
    mkdir -p ${PKG_DIR}/bin

    # Copy files
    cp ${OUTPUT_NAME} ${PKG_DIR}/x-ui
    cp x-ui.sh ${PKG_DIR}/x-ui.sh
    cp x-ui.service.debian ${PKG_DIR}/x-ui.service.debian
    cp x-ui.service.rhel ${PKG_DIR}/x-ui.service.rhel
    cp x-ui.service.arch ${PKG_DIR}/x-ui.service.arch
    cp x-ui.rc ${PKG_DIR}/x-ui.rc

    # Copy xray binary
    if [ -f "xray/xray" ]; then
        cp xray/xray ${PKG_DIR}/bin/xray
    fi

    # Set permissions
    chmod +x ${PKG_DIR}/x-ui
    chmod +x ${PKG_DIR}/x-ui.sh

    # Create tar.gz
    cd dist
    tar -czvf x-ui-linux-${ARCH}.tar.gz x-ui-${ARCH}
    cd ..

    # Cleanup
    rm -rf ${PKG_DIR}
    rm -f ${OUTPUT_NAME}

    echo -e "${green}Built: dist/x-ui-linux-${ARCH}.tar.gz${plain}"
done

echo -e "\n${green}======================================"
echo -e "  Build Complete!"
echo -e "======================================${plain}"
echo -e "Files created:"
ls -lh dist/*.tar.gz
