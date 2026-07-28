package cli

import (
	"bytes"
	"testing"
)

type outputFixture struct {
	ID           string
	LeaseToken   string
	FencingToken int64
}

func TestWriteResultFormatsStructuredValuesWithStableFieldNames(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := writeResult(&output, outputAuto, outputFixture{
		ID:           "job-42",
		LeaseToken:   "lease",
		FencingToken: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"fencing_token\": 7,\n  \"id\": \"job-42\",\n  \"lease_token\": \"lease\"\n}\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
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
