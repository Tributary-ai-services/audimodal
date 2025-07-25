#!/bin/bash

# install-go.sh - Install latest stable Go on Ubuntu (FIXED FILE SIZE CHECK)
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    armv6l) ARCH="armv6l" ;;
    *) echo -e "${RED}Unsupported architecture: $ARCH${NC}"; exit 1 ;;
esac

echo -e "${BLUE}Installing Go for linux/$ARCH${NC}"

# Function to get latest Go version
get_latest_go_version() {
    local latest_version
    
    # Method 1: Try the direct API
    latest_version=$(curl -s https://go.dev/VERSION?m=text 2>/dev/null | head -1 | grep -o 'go[0-9.]*' | head -1 2>/dev/null || echo "")
    
    # Method 2: Fallback to GitHub releases API if method 1 fails
    if [[ -z "$latest_version" ]]; then
        latest_version=$(curl -s https://api.github.com/repos/golang/go/releases/latest 2>/dev/null | grep -o '"tag_name": *"[^"]*"' | grep -o 'go[0-9.]*' | head -1 2>/dev/null || echo "")
    fi
    
    # Method 3: Final fallback to known stable version
    if [[ -z "$latest_version" || ! "$latest_version" =~ ^go[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
        latest_version="go1.21.5"
    fi
    
    echo "$latest_version"
}

# Function to check if Go is already installed
check_existing_go() {
    if command -v go >/dev/null 2>&1; then
        local current_version
        current_version=$(go version | awk '{print $3}')
        echo -e "${YELLOW}Go is already installed: $current_version${NC}"
        
        read -p "Do you want to upgrade to the latest version? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo -e "${GREEN}Keeping existing Go installation${NC}"
            exit 0
        fi
        
        # Remove existing Go installation
        echo -e "${YELLOW}Removing existing Go installation...${NC}"
        sudo rm -rf /usr/local/go
    fi
}

# Function to install Go
install_go() {
    local version=$1
    local download_url="https://go.dev/dl/${version}.linux-${ARCH}.tar.gz"
    local temp_dir="/tmp/go-install"
    
    echo -e "${YELLOW}Downloading Go $version...${NC}"
    echo -e "${BLUE}URL: $download_url${NC}"
    
    mkdir -p "$temp_dir"
    cd "$temp_dir"
    
    # Download Go with better error handling
    if ! curl -L "$download_url" -o "${version}.linux-${ARCH}.tar.gz"; then
        echo -e "${RED}Failed to download Go. Please check your internet connection.${NC}"
        exit 1
    fi
    
    # Verify download exists and has content
    if [[ ! -f "${version}.linux-${ARCH}.tar.gz" ]] || [[ ! -s "${version}.linux-${ARCH}.tar.gz" ]]; then
        echo -e "${RED}Downloaded file is missing or empty${NC}"
        exit 1
    fi
    
    # Check file size (should be > 50MB for a valid Go archive) - FIXED SIZE CHECK
    local file_size=$(stat -c%s "${version}.linux-${ARCH}.tar.gz" 2>/dev/null || echo 0)
    local file_size_mb=$((file_size / 1024 / 1024))
    echo -e "${BLUE}Downloaded file size: ${file_size_mb}MB${NC}"
    
    if [[ $file_size -lt 50000000 ]]; then  # 50MB minimum instead of 100MB
        echo -e "${RED}Downloaded file seems too small (${file_size_mb}MB)${NC}"
        echo -e "${YELLOW}This might not be a valid Go archive${NC}"
        exit 1
    fi
    
    # Verify it's a valid tar.gz file
    if ! file "${version}.linux-${ARCH}.tar.gz" | grep -q "gzip compressed"; then
        echo -e "${RED}Downloaded file is not a valid gzip archive${NC}"
        echo -e "${YELLOW}File type: $(file "${version}.linux-${ARCH}.tar.gz")${NC}"
        exit 1
    fi
    
    # Test that we can list the archive contents
    if ! tar -tzf "${version}.linux-${ARCH}.tar.gz" >/dev/null 2>&1; then
        echo -e "${RED}Archive appears to be corrupted${NC}"
        exit 1
    fi
    
    # Install Go
    echo -e "${YELLOW}Installing Go to /usr/local/go...${NC}"
    sudo tar -C /usr/local -xzf "${version}.linux-${ARCH}.tar.gz"
    
    # Cleanup
    cd - >/dev/null
    rm -rf "$temp_dir"
    
    echo -e "${GREEN}Go $version installed successfully!${NC}"
}

# Function to setup Go environment
setup_go_environment() {
    local go_path="$HOME/go"
    local profile_file=""
    
    # Determine which profile file to use
    if [[ -f "$HOME/.bash_profile" ]]; then
        profile_file="$HOME/.bash_profile"
    elif [[ -f "$HOME/.bashrc" ]]; then
        profile_file="$HOME/.bashrc"
    else
        profile_file="$HOME/.profile"
    fi
    
    echo -e "${YELLOW}Setting up Go environment in $profile_file...${NC}"
    
    # Create GOPATH directory
    mkdir -p "$go_path"/{bin,src,pkg}
    
    # Add Go to PATH if not already there
    if ! grep -q "/usr/local/go/bin" "$profile_file" 2>/dev/null; then
        cat >> "$profile_file" << 'EOF'

# Go environment
export PATH=/usr/local/go/bin:$PATH
export GOPATH=$HOME/go
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
EOF
        echo -e "${GREEN}Go environment variables added to $profile_file${NC}"
    else
        echo -e "${YELLOW}Go environment variables already exist in $profile_file${NC}"
    fi
    
    # Also add to current session
    export PATH=/usr/local/go/bin:$PATH
    export GOPATH=$HOME/go
    export GOBIN=$GOPATH/bin
    export PATH=$GOBIN:$PATH
    
    # Setup for zsh if it exists
    if [[ -f "$HOME/.zshrc" ]] && ! grep -q "/usr/local/go/bin" "$HOME/.zshrc" 2>/dev/null; then
        cat >> "$HOME/.zshrc" << 'EOF'

# Go environment
export PATH=/usr/local/go/bin:$PATH
export GOPATH=$HOME/go
export GOBIN=$GOPATH/bin
export PATH=$GOBIN:$PATH
EOF
        echo -e "${GREEN}Go environment variables added to ~/.zshrc${NC}"
    fi
}

# Function to verify installation
verify_installation() {
    echo -e "${YELLOW}Verifying Go installation...${NC}"
    
    if /usr/local/go/bin/go version >/dev/null 2>&1; then
        local installed_version
        installed_version=$(/usr/local/go/bin/go version | awk '{print $3}')
        echo -e "${GREEN}✓ Go installed successfully: $installed_version${NC}"
        
        # Test Go functionality
        echo -e "${YELLOW}Testing Go functionality...${NC}"
        local test_dir="/tmp/go-test-$$"
        mkdir -p "$test_dir"
        cd "$test_dir"
        
        # Create a simple test program
        cat > hello.go << 'EOF'
package main

import "fmt"

func main() {
    fmt.Println("Hello, Go!")
}
EOF
        
        # Set environment for test
        export PATH=/usr/local/go/bin:$PATH
        export GOPATH=$HOME/go
        
        if /usr/local/go/bin/go run hello.go 2>/dev/null | grep -q "Hello, Go!"; then
            echo -e "${GREEN}✓ Go is working correctly${NC}"
        else
            echo -e "${RED}✗ Go test failed${NC}"
        fi
        
        # Cleanup
        cd - >/dev/null
        rm -rf "$test_dir"
        
        # Show Go environment
        echo -e "\n${BLUE}Go environment:${NC}"
        /usr/local/go/bin/go env GOVERSION GOOS GOARCH GOROOT GOPATH
        
    else
        echo -e "${RED}✗ Go installation verification failed${NC}"
        exit 1
    fi
}

# Function to install useful Go tools
install_go_tools() {
    echo -e "${YELLOW}Installing useful Go development tools...${NC}"
    
    export PATH=/usr/local/go/bin:$PATH
    export GOPATH=$HOME/go
    export GOBIN=$GOPATH/bin
    
    # List of useful tools
    local tools=(
        "golang.org/x/tools/cmd/goimports@latest"
        "golang.org/x/tools/cmd/godoc@latest"
        "golang.org/x/tools/gopls@latest"
        "github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
        "github.com/air-verse/air@latest"
    )
    
    for tool in "${tools[@]}"; do
        echo -e "${YELLOW}Installing $tool...${NC}"
        if /usr/local/go/bin/go install "$tool" 2>/dev/null; then
            echo -e "${GREEN}✓ Installed $(basename $tool)${NC}"
        else
            echo -e "${YELLOW}⚠ Warning: Failed to install $tool${NC}"
        fi
    done
    
    echo -e "${GREEN}Development tools installation completed${NC}"
}

# Main installation process
main() {
    echo -e "${BLUE}Go Installation Script for Ubuntu${NC}"
    echo -e "${BLUE}===================================${NC}"
    
    # Check if running as root
    if [[ $EUID -eq 0 ]]; then
        echo -e "${RED}Please don't run this script as root${NC}"
        exit 1
    fi
    
    # Update package list
    echo -e "${YELLOW}Updating package list...${NC}"
    sudo apt update
    
    # Install required packages
    echo -e "${YELLOW}Installing required packages...${NC}"
    sudo apt install -y curl wget tar file
    
    # Check existing installation
    check_existing_go
    
    # Get latest version
    echo -e "${YELLOW}Fetching latest Go version...${NC}"
    local latest_version
    latest_version=$(get_latest_go_version)
    echo -e "${GREEN}Latest Go version: $latest_version${NC}"
    
    # Confirm before installation
    read -p "Install Go $latest_version? (Y/n): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Nn]$ ]]; then
        echo -e "${YELLOW}Installation cancelled${NC}"
        exit 0
    fi
    
    # Install Go
    install_go "$latest_version"
    
    # Setup environment
    setup_go_environment
    
    # Verify installation
    verify_installation
    
    # Ask about development tools
    read -p "Install useful Go development tools? (Y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Nn]$ ]]; then
        install_go_tools
    fi
    
    echo -e "\n${GREEN}🎉 Go installation completed successfully!${NC}"
    echo -e "${BLUE}Please restart your terminal or run:${NC}"
    echo -e "${YELLOW}source ~/.bashrc${NC}"
    echo -e "\n${BLUE}You can verify the installation by running:${NC}"
    echo -e "${YELLOW}go version${NC}"
    echo -e "${YELLOW}go env${NC}"
}

# Run main function
main "$@"
