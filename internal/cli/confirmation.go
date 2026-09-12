package cli

import (
	"fmt"

	"github.com/ferricstore/command-line/internal/commandsafety"
)

func requireProtocolConfirmation(confirmed bool, operation string, wire []any) error {
	if confirmed || !commandsafety.RequiresConfirmation(wire) {
		return nil
	}
	return fmt.Errorf("%s requires --yes", operation)
}
