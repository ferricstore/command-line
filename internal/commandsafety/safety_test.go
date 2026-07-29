package commandsafety

import "testing"

func TestRequiresConfirmation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command []any
		want    bool
	}{
		{name: "flush database", command: []any{"flushdb"}, want: true},
		{name: "configuration write", command: []any{"CONFIG", "set", "maxmemory", "1gb"}, want: true},
		{name: "configuration read", command: []any{"CONFIG", "GET", "maxmemory"}},
		{name: "ACL mutation", command: []any{"ACL", "SETUSER", "operator"}, want: true},
		{name: "ACL read", command: []any{"ACL", "LIST"}},
		{name: "cluster mutation", command: []any{"CLUSTER.JOIN", "node-b"}, want: true},
		{name: "cluster read", command: []any{"CLUSTER.STATUS"}},
		{name: "schedule deletion", command: []any{"FLOW.SCHEDULE.DELETE", "daily"}, want: true},
		{name: "policy mutation", command: []any{"FLOW.POLICY.SET", "orders"}, want: true},
		{name: "projection repair", command: []any{"FERRICSTORE.DOCTOR", "START", "REPAIR", "PROJECTIONS"}, want: true},
		{name: "doctor check", command: []any{"FERRICSTORE.DOCTOR", "START", "CHECK"}},
		{name: "module load", command: []any{"MODULE", "LOAD", "extension.so"}, want: true},
		{name: "module list", command: []any{"MODULE", "LIST"}},
		{name: "ordinary write", command: []any{"SET", "key", "value"}},
		{name: "ordinary delete", command: []any{"DEL", "key"}},
		{name: "empty"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := RequiresConfirmation(test.command); got != test.want {
				t.Fatalf("RequiresConfirmation(%#v) = %t, want %t", test.command, got, test.want)
			}
		})
	}
}

func TestFirstRequiringConfirmation(t *testing.T) {
	t.Parallel()

	index, found := FirstRequiringConfirmation([][]any{
		{"SET", "one", "1"},
		{"CONFIG", "SET", "persistence.sync", "interval"},
		{"GET", "one"},
	})
	if !found || index != 1 {
		t.Fatalf("FirstRequiringConfirmation() = %d, %t", index, found)
	}
}
