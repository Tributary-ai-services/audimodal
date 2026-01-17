#!/bin/bash
#
# Generate AudiModal Service Account API Key
#
# This script generates a long-lived JWT API key for service-to-service
# authentication between aether-backend and AudiModal.
#
# Usage:
#   ./scripts/generate-service-api-key.sh [--expiry-years N] [--help]
#

set -eo pipefail

# Default values
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
EXPIRY_YEARS=5

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --expiry-years)
            EXPIRY_YEARS="$2"
            shift 2
            ;;
        --help|-h)
            echo "Generate AudiModal Service Account API Key"
            echo ""
            echo "Usage:"
            echo "  $0 [options]"
            echo ""
            echo "Options:"
            echo "  --expiry-years N    Set API key expiry in years (default: 5)"
            echo "  --help, -h          Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  JWT_SECRET          JWT secret for signing tokens (required)"
            echo ""
            echo "Example:"
            echo "  JWT_SECRET=your-secret-key $0 --expiry-years 3"
            exit 0
            ;;
        *)
            echo -e "${RED}Error: Unknown option: $1${NC}"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Check if JWT_SECRET is set
if [ -z "$JWT_SECRET" ]; then
    echo -e "${RED}Error: JWT_SECRET environment variable is not set${NC}"
    echo ""
    echo "Usage:"
    echo "  JWT_SECRET=your-secret-key $0"
    echo ""
    echo "To get the JWT secret from Kubernetes:"
    echo "  kubectl get secret audimodal-secrets -n tas-secrets -o jsonpath='{.data.JWT_SECRET}' | base64 -d"
    echo ""
    exit 1
fi

echo -e "${BLUE}=== AudiModal API Key Generator ===${NC}"
echo ""
echo "Configuration:"
echo "  Expiry:     $EXPIRY_YEARS years"
echo "  User ID:    00000000-0000-0000-0000-000000000001"
echo "  Tenant ID:  00000000-0000-0000-0000-000000000001"
echo "  Scopes:     data:read,data:write,data:process,api:read,api:write"
echo ""

# Change to project root
cd "$PROJECT_ROOT"

# Build the admin CLI tool
echo -e "${BLUE}Building admin CLI tool...${NC}"
if ! go build -o /tmp/audimodal-admin cmd/admin/main.go; then
    echo -e "${RED}Error: Failed to build admin CLI tool${NC}"
    exit 1
fi
echo -e "${GREEN}Build successful${NC}"
echo ""

# Generate the API key
echo -e "${BLUE}Generating API key...${NC}"
OUTPUT=$(/tmp/audimodal-admin --jwt-secret="$JWT_SECRET" --expiry-years=$EXPIRY_YEARS)

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to generate API key${NC}"
    exit 1
fi

# Display the output
echo "$OUTPUT"
echo ""

# Extract just the API key (the line after "API Key:")
API_KEY=$(echo "$OUTPUT" | sed -n '/^API Key:$/{ n; p; }' | tr -d '[:space:]')

if [ -z "$API_KEY" ]; then
    echo -e "${RED}Error: Failed to extract API key from output${NC}"
    exit 1
fi

# Cleanup
rm -f /tmp/audimodal-admin

echo -e "${GREEN}=== Next Steps ===${NC}"
echo ""
echo "1. Store this API key in aether-secrets repository:"
echo -e "   ${YELLOW}export AUDIMODAL_API_KEY='$API_KEY'${NC}"
echo ""
echo "2. Update aether-secrets/apps/aether-be/service-keys.env:"
echo -e "   ${YELLOW}AUDIMODAL_API_KEY=$API_KEY${NC}"
echo ""
echo "3. Update Kubernetes secrets and restart services"
echo ""
echo -e "${BLUE}For detailed instructions, see:${NC}"
echo "  audimodal/docs/API_KEY_GENERATION.md"
echo ""
