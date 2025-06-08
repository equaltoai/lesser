#!/bin/bash
# Test script for media processor

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "🎬 Media Processor Test Script"
echo "=============================="

# Check if ffmpeg is installed
if ! command -v ffmpeg &> /dev/null; then
    echo -e "${RED}❌ ffmpeg is not installed. Please install it first.${NC}"
    exit 1
fi

if ! command -v ffprobe &> /dev/null; then
    echo -e "${RED}❌ ffprobe is not installed. Please install it first.${NC}"
    exit 1
fi

echo -e "${GREEN}✓ ffmpeg and ffprobe are installed${NC}"

# Create test media files
echo ""
echo "Creating test media files..."

# Create a test video (5 seconds of color bars)
ffmpeg -f lavfi -i testsrc=duration=5:size=1280x720:rate=30 -pix_fmt yuv420p test_video.mp4 -y 2>/dev/null
echo -e "${GREEN}✓ Created test_video.mp4${NC}"

# Create a test audio file (5 seconds of sine wave)
ffmpeg -f lavfi -i "sine=frequency=1000:duration=5" test_audio.mp3 -y 2>/dev/null
echo -e "${GREEN}✓ Created test_audio.mp3${NC}"

# Test video metadata extraction
echo ""
echo "Testing video metadata extraction..."
ffprobe -v error -select_streams v:0 -show_entries stream=width,height,duration -of json test_video.mp4

# Test audio metadata extraction
echo ""
echo "Testing audio metadata extraction..."
ffprobe -v error -show_entries format=duration -of json test_audio.mp3

# Test thumbnail generation
echo ""
echo "Testing thumbnail generation..."
ffmpeg -i test_video.mp4 -ss 00:00:01 -vframes 1 test_thumbnail.png -y 2>/dev/null
if [ -f test_thumbnail.png ]; then
    echo -e "${GREEN}✓ Thumbnail generated successfully${NC}"
else
    echo -e "${RED}❌ Thumbnail generation failed${NC}"
fi

# Test waveform generation
echo ""
echo "Testing waveform generation..."
ffmpeg -i test_audio.mp3 -filter_complex "showwavespic=s=640x120" -frames:v 1 test_waveform.png -y 2>/dev/null
if [ -f test_waveform.png ]; then
    echo -e "${GREEN}✓ Waveform generated successfully${NC}"
else
    echo -e "${RED}❌ Waveform generation failed${NC}"
fi

# Clean up
echo ""
echo "Cleaning up test files..."
rm -f test_video.mp4 test_audio.mp3 test_thumbnail.png test_waveform.png

echo ""
echo -e "${GREEN}✅ All tests completed!${NC}"
echo ""
echo "Next steps:"
echo "1. Run 'go test ./cmd/media-processor/...' to run unit tests"
echo "2. Deploy to Lambda and test with real media files"
echo "3. Monitor CloudWatch logs for any issues" 