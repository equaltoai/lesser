package streaming

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHLSGenerator_parseSampleTimes_ValidSTTS(t *testing.T) {
	g := &HLSGenerator{}

	data := append([]byte("prefix"), buildSTTSAtom([]sttsEntry{
		{SampleCount: 2, SampleDuration: 10},
		{SampleCount: 1, SampleDuration: 20},
	})...)

	got := g.parseSampleTimes(data)
	require.Equal(t, []uint32{0, 10, 20}, got)
}

func TestHLSGenerator_parseSampleTimes_InvalidAtomSize(t *testing.T) {
	g := &HLSGenerator{}

	atom := buildSTTSAtom([]sttsEntry{{SampleCount: 1, SampleDuration: 10}})
	binary.BigEndian.PutUint32(atom[0:4], uint32(len(atom)+4)) // Make size invalid.

	assert.Nil(t, g.parseSampleTimes(atom))
}

func TestHLSGenerator_parseChunkOffsets_ValidSTCO(t *testing.T) {
	g := &HLSGenerator{}

	data := append([]byte("prefix"), buildSTCOAtom([]uint32{100, 200, 300})...)
	require.Equal(t, []uint32{100, 200, 300}, g.parseChunkOffsets(data))
}

func TestHLSGenerator_parseChunkOffsets_InvalidEntryCount(t *testing.T) {
	g := &HLSGenerator{}

	atom := buildSTCOAtom(nil)
	assert.Nil(t, g.parseChunkOffsets(atom))
}

func TestHLSGenerator_calculatePTS_And_calculateByteOffset(t *testing.T) {
	g := &HLSGenerator{}

	assert.InDelta(t, 1.0, g.calculatePTS(1, []uint32{0, 10}, 10), 0.0001)
	assert.InDelta(t, 10.0, g.calculatePTS(5, []uint32{0, 10}, 10), 0.0001)

	assert.Equal(t, int64(200), g.calculateByteOffset(3, []uint32{100, 200}, []byte("data"), 10))
	assert.Equal(t, int64(500), g.calculateByteOffset(2, nil, make([]byte, 1000), 4))
}

type sttsEntry struct {
	SampleCount    uint32
	SampleDuration uint32
}

func buildSTTSAtom(entries []sttsEntry) []byte {
	payload := make([]byte, 8+len(entries)*8) // version/flags + entry count + entries
	binary.BigEndian.PutUint32(payload[4:8], uint32(len(entries)))

	for i, entry := range entries {
		offset := 8 + i*8
		binary.BigEndian.PutUint32(payload[offset:offset+4], entry.SampleCount)
		binary.BigEndian.PutUint32(payload[offset+4:offset+8], entry.SampleDuration)
	}

	size := uint32(8 + len(payload))
	atom := make([]byte, int(size))
	binary.BigEndian.PutUint32(atom[0:4], size)
	copy(atom[4:8], []byte("stts"))
	copy(atom[8:], payload)

	return atom
}

func buildSTCOAtom(offsets []uint32) []byte {
	payload := make([]byte, 8+len(offsets)*4) // version/flags + entry count + offsets
	binary.BigEndian.PutUint32(payload[4:8], uint32(len(offsets)))
	for i, offsetValue := range offsets {
		offset := 8 + i*4
		binary.BigEndian.PutUint32(payload[offset:offset+4], offsetValue)
	}

	size := uint32(8 + len(payload))
	atom := make([]byte, int(size))
	binary.BigEndian.PutUint32(atom[0:4], size)
	copy(atom[4:8], []byte("stco"))
	copy(atom[8:], payload)

	return atom
}

