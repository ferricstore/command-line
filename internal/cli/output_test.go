package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

type outputFixture struct {
	ID           string
	LeaseToken   string
	FencingToken int64
}

func TestWriteResultRejectsStructWithoutExplicitOutputContract(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := writeResult(&output, outputAuto, outputFixture{
		ID:           "job-42",
		LeaseToken:   "lease",
		FencingToken: 7,
	})
	if err == nil || !strings.Contains(err.Error(), "explicit output contract") {
		t.Fatalf("writeResult() error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("writeResult() wrote partial output: %q", output.String())
	}
}

func TestWriteResultRawPrintsOneScalarPerLine(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := writeResult(&output, outputRaw, []any{"a", int64(2), nil}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "a\n2\n\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestOutputFormatRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	var format outputFormat
	if err := format.Set("yaml"); err == nil {
		t.Fatal("Set() error = nil")
	}
}

func TestJSONOutputPreservesInvalidUTF8AsTaggedBase64(t *testing.T) {
	t.Parallel()

	value := []byte{0xff, 0x00, 0xfe}
	var output bytes.Buffer
	if err := writeResult(&output, outputJSON, map[string]any{"value": value}); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	encoded, ok := decoded["value"].(map[string]any)
	if !ok {
		t.Fatalf("JSON value = %#v, want tagged binary object", decoded["value"])
	}
	if encoded["encoding"] != "base64" || encoded["data"] != base64.StdEncoding.EncodeToString(value) {
		t.Fatalf("JSON value = %#v", encoded)
	}
}
