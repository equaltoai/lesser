#!/bin/bash
#
# Setup script for Lesser test environment
#

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Setting up Lesser test environment...${NC}"

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="$SCRIPT_DIR/../test_venv"

# Create virtual environment if it doesn't exist
if [ ! -d "$VENV_DIR" ]; then
    echo -e "${YELLOW}Creating virtual environment...${NC}"
    python3 -m venv "$VENV_DIR"
else
    echo -e "${GREEN}Virtual environment already exists${NC}"
fi

# Activate virtual environment
echo -e "${YELLOW}Activating virtual environment...${NC}"
source "$VENV_DIR/bin/activate"

# Upgrade pip
echo -e "${YELLOW}Upgrading pip...${NC}"
pip install --upgrade pip

# Install requirements
if [ -f "$SCRIPT_DIR/requirements.txt" ]; then
    echo -e "${YELLOW}Installing test requirements...${NC}"
    pip install -r "$SCRIPT_DIR/requirements.txt"
else
    echo -e "${YELLOW}⚠ requirements.txt not found, installing basic dependencies...${NC}"
    pip install requests websocket-client cryptography Pillow jsonschema pytest python-dateutil urllib3 python-dotenv
fi

# Verify installation
echo -e "${BLUE}Verifying installations...${NC}"
python -c "import requests; print(f'✓ requests {requests.__version__}')"
python -c "import websocket; print('✓ websocket-client installed')"
python -c "import cryptography; print('✓ cryptography installed')"

echo -e "${GREEN}Test environment setup complete!${NC}"
echo -e "${YELLOW}To run tests, use:${NC}"
echo "  export LESSER_URL=https://lesser.host"
echo "  export LESSER_AUTH_TOKEN=your-token-here"
echo "  ./run_comprehensive_validation.sh" 