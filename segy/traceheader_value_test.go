package segy

import (
	"encoding/binary"
	"testing"
)

func TestDecodeTraceHeaderValue(t *testing.T) {
	data := make([]byte, 8)
	binary.BigEndian.PutUint32(data[0:4], 0x3f800000) // 1.0f
	binary.BigEndian.PutUint16(data[4:6], 1234)
	data[6] = 7
	data[7] = 9

	v, err := DecodeTraceHeaderValue(data, TraceHeaderValueTypeFloat32, binary.BigEndian)
	if err != nil {
		t.Fatalf("float32 decode failed: %v", err)
	}
	if got, ok := v.(float32); !ok || got != 1.0 {
		t.Fatalf("float32 decode mismatch: got %#v want 1.0", v)
	}

	v, err = DecodeTraceHeaderValue(data[4:8], TraceHeaderValueTypeUint16, binary.BigEndian)
	if err != nil {
		t.Fatalf("uint16 decode failed: %v", err)
	}
	if got, ok := v.(uint16); !ok || got != 1234 {
		t.Fatalf("uint16 decode mismatch: got %#v want 1234", v)
	}

	v, err = DecodeTraceHeaderValue(data[6:8], TraceHeaderValueTypeUint8, binary.BigEndian)
	if err != nil {
		t.Fatalf("uint8 decode failed: %v", err)
	}
	if got, ok := v.(uint8); !ok || got != 7 {
		t.Fatalf("uint8 decode mismatch: got %#v want 7", v)
	}
}

func TestReadCustomTraceValuesFromBytes(t *testing.T) {
	trace := make([]byte, 16)
	binary.BigEndian.PutUint16(trace[0:2], 1234)
	binary.BigEndian.PutUint32(trace[4:8], 0x40400000) // 3.0f
	trace[8] = 42

	fields := []CustomTraceField{
		{Name: "foo", Offset: 1, Type: TraceHeaderValueTypeUint16},
		{Name: "bar", Offset: 5, Type: TraceHeaderValueTypeFloat32},
		{Name: "baz", Offset: 9, Type: TraceHeaderValueTypeUint8},
	}

	got, err := ReadCustomTraceValuesFromBytes(trace, fields, binary.BigEndian)
	if err != nil {
		t.Fatalf("ReadCustomTraceValuesFromBytes failed: %v", err)
	}
	if got["foo"] != uint16(1234) {
		t.Fatalf("foo mismatch: got %#v want 1234", got["foo"])
	}
	if got["bar"] != float32(3.0) {
		t.Fatalf("bar mismatch: got %#v want 3.0", got["bar"])
	}
	if got["baz"] != uint8(42) {
		t.Fatalf("baz mismatch: got %#v want 42", got["baz"])
	}
}
