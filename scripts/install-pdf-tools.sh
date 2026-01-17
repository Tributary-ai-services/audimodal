#!/bin/bash

# scripts/install-pdf-tools.sh
# Installs PDF processing tools required for the mapreduce PDF pipeline
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Installing PDF Processing Tools${NC}"
echo -e "${BLUE}================================${NC}\n"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to check if running as root or can use sudo
can_sudo() {
    if [[ $EUID -eq 0 ]]; then
        return 0
    elif command_exists sudo; then
        sudo -n true 2>/dev/null
        return $?
    fi
    return 1
}

# Function to run command with sudo if needed
run_privileged() {
    if [[ $EUID -eq 0 ]]; then
        "$@"
    else
        sudo "$@"
    fi
}

# Check current status of tools
echo -e "${BLUE}Checking current tool status...${NC}\n"

TOOLS_STATUS=()
MISSING_TOOLS=()

check_tool() {
    local tool=$1
    local description=$2
    if command_exists "$tool"; then
        local version=$("$tool" --version 2>/dev/null | head -1 || echo "installed")
        echo -e "  ${GREEN}✓${NC} $tool - $version"
        TOOLS_STATUS+=("$tool:installed")
    else
        echo -e "  ${RED}✗${NC} $tool - NOT INSTALLED"
        TOOLS_STATUS+=("$tool:missing")
        MISSING_TOOLS+=("$tool")
    fi
}

echo -e "${YELLOW}Poppler Utils (PDF text/image extraction):${NC}"
check_tool "pdftotext" "Extract text from PDF"
check_tool "pdfinfo" "Get PDF metadata"
check_tool "pdffonts" "List fonts in PDF"
check_tool "pdfimages" "Extract images from PDF"
check_tool "pdftoppm" "Convert PDF to images"

echo -e "\n${YELLOW}OCR Tools:${NC}"
check_tool "tesseract" "OCR engine"

echo ""

if [[ ${#MISSING_TOOLS[@]} -eq 0 ]]; then
    echo -e "${GREEN}All PDF processing tools are already installed!${NC}"
    exit 0
fi

echo -e "${YELLOW}Missing tools: ${MISSING_TOOLS[*]}${NC}\n"

# Check if we can install
if ! can_sudo; then
    echo -e "${RED}Cannot install packages without sudo access.${NC}"
    echo -e "${YELLOW}Please run the following commands manually:${NC}\n"

    case $OS in
        linux)
            if [[ -f /etc/debian_version ]]; then
                echo -e "  ${BLUE}# Debian/Ubuntu:${NC}"
                echo -e "  sudo apt-get update"
                echo -e "  sudo apt-get install -y poppler-utils tesseract-ocr tesseract-ocr-eng"
            elif [[ -f /etc/redhat-release ]]; then
                echo -e "  ${BLUE}# RHEL/CentOS/Fedora:${NC}"
                echo -e "  sudo dnf install -y poppler-utils tesseract tesseract-langpack-eng"
            elif [[ -f /etc/arch-release ]]; then
                echo -e "  ${BLUE}# Arch Linux:${NC}"
                echo -e "  sudo pacman -S poppler tesseract tesseract-data-eng"
            elif [[ -f /etc/alpine-release ]]; then
                echo -e "  ${BLUE}# Alpine Linux:${NC}"
                echo -e "  sudo apk add poppler-utils tesseract-ocr tesseract-ocr-data-eng"
            else
                echo -e "  ${BLUE}# Generic Linux (check your package manager):${NC}"
                echo -e "  # Install: poppler-utils tesseract-ocr tesseract-ocr-eng"
            fi
            ;;
        darwin)
            echo -e "  ${BLUE}# macOS with Homebrew:${NC}"
            echo -e "  brew install poppler tesseract"
            ;;
        *)
            echo -e "  ${RED}Unsupported OS: $OS${NC}"
            ;;
    esac

    echo ""
    exit 1
fi

# Install based on OS
echo -e "${BLUE}Installing missing tools...${NC}\n"

case $OS in
    linux)
        if [[ -f /etc/debian_version ]]; then
            echo -e "${YELLOW}Detected Debian/Ubuntu...${NC}"
            run_privileged apt-get update
            run_privileged apt-get install -y poppler-utils tesseract-ocr tesseract-ocr-eng
        elif [[ -f /etc/redhat-release ]]; then
            echo -e "${YELLOW}Detected RHEL/CentOS/Fedora...${NC}"
            run_privileged dnf install -y poppler-utils tesseract tesseract-langpack-eng
        elif [[ -f /etc/arch-release ]]; then
            echo -e "${YELLOW}Detected Arch Linux...${NC}"
            run_privileged pacman -S --noconfirm poppler tesseract tesseract-data-eng
        elif [[ -f /etc/alpine-release ]]; then
            echo -e "${YELLOW}Detected Alpine Linux...${NC}"
            run_privileged apk add poppler-utils tesseract-ocr tesseract-ocr-data-eng
        else
            echo -e "${RED}Unsupported Linux distribution${NC}"
            echo -e "${YELLOW}Please install manually: poppler-utils tesseract-ocr tesseract-ocr-eng${NC}"
            exit 1
        fi
        ;;
    darwin)
        if command_exists brew; then
            echo -e "${YELLOW}Installing with Homebrew...${NC}"
            brew install poppler tesseract
        else
            echo -e "${RED}Homebrew not found. Please install Homebrew first:${NC}"
            echo -e "  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
            exit 1
        fi
        ;;
    *)
        echo -e "${RED}Unsupported OS: $OS${NC}"
        exit 1
        ;;
esac

# Verify installation
echo -e "\n${BLUE}Verifying installation...${NC}\n"

ALL_INSTALLED=true
for tool in pdftotext pdfinfo pdffonts pdfimages pdftoppm tesseract; do
    if command_exists "$tool"; then
        echo -e "  ${GREEN}✓${NC} $tool installed successfully"
    else
        echo -e "  ${RED}✗${NC} $tool installation failed"
        ALL_INSTALLED=false
    fi
done

echo ""

if $ALL_INSTALLED; then
    echo -e "${GREEN}All PDF processing tools installed successfully!${NC}"
else
    echo -e "${RED}Some tools failed to install. Please check the output above.${NC}"
    exit 1
fi

# Print versions
echo -e "\n${BLUE}Installed versions:${NC}"
echo -e "  pdftotext: $(pdftotext -v 2>&1 | head -1 || echo 'unknown')"
echo -e "  tesseract: $(tesseract --version 2>&1 | head -1 || echo 'unknown')"

echo -e "\n${GREEN}PDF processing tools are ready!${NC}"
