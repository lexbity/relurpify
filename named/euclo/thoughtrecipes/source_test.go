// Package thoughtrecipe implements the Euclo thoughtrecipe DSL.
package thoughtrecipe

import (
	"bytes"
	"strings"
	"testing"
)

func TestSourceBuffer_Basic(t *testing.T) {
	src := "thoughtrecipe test\n\ntrigger as capability:\n  may read workspace"
	buffer := NewSourceBufferFromString("test.erpe", src)

	if buffer.Filename() != "test.erpe" {
		t.Errorf("expected filename 'test.erpe', got %q", buffer.Filename())
	}

	if buffer.String() != src {
		t.Errorf("expected source %q, got %q", src, buffer.String())
	}

	if buffer.Len() != len(src) {
		t.Errorf("expected length %d, got %d", len(src), buffer.Len())
	}

	if buffer.Digest() == "" {
		t.Error("expected non-empty digest")
	}
}

func TestSourceBuffer_Equal(t *testing.T) {
	src := "thoughtrecipe test"
	buf1 := NewSourceBufferFromString("a.erpe", src)
	buf2 := NewSourceBufferFromString("b.erpe", src)
	buf3 := NewSourceBufferFromString("c.erpe", "different")

	if !buf1.Equal(buf2) {
		t.Error("expected buf1 == buf2")
	}

	if buf1.Equal(buf3) {
		t.Error("expected buf1 != buf3")
	}

	if buf1.Equal(nil) {
		t.Error("expected buf1 != nil")
	}

	if !buf1.Equal(buf1) {
		t.Error("expected buf1 == buf1")
	}

	var nilBuf *SourceBuffer
	if !nilBuf.Equal(nilBuf) {
		t.Error("expected nil == nil")
	}
}

func TestSourceBuffer_CRLFPreservation(t *testing.T) {
	// Test that CRLF line endings are preserved
	srcWithCRLF := "line1\r\nline2\r\nline3"
	buffer := NewSourceBufferFromString("crlf.erpe", srcWithCRLF)

	if buffer.String() != srcWithCRLF {
		t.Errorf("CRLF not preserved: expected %q, got %q", srcWithCRLF, buffer.String())
	}

	if !bytes.Contains(buffer.Bytes(), []byte("\r\n")) {
		t.Error("expected CRLF bytes in buffer")
	}

	// Test that LF line endings are also preserved
	srcWithLF := "line1\nline2\nline3"
	buffer2 := NewSourceBufferFromString("lf.erpe", srcWithLF)

	if buffer2.String() != srcWithLF {
		t.Errorf("LF not preserved: expected %q, got %q", srcWithLF, buffer2.String())
	}

	if bytes.Contains(buffer2.Bytes(), []byte("\r")) {
		t.Error("unexpected CR in LF-only buffer")
	}
}

func TestSourceBuffer_SliceAndByteAt(t *testing.T) {
	src := "hello world"
	buffer := NewSourceBufferFromString("test.erpe", src)

	if buffer.ByteAt(0) != 'h' {
		t.Errorf("expected 'h' at position 0, got %c", buffer.ByteAt(0))
	}

	if buffer.ByteAt(6) != 'w' {
		t.Errorf("expected 'w' at position 6, got %c", buffer.ByteAt(6))
	}

	slice := buffer.Slice(0, 5)
	if string(slice) != "hello" {
		t.Errorf("expected 'hello', got %q", string(slice))
	}

	slice2 := buffer.Slice(6, 11)
	if string(slice2) != "world" {
		t.Errorf("expected 'world', got %q", string(slice2))
	}
}

func TestSourceSpan(t *testing.T) {
	span := NewByteSpan(10, 20)

	if span.Start != 10 {
		t.Errorf("expected start 10, got %d", span.Start)
	}

	if span.End != 20 {
		t.Errorf("expected end 20, got %d", span.End)
	}

	if span.Len() != 10 {
		t.Errorf("expected length 10, got %d", span.Len())
	}

	if !span.IsValid() {
		t.Error("expected span to be valid")
	}

	if !span.Contains(15) {
		t.Error("expected span to contain 15")
	}

	if !span.Contains(10) {
		t.Error("expected span to contain start")
	}

	if span.Contains(20) {
		t.Error("expected span not to contain end (exclusive)")
	}

	// Test invalid span
	invalid := NewByteSpan(20, 10)
	if invalid.IsValid() {
		t.Error("expected invalid span to report !IsValid")
	}
	if invalid.Len() != 0 {
		t.Errorf("expected invalid span length 0, got %d", invalid.Len())
	}

	// Test zero-length span
	zero := NewByteSpan(5, 5)
	if !zero.IsValid() {
		t.Error("expected zero-length span to be valid")
	}
	if zero.Len() != 0 {
		t.Errorf("expected zero-length span, got %d", zero.Len())
	}
}

func TestSourceSpan_Overlaps(t *testing.T) {
	span1 := NewByteSpan(0, 10)
	span2 := NewByteSpan(5, 15)
	span3 := NewByteSpan(15, 20)
	span4 := NewByteSpan(0, 5)

	if !span1.Overlaps(span2) {
		t.Error("expected span1 to overlap span2")
	}

	if span1.Overlaps(span3) {
		t.Error("expected span1 not to overlap span3")
	}

	if !span1.Overlaps(span4) {
		t.Error("expected span1 to overlap span4")
	}
}

func TestParseSourceWithBuffer(t *testing.T) {
	src := `thoughtrecipe test
"A test recipe"

trigger as capability:
  may read workspace
`
	
	docSrc, err := ParseSourceWithBuffer("test.erpe", src)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if docSrc.Buffer == nil {
		t.Fatal("expected buffer to be set")
	}

	if docSrc.Document == nil {
		t.Fatal("expected document to be set")
	}

	if docSrc.Digest != docSrc.Buffer.Digest() {
		t.Error("expected digest to match buffer digest")
	}

	if docSrc.Document.Name != "test" {
		t.Errorf("expected name 'test', got %q", docSrc.Document.Name)
	}
}

func TestParseFile(t *testing.T) {
	// Create a temporary file
	content := `thoughtrecipe test
"Test"
`
	
	// For now, just test that ParseSourceWithBuffer works
	// A full ParseFile test would require a real filesystem
	docSrc, err := ParseSourceWithBuffer("test.erpe", content)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if docSrc.Buffer.String() != content {
		t.Errorf("expected content %q, got %q", content, docSrc.Buffer.String())
	}
}

func TestSourceBuffer_Empty(t *testing.T) {
	buffer := NewSourceBufferFromString("", "")

	if buffer.Len() != 0 {
		t.Errorf("expected length 0, got %d", buffer.Len())
	}

	if buffer.String() != "" {
		t.Errorf("expected empty string, got %q", buffer.String())
	}

	if buffer.Digest() == "" {
		// SHA-256 of empty bytes is a known value
		t.Error("expected digest for empty content")
	}
}

func TestSourceBuffer_NilBytes(t *testing.T) {
	buffer := NewSourceBuffer("", nil)

	if buffer.Len() != 0 {
		t.Errorf("expected length 0 for nil bytes, got %d", buffer.Len())
	}

	if buffer.String() != "" {
		t.Errorf("expected empty string for nil bytes, got %q", buffer.String())
	}
}

func TestSourceBuffer_Unicode(t *testing.T) {
	// Test with multi-byte UTF-8 characters
	src := "thoughtrecipe 测试🎉\n" + strings.Repeat("x", 100)
	buffer := NewSourceBufferFromString("unicode.erpe", src)

	if buffer.String() != src {
		t.Errorf("expected unicode preserved: %q vs %q", src, buffer.String())
	}

	// Check that byte length is correct
	if buffer.Len() != len(src) {
		t.Errorf("expected byte length %d, got %d", len(src), buffer.Len())
	}
}
