// Package commandsafety classifies protocol commands that need an explicit
// operator confirmation before the CLI sends them to FerricStore.
package commandsafety

import (
	"fmt"
	"strings"
)

// RequiresConfirmation reports whether a raw command changes global,
// administrative, security, topology, or durable workflow configuration.
// Ordinary data-plane mutations such as SET and DEL intentionally do not
// require confirmation.
func RequiresConfirmation(command []any) bool {
	if len(command) == 0 {
		return false
	}
	name := token(command[0])
	subcommand := ""
	if len(command) > 1 {
		subcommand = token(command[1])
	}

	switch name {
	case "FLUSHDB", "FLUSHALL", "SHUTDOWN", "SAVE", "BGSAVE", "BGREWRITEAOF",
		"REPLICAOF", "SLAVEOF", "MIGRATE", "FERRICSTORE.BLOBGC",
		"FLOW.POLICY.SET", "FLOW.SCHEDULE.DELETE", "FLOW.CIRCUIT.OPEN", "FLOW.CIRCUIT.CLOSE":
		return true
	case "CONFIG", "FERRICSTORE.CONFIG":
		return matches(subcommand, "SET", "RESETSTAT", "REWRITE")
	case "SLOWLOG", "LATENCY":
		return subcommand == "RESET"
	case "ACL":
		return matches(subcommand, "SETUSER", "DELUSER", "LOAD", "SAVE")
	case "CLUSTER":
		return matches(subcommand, "MEET", "FORGET", "REPLICATE", "FAILOVER", "RESET", "ADDSLOTS", "DELSLOTS", "SETSLOT")
	case "CLUSTER.JOIN", "CLUSTER.LEAVE", "CLUSTER.FAILOVER", "CLUSTER.PROMOTE", "CLUSTER.DEMOTE":
		return true
	case "SCRIPT":
		return matches(subcommand, "FLUSH", "KILL")
	case "FUNCTION":
		return matches(subcommand, "DELETE", "FLUSH", "RESTORE")
	case "MODULE":
		return matches(subcommand, "LOAD", "LOADEX", "UNLOAD")
	case "CLIENT":
		return subcommand == "KILL"
	case "FERRICSTORE.DOCTOR":
		return subcommand == "START" && contains(command[2:], "REPAIR")
	default:
		return false
	}
}

// FirstRequiringConfirmation returns the zero-based position of the first
// safety-sensitive command in a batch.
func FirstRequiringConfirmation(commands [][]any) (int, bool) {
	for index, command := range commands {
		if RequiresConfirmation(command) {
			return index, true
		}
	}
	return 0, false
}

func token(value any) string {
	return strings.ToUpper(strings.TrimSpace(fmt.Sprint(value)))
}

func matches(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func contains(values []any, choice string) bool {
	for _, value := range values {
		if token(value) == choice {
			return true
		}
	}
	return false
}
