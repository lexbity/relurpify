package graphdb

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeFrame(t *testing.T) {
	payload := []byte("hello world")
	frame := encodeFrame(frameTypeOp, payload)

	// decode using readFrame
	r := bytes.NewReader(frame)
	ft, decoded, err := readFrame(r)
	require.NoError(t, err)
	require.Equal(t, frameTypeOp, ft)
	require.Equal(t, payload, decoded)
}

func TestReadFrame_TruncatedHeader(t *testing.T) {
	r := bytes.NewReader([]byte{frameTypeOp}) // missing length & crc
	_, _, err := readFrame(r)
	require.ErrorIs(t, err, errTruncatedFrame)
}

func TestReadFrame_CorruptChecksum(t *testing.T) {
	payload := []byte("test")
	frame := encodeFrame(frameTypeOp, payload)
	// corrupt checksum
	binary.LittleEndian.PutUint32(frame[len(frame)-4:], 0xdeadbeef)

	r := bytes.NewReader(frame)
	_, _, err := readFrame(r)
	require.ErrorIs(t, err, errCorruptFrame)
}

func TestReplayAOF_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.aof")
	err := os.WriteFile(path, []byte{}, 0o644)
	require.NoError(t, err)

	calls := 0
	apply := func(op binaryOp) error {
		calls++
		return nil
	}
	applyLegacy := func([]byte) error {
		calls++
		return nil
	}
	err = replayAOF(path, apply, applyLegacy)
	require.NoError(t, err)
	require.Zero(t, calls)
}

func TestReplayAOF_TruncatedFrameIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "truncated.aof")
	payload := []byte("payload")
	frame := encodeFrame(frameTypeOp, payload)
	// write only first half of frame
	file, err := os.Create(path)
	require.NoError(t, err)
	_, err = file.Write(frame[:len(frame)-8])
	require.NoError(t, err)
	file.Close()

	called := false
	apply := func(op binaryOp) error {
		called = true
		return nil
	}
	err = replayAOF(path, apply, func([]byte) error { return nil })
	require.NoError(t, err)
	require.False(t, called) // truncated frame should be ignored, not cause error
}

func TestBinaryEncoderDecoderRoundtrip(t *testing.T) {
	node := NodeRecord{
		ID:        "test-id",
		Kind:      "function",
		SourceID:  "source.go",
		Labels:    []string{"l1", "l2"},
		Props:     []byte(`{"x":1}`),
		CreatedAt: 1000,
		UpdatedAt: 2000,
		DeletedAt: 0,
	}
	encoded := encodeNodeRecord(node)
	dec := binaryDecoderFromBytes(encoded)
	decoded, err := dec.readNodeRecord()
	require.NoError(t, err)
	require.Equal(t, node, decoded)
	require.NoError(t, dec.finish())
}

func TestBinaryDecoder_TrailingDataError(t *testing.T) {
	data := []byte{0, 0, 0, 1, 'a'} // extra byte after valid string length=1 + 'a'
	dec := binaryDecoderFromBytes(data)
	_, err := dec.readString()
	require.NoError(t, err)
	err = dec.finish()
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing binary op data")
}

func TestOpKindForPayload_Binary(t *testing.T) {
	kind, err := opKindForPayload([]byte{opCodeUpsertNode})
	require.NoError(t, err)
	require.Equal(t, "upsert_node", kind)

	kind, err = opKindForPayload([]byte{opCodeLinkEdges})
	require.NoError(t, err)
	require.Equal(t, "link_edges", kind)

	_, err = opKindForPayload([]byte{0x99})
	require.Error(t, err)
}

func TestOpKindForPayload_JSON(t *testing.T) {
	payload := []byte(`{"kind":"upsert_node"}`)
	kind, err := opKindForPayload(payload)
	require.NoError(t, err)
	require.Equal(t, "upsert_node", kind)
}

func TestAOFWriter_SyncModes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.aof")
	opts := DefaultOptions(dir)
	opts.SyncMode = SyncOnFlush
	writer, err := openAOF(path, opts)
	require.NoError(t, err)
	defer writer.close()

	// write an op
	op := binaryOp{code: opCodeUpsertNode, data: encodeNodeRecord(NodeRecord{ID: "x"})}
	require.NoError(t, writer.appendOp(op))

	// file should exist
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))
}
