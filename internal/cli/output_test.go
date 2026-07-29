package cli

import (
	"bytes"
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
