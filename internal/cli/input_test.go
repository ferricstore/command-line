package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValueInputRejectsOversizedStdin(t *testing.T) {
	t.Parallel()

	command := &cobra.Command{}
	command.SetIn(strings.NewReader(strings.Repeat("x", 16*1024*1024+1)))
	_, _, err := (valueInput{file: "-"}).read(command, nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("valueInput.read() error = %v, want size-limit rejection", err)
	}
}
