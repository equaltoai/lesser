#!/bin/bash

echo "Checking test file paths..."
echo

echo "1. API test file:"
if [ -f "tests/api/comprehensive_api_test.py" ]; then
    echo "   ✓ tests/api/comprehensive_api_test.py exists"
else
    echo "   ✗ tests/api/comprehensive_api_test.py NOT FOUND"
fi

echo
echo "2. Federation test file:"
if [ -f "tests/federation/test_federation_validation.py" ]; then
    echo "   ✓ tests/federation/test_federation_validation.py exists"
else
    echo "   ✗ tests/federation/test_federation_validation.py NOT FOUND"
fi

echo
echo "3. Checking Python:"
which python3 || echo "   ✗ python3 not found in PATH"
python3 --version || echo "   ✗ Cannot get Python version"

echo
echo "4. Checking Python packages:"
python3 -c "import requests" 2>/dev/null && echo "   ✓ requests module installed" || echo "   ✗ requests module NOT installed" 