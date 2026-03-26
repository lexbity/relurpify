package graphdb

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"io"
	"math"
	"os"
	"sync"
	"time"
)

const (
	frameTypeOp byte = 1

	opCodeUpsertNode byte = iota + 1
	opCodeUpsertNodes
	opCodeDeleteNode
	opCodeDeleteNodes
	opCodeLinkEdge
	opCodeLinkEdges
	opCodeUnlinkEdge
)

type nodeOp struct {
	Node NodeRecord `json:"node"`
}

type nodeBatchOp struct {
	Nodes []NodeRecord `json:"nodes"`
}

type deleteNodeOp struct {
	ID string `json:"id"`
}

type deleteNodesOp struct {
	IDs []string `json:"ids"`
}

type edgeOp struct {
	Edge EdgeRecord `json:"edge"`
}

type edgeBatchOp struct {
	Edges []EdgeRecord `json:"edges"`
}

type unlinkOp struct {
	SourceID string   `json:"source_id"`
	TargetID string   `json:"target_id"`
	Kind     EdgeKind `json:"kind"`
	Hard     bool     `json:"hard"`
}

type binaryOp struct {
	code byte
	data []byte
}

type aofWriter struct {
	mu           sync.Mutex
	file         *os.File
	path         string
	syncMode     SyncMode
	syncInterval time.Duration
	lastSync     time.Time
}

func openAOF(path string, opts Options) (*aofWriter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	mode := opts.SyncMode
	if mode == "" {
		mode = SyncAlways
	}
	interval := opts.SyncInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	return &aofWriter{
		file:         file,
		path:         path,
		syncMode:     mode,
		syncInterval: interval,
		lastSync:     time.Now(),
	}, nil
}

func (w *aofWriter) appendOp(op binaryOp) error {
	if w == nil {
		return nil
	}
	frame := encodeFrame(frameTypeOp, append([]byte{op.code}, op.data...))
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.file.Write(frame); err != nil {
		return err
	}
	return w.syncLocked(false)
}

func (w *aofWriter) truncate() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.file.Truncate(0); err != nil {
		return err
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return w.syncLocked(true)
}

func (w *aofWriter) size() (int64, error) {
	if w == nil {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	info, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (w *aofWriter) close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.syncLocked(true); err != nil {
		return err
	}
	return w.file.Close()
}

func (w *aofWriter) syncLocked(force bool) error {
	if w == nil || w.file == nil {
		return nil
	}
	switch w.syncMode {
	case SyncOnFlush:
		if !force {
			return nil
		}
	case SyncInterval:
		if !force && time.Since(w.lastSync) < w.syncInterval {
			return nil
		}
	case SyncAlways:
	default:
		if !force {
			return nil
		}
	}
	if err := w.file.Sync(); err != nil {
		return err
	}
	w.lastSync = time.Now()
	return nil
}

func encodeFrame(frameType byte, payload []byte) []byte {
	out := make([]byte, 1+4+len(payload)+4)
	out[0] = frameType
	binary.LittleEndian.PutUint32(out[1:5], uint32(len(payload)))
	copy(out[5:5+len(payload)], payload)
	crc := crc32.ChecksumIEEE(payload)
	binary.LittleEndian.PutUint32(out[5+len(payload):], crc)
	return out
}

func replayAOF(path string, applyBinary func(binaryOp) error, applyLegacy func([]byte) error) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	for {
		frameType, payload, err := readFrame(file)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, errTruncatedFrame) {
			return nil
		}
		if err != nil {
			return err
		}
		if frameType != frameTypeOp {
			continue
		}
		if len(payload) == 0 {
			return io.ErrUnexpectedEOF
		}
		if payload[0] == '{' {
			if err := applyLegacy(payload); err != nil {
				return err
			}
			continue
		}
		if err := applyBinary(binaryOp{code: payload[0], data: payload[1:]}); err != nil {
			return err
		}
	}
}

var errTruncatedFrame = errors.New("truncated frame")
var errCorruptFrame = errors.New("corrupt frame")

func readFrame(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 5)
	n, err := io.ReadFull(r, header)
	if err != nil {
		if errors.Is(err, io.EOF) && n == 0 {
			return 0, nil, io.EOF
		}
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return 0, nil, errTruncatedFrame
		}
		return 0, nil, err
	}
	frameType := header[0]
	length := binary.LittleEndian.Uint32(header[1:5])
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return 0, nil, errTruncatedFrame
		}
		return 0, nil, err
	}
	crcBytes := make([]byte, 4)
	if _, err := io.ReadFull(r, crcBytes); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return 0, nil, errTruncatedFrame
		}
		return 0, nil, err
	}
	want := binary.LittleEndian.Uint32(crcBytes)
	if crc32.ChecksumIEEE(payload) != want {
		return 0, nil, errCorruptFrame
	}
	return frameType, payload, nil
}

func encodeBinaryOp(kind string, payload any) (binaryOp, error) {
	switch kind {
	case "upsert_node":
		op, ok := payload.(nodeOp)
		if !ok {
			return binaryOp{}, errors.New("graphdb: invalid upsert_node payload")
		}
		return binaryOp{code: opCodeUpsertNode, data: encodeNodeRecord(op.Node)}, nil
	case "upsert_nodes":
		op, ok := payload.(nodeBatchOp)
		if !ok {
			return binaryOp{}, errors.New("graphdb: invalid upsert_nodes payload")
		}
		var enc binaryEncoder
		enc.writeNodeRecords(op.Nodes)
		return binaryOp{code: opCodeUpsertNodes, data: enc.Bytes()}, nil
	case "delete_node":
		op, ok := payload.(deleteNodeOp)
		if !ok {
			return binaryOp{}, errors.New("graphdb: invalid delete_node payload")
		}
		return binaryOp{code: opCodeDeleteNode, data: encodeString(op.ID)}, nil
	case "delete_nodes":
		op, ok := payload.(deleteNodesOp)
		if !ok {
			return binaryOp{}, errors.New("graphdb: invalid delete_nodes payload")
		}
		var enc binaryEncoder
		enc.writeStrings(op.IDs)
		return binaryOp{code: opCodeDeleteNodes, data: enc.Bytes()}, nil
	case "link_edge":
		op, ok := payload.(edgeOp)
		if !ok {
			return binaryOp{}, errors.New("graphdb: invalid link_edge payload")
		}
		return binaryOp{code: opCodeLinkEdge, data: encodeEdgeRecord(op.Edge)}, nil
	case "link_edges":
		op, ok := payload.(edgeBatchOp)
		if !ok {
			return binaryOp{}, errors.New("graphdb: invalid link_edges payload")
		}
		var enc binaryEncoder
		enc.writeEdgeRecords(op.Edges)
		return binaryOp{code: opCodeLinkEdges, data: enc.Bytes()}, nil
	case "unlink_edge":
		op, ok := payload.(unlinkOp)
		if !ok {
			return binaryOp{}, errors.New("graphdb: invalid unlink_edge payload")
		}
		var enc binaryEncoder
		enc.writeString(op.SourceID)
		enc.writeString(op.TargetID)
		enc.writeString(string(op.Kind))
		enc.writeBool(op.Hard)
		return binaryOp{code: opCodeUnlinkEdge, data: enc.Bytes()}, nil
	default:
		return binaryOp{}, errors.New("graphdb: unsupported persisted op kind")
	}
}

func encodeNodeRecord(node NodeRecord) []byte {
	var enc binaryEncoder
	enc.writeNodeRecord(node)
	return enc.Bytes()
}

func encodeEdgeRecord(edge EdgeRecord) []byte {
	var enc binaryEncoder
	enc.writeEdgeRecord(edge)
	return enc.Bytes()
}

func encodeString(value string) []byte {
	var enc binaryEncoder
	enc.writeString(value)
	return enc.Bytes()
}

type binaryEncoder struct {
	buf bytes.Buffer
}

func (e *binaryEncoder) Bytes() []byte {
	return e.buf.Bytes()
}

func (e *binaryEncoder) writeBool(v bool) {
	if v {
		e.buf.WriteByte(1)
		return
	}
	e.buf.WriteByte(0)
}

func (e *binaryEncoder) writeUint32(v uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	e.buf.Write(buf[:])
}

func (e *binaryEncoder) writeInt64(v int64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(v))
	e.buf.Write(buf[:])
}

func (e *binaryEncoder) writeFloat32(v float32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], math.Float32bits(v))
	e.buf.Write(buf[:])
}

func (e *binaryEncoder) writeBytes(v []byte) {
	e.writeUint32(uint32(len(v)))
	e.buf.Write(v)
}

func (e *binaryEncoder) writeString(v string) {
	e.writeBytes([]byte(v))
}

func (e *binaryEncoder) writeStrings(values []string) {
	e.writeUint32(uint32(len(values)))
	for _, value := range values {
		e.writeString(value)
	}
}

func (e *binaryEncoder) writeNodeRecord(node NodeRecord) {
	e.writeString(node.ID)
	e.writeString(string(node.Kind))
	e.writeString(node.SourceID)
	e.writeStrings(node.Labels)
	e.writeBytes(node.Props)
	e.writeInt64(node.CreatedAt)
	e.writeInt64(node.UpdatedAt)
	e.writeInt64(node.DeletedAt)
}

func (e *binaryEncoder) writeNodeRecords(nodes []NodeRecord) {
	e.writeUint32(uint32(len(nodes)))
	for _, node := range nodes {
		e.writeNodeRecord(node)
	}
}

func (e *binaryEncoder) writeEdgeRecord(edge EdgeRecord) {
	e.writeString(edge.SourceID)
	e.writeString(edge.TargetID)
	e.writeString(string(edge.Kind))
	e.writeFloat32(edge.Weight)
	e.writeBytes(edge.Props)
	e.writeInt64(edge.CreatedAt)
	e.writeInt64(edge.DeletedAt)
}

func (e *binaryEncoder) writeEdgeRecords(edges []EdgeRecord) {
	e.writeUint32(uint32(len(edges)))
	for _, edge := range edges {
		e.writeEdgeRecord(edge)
	}
}

type binaryDecoder struct {
	r *bytes.Reader
}

func binaryDecoderFromBytes(data []byte) binaryDecoder {
	return binaryDecoder{r: bytes.NewReader(data)}
}

func (d *binaryDecoder) finish() error {
	if d.r.Len() != 0 {
		return errors.New("graphdb: trailing binary op data")
	}
	return nil
}

func (d *binaryDecoder) readBool() (bool, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return false, err
	}
	return b != 0, nil
}

func (d *binaryDecoder) readUint32() (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

func (d *binaryDecoder) readInt64() (int64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(buf[:])), nil
}

func (d *binaryDecoder) readFloat32() (float32, error) {
	bits, err := d.readUint32()
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(bits), nil
}

func (d *binaryDecoder) readBytes() ([]byte, error) {
	n, err := d.readUint32()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(d.r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (d *binaryDecoder) readString() (string, error) {
	buf, err := d.readBytes()
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func (d *binaryDecoder) readStrings() ([]string, error) {
	n, err := d.readUint32()
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, n)
	for i := uint32(0); i < n; i++ {
		value, err := d.readString()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (d *binaryDecoder) readNodeRecord() (NodeRecord, error) {
	id, err := d.readString()
	if err != nil {
		return NodeRecord{}, err
	}
	kind, err := d.readString()
	if err != nil {
		return NodeRecord{}, err
	}
	sourceID, err := d.readString()
	if err != nil {
		return NodeRecord{}, err
	}
	labels, err := d.readStrings()
	if err != nil {
		return NodeRecord{}, err
	}
	props, err := d.readBytes()
	if err != nil {
		return NodeRecord{}, err
	}
	createdAt, err := d.readInt64()
	if err != nil {
		return NodeRecord{}, err
	}
	updatedAt, err := d.readInt64()
	if err != nil {
		return NodeRecord{}, err
	}
	deletedAt, err := d.readInt64()
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:        id,
		Kind:      NodeKind(kind),
		SourceID:  sourceID,
		Labels:    labels,
		Props:     props,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		DeletedAt: deletedAt,
	}, nil
}

func (d *binaryDecoder) readNodeRecords() ([]NodeRecord, error) {
	n, err := d.readUint32()
	if err != nil {
		return nil, err
	}
	nodes := make([]NodeRecord, 0, n)
	for i := uint32(0); i < n; i++ {
		node, err := d.readNodeRecord()
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (d *binaryDecoder) readEdgeRecord() (EdgeRecord, error) {
	sourceID, err := d.readString()
	if err != nil {
		return EdgeRecord{}, err
	}
	targetID, err := d.readString()
	if err != nil {
		return EdgeRecord{}, err
	}
	kind, err := d.readString()
	if err != nil {
		return EdgeRecord{}, err
	}
	weight, err := d.readFloat32()
	if err != nil {
		return EdgeRecord{}, err
	}
	props, err := d.readBytes()
	if err != nil {
		return EdgeRecord{}, err
	}
	createdAt, err := d.readInt64()
	if err != nil {
		return EdgeRecord{}, err
	}
	deletedAt, err := d.readInt64()
	if err != nil {
		return EdgeRecord{}, err
	}
	return EdgeRecord{
		SourceID:  sourceID,
		TargetID:  targetID,
		Kind:      EdgeKind(kind),
		Weight:    weight,
		Props:     props,
		CreatedAt: createdAt,
		DeletedAt: deletedAt,
	}, nil
}

func (d *binaryDecoder) readEdgeRecords() ([]EdgeRecord, error) {
	n, err := d.readUint32()
	if err != nil {
		return nil, err
	}
	edges := make([]EdgeRecord, 0, n)
	for i := uint32(0); i < n; i++ {
		edge, err := d.readEdgeRecord()
		if err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

func opKindForPayload(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", io.ErrUnexpectedEOF
	}
	if payload[0] == '{' {
		var op struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(payload, &op); err != nil {
			return "", err
		}
		return op.Kind, nil
	}
	switch payload[0] {
	case opCodeUpsertNode:
		return "upsert_node", nil
	case opCodeUpsertNodes:
		return "upsert_nodes", nil
	case opCodeDeleteNode:
		return "delete_node", nil
	case opCodeDeleteNodes:
		return "delete_nodes", nil
	case opCodeLinkEdge:
		return "link_edge", nil
	case opCodeLinkEdges:
		return "link_edges", nil
	case opCodeUnlinkEdge:
		return "unlink_edge", nil
	default:
		return "", errors.New("graphdb: unknown binary op code")
	}
}
