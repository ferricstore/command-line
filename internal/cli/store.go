package cli

import (
	"errors"
	"strings"

	"github.com/ferricstore/command-line/internal/commandsafety"
	"github.com/spf13/cobra"
)

const (
	storeStringsGroup     = "strings"
	storeHashesGroup      = "hashes"
	storeListsGroup       = "lists"
	storeSetsGroup        = "sets"
	storeSortedSetsGroup  = "sorted-sets"
	storeStreamsGroup     = "streams"
	storeKeysGroup        = "keys"
	storeSpecializedGroup = "specialized"
	storeNativeGroup      = "native"
	storeEscapeHatchGroup = "escape-hatch"
)

func newStoreCommand(dependencies dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "store",
		Short: "Read and change FerricStore data",
		Long: "Read and change FerricStore data using familiar Redis command names.\n\n" +
			"Arguments after the first protocol argument are passed in Redis order. " +
			"Use 'ferric store command' as an escape hatch for specialized commands.",
		Example: "  ferric store get user:42\n" +
			"  ferric store set user:42 active EX 300\n" +
			"  ferric store hgetall user:42\n" +
			"  ferric store zrange leaderboard 0 -1 WITHSCORES",
		Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddGroup(
		&cobra.Group{ID: storeStringsGroup, Title: "String Commands:"},
		&cobra.Group{ID: storeHashesGroup, Title: "Hash Commands:"},
		&cobra.Group{ID: storeListsGroup, Title: "List Commands:"},
		&cobra.Group{ID: storeSetsGroup, Title: "Set Commands:"},
		&cobra.Group{ID: storeSortedSetsGroup, Title: "Sorted Set Commands:"},
		&cobra.Group{ID: storeStreamsGroup, Title: "Stream Commands:"},
		&cobra.Group{ID: storeKeysGroup, Title: "Key and Expiry Commands:"},
		&cobra.Group{ID: storeSpecializedGroup, Title: "Specialized Data Commands:"},
		&cobra.Group{ID: storeNativeGroup, Title: "FerricStore-Native Commands:"},
		&cobra.Group{ID: storeEscapeHatchGroup, Title: "Advanced Commands:"},
	)
	for _, spec := range storeCommandSpecs() {
		command.AddCommand(newSDKCommand(dependencies, spec))
	}
	command.AddCommand(newStoreRawCommand(dependencies))
	command.AddCommand(newStoreTransactionCommand(dependencies))
	return command
}

func newStoreRawCommand(dependencies dependencies) *cobra.Command {
	var yes bool
	spec := sdkCommandSpec{
		name:  "command",
		use:   "command <name> [args...]",
		short: "Execute one SDK command without a dedicated helper",
		long:  "Execute exactly one FerricStore command through the Go SDK. Prefer a named store helper when one exists.",
		example: "  ferric store command ECHO hello\n" +
			"  ferric store command FERRICSTORE.CONFIG GET persistence.*",
		group:   storeEscapeHatchGroup,
		wire:    []any{"COMMAND_PLACEHOLDER"},
		minArgs: 1,
		maxArgs: -1,
	}
	command := newSDKCommand(dependencies, spec)
	command.RunE = func(command *cobra.Command, args []string) error {
		confirmed := yes
		protocolArgs := append([]string(nil), args...)
		candidate := make([]any, len(protocolArgs))
		for index, argument := range protocolArgs {
			candidate[index] = argument
		}
		sensitive := commandsafety.RequiresConfirmation(candidate)
		if sensitive && !confirmed {
			for index := len(protocolArgs) - 1; index >= 0; index-- {
				if protocolArgs[index] == "--yes" {
					confirmed = true
					protocolArgs = append(protocolArgs[:index], protocolArgs[index+1:]...)
					break
				}
			}
		}
		if len(protocolArgs) == 0 {
			return errors.New("raw command name is required")
		}
		wireArgs := make([]any, 0, len(protocolArgs))
		for index, argument := range protocolArgs {
			if index == 0 {
				argument = strings.ToUpper(argument)
			}
			wireArgs = append(wireArgs, argument)
		}
		if sensitive && !confirmed {
			return errors.New(strings.ToLower(protocolArgs[0]) + " requires --yes")
		}
		actual := sdkCommandSpec{
			wire:    wireArgs,
			minArgs: 0,
			maxArgs: 0,
		}
		delegate := newSDKCommand(dependencies, actual)
		delegate.SetOut(command.OutOrStdout())
		delegate.SetErr(command.ErrOrStderr())
		delegate.SetIn(command.InOrStdin())
		delegate.SetContext(command.Context())
		return delegate.RunE(command, nil)
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm a safety-sensitive raw command")
	return command
}

func storeSpec(group, name, use, short, example string, minArgs, maxArgs int) sdkCommandSpec {
	return sdkCommandSpec{
		name:    name,
		use:     use,
		short:   short,
		example: "  ferric store " + example,
		group:   group,
		wire:    []any{strings.ToUpper(name)},
		minArgs: minArgs,
		maxArgs: maxArgs,
	}
}

func storeCommandSpecs() []sdkCommandSpec {
	return []sdkCommandSpec{
		// Strings.
		storeSpec(storeStringsGroup, "get", "get <key>", "Get the value of a key", "get user:42", 1, 1),
		storeSpec(storeStringsGroup, "set", "set <key> <value> [condition or expiry...]", "Set a key using Redis-compatible options", "set session:42 active EX 300 NX", 2, -1),
		storeSpec(storeStringsGroup, "mget", "mget <key>...", "Get multiple string values", "mget user:1 user:2", 1, -1),
		storeSpec(storeStringsGroup, "mset", "mset <key> <value> [key value...]", "Set multiple string values", "mset user:1 Ada user:2 Grace", 2, -1),
		storeSpec(storeStringsGroup, "msetnx", "msetnx <key> <value> [key value...]", "Set multiple values only when none exists", "msetnx lock:a 1 lock:b 1", 2, -1),
		storeSpec(storeStringsGroup, "incr", "incr <key>", "Increment an integer value", "incr page:views", 1, 1),
		storeSpec(storeStringsGroup, "decr", "decr <key>", "Decrement an integer value", "decr available:slots", 1, 1),
		storeSpec(storeStringsGroup, "incrby", "incrby <key> <increment>", "Increment an integer by an amount", "incrby page:views 10", 2, 2),
		storeSpec(storeStringsGroup, "decrby", "decrby <key> <decrement>", "Decrement an integer by an amount", "decrby available:slots 2", 2, 2),
		storeSpec(storeStringsGroup, "incrbyfloat", "incrbyfloat <key> <increment>", "Increment a floating-point value", "incrbyfloat balance:42 1.25", 2, 2),
		storeSpec(storeStringsGroup, "append", "append <key> <value>", "Append to a string", "append log:42 next", 2, 2),
		storeSpec(storeStringsGroup, "strlen", "strlen <key>", "Get a string length", "strlen user:42", 1, 1),
		storeSpec(storeStringsGroup, "getset", "getset <key> <value>", "Replace and return a string value", "getset current:leader node-2", 2, 2),
		storeSpec(storeStringsGroup, "getdel", "getdel <key>", "Get and delete a string value", "getdel one-time:token", 1, 1),
		storeSpec(storeStringsGroup, "getex", "getex <key> [expiry option]", "Get a value and optionally change its expiry", "getex session:42 EX 300", 1, -1),
		storeSpec(storeStringsGroup, "setnx", "setnx <key> <value>", "Set a value only when the key does not exist", "setnx lock:42 owner-a", 2, 2),
		storeSpec(storeStringsGroup, "setex", "setex <key> <seconds> <value>", "Set a value with a lifetime in seconds", "setex session:42 300 active", 3, 3),
		storeSpec(storeStringsGroup, "psetex", "psetex <key> <milliseconds> <value>", "Set a value with a lifetime in milliseconds", "psetex lock:42 1500 owner-a", 3, 3),
		storeSpec(storeStringsGroup, "getrange", "getrange <key> <start> <end>", "Get a byte range from a string", "getrange document:42 0 99", 3, 3),
		storeSpec(storeStringsGroup, "setrange", "setrange <key> <offset> <value>", "Overwrite a byte range in a string", "setrange document:42 0 prefix", 3, 3),

		// Hashes.
		storeSpec(storeHashesGroup, "hset", "hset <key> <field> <value> [field value...]", "Set one or more hash fields", "hset user:42 name Ada active true", 3, -1),
		storeSpec(storeHashesGroup, "hget", "hget <key> <field>", "Get a hash field", "hget user:42 name", 2, 2),
		storeSpec(storeHashesGroup, "hdel", "hdel <key> <field>...", "Delete hash fields", "hdel user:42 temporary", 2, -1),
		storeSpec(storeHashesGroup, "hmget", "hmget <key> <field>...", "Get multiple hash fields", "hmget user:42 name active", 2, -1),
		storeSpec(storeHashesGroup, "hgetall", "hgetall <key>", "Get every field and value in a hash", "hgetall user:42", 1, 1),
		storeSpec(storeHashesGroup, "hexists", "hexists <key> <field>", "Test whether a hash field exists", "hexists user:42 email", 2, 2),
		storeSpec(storeHashesGroup, "hlen", "hlen <key>", "Count fields in a hash", "hlen user:42", 1, 1),
		storeSpec(storeHashesGroup, "hkeys", "hkeys <key>", "List hash field names", "hkeys user:42", 1, 1),
		storeSpec(storeHashesGroup, "hvals", "hvals <key>", "List hash values", "hvals user:42", 1, 1),
		storeSpec(storeHashesGroup, "hincrby", "hincrby <key> <field> <increment>", "Increment an integer hash field", "hincrby user:42 logins 1", 3, 3),
		storeSpec(storeHashesGroup, "hincrbyfloat", "hincrbyfloat <key> <field> <increment>", "Increment a floating-point hash field", "hincrbyfloat user:42 balance 2.5", 3, 3),
		storeSpec(storeHashesGroup, "hsetnx", "hsetnx <key> <field> <value>", "Set a hash field only when absent", "hsetnx user:42 created 2026-07-25", 3, 3),
		storeSpec(storeHashesGroup, "hstrlen", "hstrlen <key> <field>", "Get a hash field string length", "hstrlen user:42 name", 2, 2),
		storeSpec(storeHashesGroup, "hrandfield", "hrandfield <key> [count [WITHVALUES]]", "Return random hash fields", "hrandfield user:42 2 WITHVALUES", 1, 3),
		storeSpec(storeHashesGroup, "hscan", "hscan <key> <cursor> [MATCH pattern] [COUNT count]", "Incrementally scan a hash", "hscan user:42 0 MATCH address:* COUNT 100", 2, -1),
		storeSpec(storeHashesGroup, "hexpire", "hexpire <key> <seconds> FIELDS <count> <field>...", "Set hash-field expiry in seconds", "hexpire user:42 300 FIELDS 1 session", 5, -1),
		storeSpec(storeHashesGroup, "hpexpire", "hpexpire <key> <milliseconds> FIELDS <count> <field>...", "Set hash-field expiry in milliseconds", "hpexpire user:42 1500 FIELDS 1 session", 5, -1),
		storeSpec(storeHashesGroup, "httl", "httl <key> FIELDS <count> <field>...", "Get hash-field expiry in seconds", "httl user:42 FIELDS 1 session", 4, -1),
		storeSpec(storeHashesGroup, "hpttl", "hpttl <key> FIELDS <count> <field>...", "Get hash-field expiry in milliseconds", "hpttl user:42 FIELDS 1 session", 4, -1),
		storeSpec(storeHashesGroup, "hpersist", "hpersist <key> FIELDS <count> <field>...", "Remove hash-field expiry", "hpersist user:42 FIELDS 1 session", 4, -1),
		storeSpec(storeHashesGroup, "hexpireat", "hexpireat <key> <unix-seconds> FIELDS <count> <field>...", "Set hash-field expiry at a Unix timestamp", "hexpireat user:42 1800000000 FIELDS 1 session", 5, -1),
		storeSpec(storeHashesGroup, "hpexpireat", "hpexpireat <key> <unix-milliseconds> FIELDS <count> <field>...", "Set hash-field expiry at a Unix millisecond timestamp", "hpexpireat user:42 1800000000000 FIELDS 1 session", 5, -1),
		storeSpec(storeHashesGroup, "hexpiretime", "hexpiretime <key> FIELDS <count> <field>...", "Get hash-field expiry as Unix seconds", "hexpiretime user:42 FIELDS 1 session", 4, -1),
		storeSpec(storeHashesGroup, "hpexpiretime", "hpexpiretime <key> FIELDS <count> <field>...", "Get hash-field expiry as Unix milliseconds", "hpexpiretime user:42 FIELDS 1 session", 4, -1),

		// Lists.
		storeSpec(storeListsGroup, "lpush", "lpush <key> <element>...", "Prepend elements to a list", "lpush jobs job-3 job-2 job-1", 2, -1),
		storeSpec(storeListsGroup, "rpush", "rpush <key> <element>...", "Append elements to a list", "rpush jobs job-1 job-2 job-3", 2, -1),
		storeSpec(storeListsGroup, "lpop", "lpop <key> [count]", "Remove elements from the list head", "lpop jobs 1", 1, 2),
		storeSpec(storeListsGroup, "rpop", "rpop <key> [count]", "Remove elements from the list tail", "rpop jobs 1", 1, 2),
		storeSpec(storeListsGroup, "lrange", "lrange <key> <start> <stop>", "Return a list range", "lrange jobs 0 -1", 3, 3),
		storeSpec(storeListsGroup, "llen", "llen <key>", "Get a list length", "llen jobs", 1, 1),
		storeSpec(storeListsGroup, "lindex", "lindex <key> <index>", "Get an element by list index", "lindex jobs 0", 2, 2),
		storeSpec(storeListsGroup, "lset", "lset <key> <index> <element>", "Set an element by list index", "lset jobs 0 urgent-job", 3, 3),
		storeSpec(storeListsGroup, "lrem", "lrem <key> <count> <element>", "Remove matching list elements", "lrem jobs 0 cancelled-job", 3, 3),
		storeSpec(storeListsGroup, "ltrim", "ltrim <key> <start> <stop>", "Keep only a list range", "ltrim jobs 0 999", 3, 3),
		storeSpec(storeListsGroup, "lpos", "lpos <key> <element> [options...]", "Find an element in a list", "lpos jobs job-42", 2, -1),
		storeSpec(storeListsGroup, "linsert", "linsert <key> BEFORE|AFTER <pivot> <element>", "Insert an element relative to a pivot", "linsert jobs BEFORE job-42 urgent-job", 4, 4),
		storeSpec(storeListsGroup, "lmove", "lmove <source> <destination> LEFT|RIGHT LEFT|RIGHT", "Move an element between lists", "lmove pending running LEFT RIGHT", 4, 4),
		storeSpec(storeListsGroup, "rpoplpush", "rpoplpush <source> <destination>", "Move the tail element to another list", "rpoplpush pending running", 2, 2),
		storeSpec(storeListsGroup, "lpushx", "lpushx <key> <element>...", "Prepend only when the list exists", "lpushx jobs urgent-job", 2, -1),
		storeSpec(storeListsGroup, "rpushx", "rpushx <key> <element>...", "Append only when the list exists", "rpushx jobs deferred-job", 2, -1),
		storeSpec(storeListsGroup, "blpop", "blpop <key>... <timeout>", "Block while popping from list heads", "blpop high low 5", 2, -1),
		storeSpec(storeListsGroup, "brpop", "brpop <key>... <timeout>", "Block while popping from list tails", "brpop high low 5", 2, -1),
		storeSpec(storeListsGroup, "blmove", "blmove <source> <destination> LEFT|RIGHT LEFT|RIGHT <timeout>", "Block while moving an element between lists", "blmove pending running LEFT RIGHT 5", 5, 5),
		storeSpec(storeListsGroup, "blmpop", "blmpop <timeout> <numkeys> <key>... LEFT|RIGHT [COUNT count]", "Block while popping from multiple lists", "blmpop 5 2 high low LEFT COUNT 1", 5, -1),

		// Sets.
		storeSpec(storeSetsGroup, "sadd", "sadd <key> <member>...", "Add members to a set", "sadd online user:1 user:2", 2, -1),
		storeSpec(storeSetsGroup, "srem", "srem <key> <member>...", "Remove members from a set", "srem online user:2", 2, -1),
		storeSpec(storeSetsGroup, "smembers", "smembers <key>", "List set members", "smembers online", 1, 1),
		storeSpec(storeSetsGroup, "sismember", "sismember <key> <member>", "Test set membership", "sismember online user:1", 2, 2),
		storeSpec(storeSetsGroup, "scard", "scard <key>", "Count set members", "scard online", 1, 1),
		storeSpec(storeSetsGroup, "srandmember", "srandmember <key> [count]", "Return random set members", "srandmember online 2", 1, 2),
		storeSpec(storeSetsGroup, "spop", "spop <key> [count]", "Remove random set members", "spop lottery 1", 1, 2),
		storeSpec(storeSetsGroup, "sdiff", "sdiff <key>...", "Return the difference between sets", "sdiff all-users blocked-users", 1, -1),
		storeSpec(storeSetsGroup, "sinter", "sinter <key>...", "Return the intersection of sets", "sinter active paid", 1, -1),
		storeSpec(storeSetsGroup, "sunion", "sunion <key>...", "Return the union of sets", "sunion admins operators", 1, -1),
		storeSpec(storeSetsGroup, "sdiffstore", "sdiffstore <destination> <key>...", "Store a set difference", "sdiffstore eligible all-users blocked-users", 2, -1),
		storeSpec(storeSetsGroup, "sinterstore", "sinterstore <destination> <key>...", "Store a set intersection", "sinterstore active-paid active paid", 2, -1),
		storeSpec(storeSetsGroup, "sunionstore", "sunionstore <destination> <key>...", "Store a set union", "sunionstore staff admins operators", 2, -1),
		storeSpec(storeSetsGroup, "sintercard", "sintercard <numkeys> <key>... [LIMIT limit]", "Count a set intersection", "sintercard 2 active paid LIMIT 100", 3, -1),
		storeSpec(storeSetsGroup, "smismember", "smismember <key> <member>...", "Test multiple set members", "smismember online user:1 user:2", 2, -1),
		storeSpec(storeSetsGroup, "smove", "smove <source> <destination> <member>", "Move a member between sets", "smove pending active user:1", 3, 3),
		storeSpec(storeSetsGroup, "sscan", "sscan <key> <cursor> [MATCH pattern] [COUNT count]", "Incrementally scan a set", "sscan online 0 MATCH user:* COUNT 100", 2, -1),

		// Sorted sets.
		storeSpec(storeSortedSetsGroup, "zadd", "zadd <key> [options...] <score> <member>...", "Add members to a sorted set", "zadd leaderboard 100 ada 90 grace", 3, -1),
		storeSpec(storeSortedSetsGroup, "zscore", "zscore <key> <member>", "Get a sorted-set member score", "zscore leaderboard ada", 2, 2),
		storeSpec(storeSortedSetsGroup, "zmscore", "zmscore <key> <member>...", "Get multiple sorted-set scores", "zmscore leaderboard ada grace", 2, -1),
		storeSpec(storeSortedSetsGroup, "zrank", "zrank <key> <member>", "Get a member's ascending rank", "zrank leaderboard ada", 2, 2),
		storeSpec(storeSortedSetsGroup, "zrevrank", "zrevrank <key> <member>", "Get a member's descending rank", "zrevrank leaderboard ada", 2, 2),
		storeSpec(storeSortedSetsGroup, "zrange", "zrange <key> <start> <stop> [WITHSCORES]", "Return a sorted-set range", "zrange leaderboard 0 -1 WITHSCORES", 3, -1),
		storeSpec(storeSortedSetsGroup, "zrevrange", "zrevrange <key> <start> <stop> [WITHSCORES]", "Return a descending sorted-set range", "zrevrange leaderboard 0 9 WITHSCORES", 3, -1),
		storeSpec(storeSortedSetsGroup, "zrangebyscore", "zrangebyscore <key> <min> <max> [options...]", "Return members in a score range", "zrangebyscore leaderboard 80 +inf WITHSCORES", 3, -1),
		storeSpec(storeSortedSetsGroup, "zrevrangebyscore", "zrevrangebyscore <key> <max> <min> [options...]", "Return members in a descending score range", "zrevrangebyscore leaderboard +inf 80 WITHSCORES", 3, -1),
		storeSpec(storeSortedSetsGroup, "zcount", "zcount <key> <min> <max>", "Count members in a score range", "zcount leaderboard 80 +inf", 3, 3),
		storeSpec(storeSortedSetsGroup, "zincrby", "zincrby <key> <increment> <member>", "Increment a sorted-set score", "zincrby leaderboard 10 ada", 3, 3),
		storeSpec(storeSortedSetsGroup, "zpopmin", "zpopmin <key> [count]", "Remove lowest-score members", "zpopmin leaderboard 1", 1, 2),
		storeSpec(storeSortedSetsGroup, "zpopmax", "zpopmax <key> [count]", "Remove highest-score members", "zpopmax leaderboard 1", 1, 2),
		storeSpec(storeSortedSetsGroup, "zrandmember", "zrandmember <key> [count [WITHSCORES]]", "Return random sorted-set members", "zrandmember leaderboard 2 WITHSCORES", 1, 3),
		storeSpec(storeSortedSetsGroup, "zscan", "zscan <key> <cursor> [MATCH pattern] [COUNT count]", "Incrementally scan a sorted set", "zscan leaderboard 0 COUNT 100", 2, -1),
		storeSpec(storeSortedSetsGroup, "zcard", "zcard <key>", "Count sorted-set members", "zcard leaderboard", 1, 1),
		storeSpec(storeSortedSetsGroup, "zrem", "zrem <key> <member>...", "Remove sorted-set members", "zrem leaderboard former-user", 2, -1),

		// Streams.
		storeSpec(storeStreamsGroup, "xadd", "xadd <key> <id> <field> <value>...", "Append an entry to a stream", "xadd events '*' type signup user 42", 4, -1),
		storeSpec(storeStreamsGroup, "xlen", "xlen <key>", "Get a stream length", "xlen events", 1, 1),
		storeSpec(storeStreamsGroup, "xrange", "xrange <key> <start> <end> [COUNT count]", "Return a forward stream range", "xrange events - + COUNT 100", 3, -1),
		storeSpec(storeStreamsGroup, "xrevrange", "xrevrange <key> <end> <start> [COUNT count]", "Return a reverse stream range", "xrevrange events + - COUNT 100", 3, -1),
		storeSpec(storeStreamsGroup, "xread", "xread [options...] STREAMS <key>... <id>...", "Read entries from streams", "xread COUNT 10 STREAMS events 0", 3, -1),
		storeSpec(storeStreamsGroup, "xtrim", "xtrim <key> MAXLEN|MINID [~] <threshold> [LIMIT count]", "Trim a stream", "xtrim events MAXLEN '~' 10000", 3, -1),
		storeSpec(storeStreamsGroup, "xdel", "xdel <key> <id>...", "Delete stream entries", "xdel events 1700000000000-0", 2, -1),
		storeSpec(storeStreamsGroup, "xinfo", "xinfo STREAM <key>", "Inspect a stream", "xinfo STREAM events", 2, -1),
		storeSpec(storeStreamsGroup, "xgroup", "xgroup CREATE <key> <group> <id> [MKSTREAM]", "Manage stream consumer groups", "xgroup CREATE events workers 0 MKSTREAM", 4, -1),
		storeSpec(storeStreamsGroup, "xreadgroup", "xreadgroup GROUP <group> <consumer> [options...] STREAMS <key>... <id>...", "Read through a stream consumer group", "xreadgroup GROUP workers worker-1 COUNT 10 STREAMS events '>'", 7, -1),
		storeSpec(storeStreamsGroup, "xack", "xack <key> <group> <id>...", "Acknowledge stream entries", "xack events workers 1700000000000-0", 3, -1),

		// Generic keys and expiry.
		storeSpec(storeKeysGroup, "del", "del <key>...", "Delete keys", "del session:1 session:2", 1, -1),
		storeSpec(storeKeysGroup, "exists", "exists <key>...", "Count existing keys", "exists user:1 user:2", 1, -1),
		storeSpec(storeKeysGroup, "type", "type <key>", "Get the data type of a key", "type user:42", 1, 1),
		storeSpec(storeKeysGroup, "rename", "rename <key> <new-key>", "Rename a key", "rename draft:42 article:42", 2, 2),
		storeSpec(storeKeysGroup, "renamenx", "renamenx <key> <new-key>", "Rename only when the destination is absent", "renamenx draft:42 article:42", 2, 2),
		storeSpec(storeKeysGroup, "copy", "copy <source> <destination> [REPLACE]", "Copy a key", "copy template:1 document:42 REPLACE", 2, -1),
		storeSpec(storeKeysGroup, "scan", "scan <cursor> [MATCH pattern] [COUNT count] [TYPE type]", "Incrementally scan keys", "scan 0 MATCH user:* COUNT 100", 1, -1),
		storeSpec(storeKeysGroup, "keys", "keys <pattern>", "List keys matching a pattern", "keys user:*", 1, 1),
		storeSpec(storeKeysGroup, "randomkey", "randomkey", "Return a random key", "randomkey", 0, 0),
		storeSpec(storeKeysGroup, "dbsize", "dbsize", "Count keys", "dbsize", 0, 0),
		storeSpec(storeKeysGroup, "expire", "expire <key> <seconds> [condition]", "Set key expiry in seconds", "expire session:42 300", 2, 3),
		storeSpec(storeKeysGroup, "pexpire", "pexpire <key> <milliseconds> [condition]", "Set key expiry in milliseconds", "pexpire lock:42 1500", 2, 3),
		storeSpec(storeKeysGroup, "expireat", "expireat <key> <unix-seconds> [condition]", "Set key expiry at a Unix timestamp", "expireat session:42 1800000000", 2, 3),
		storeSpec(storeKeysGroup, "pexpireat", "pexpireat <key> <unix-milliseconds> [condition]", "Set key expiry at a Unix millisecond timestamp", "pexpireat session:42 1800000000000", 2, 3),
		storeSpec(storeKeysGroup, "ttl", "ttl <key>", "Get key expiry in seconds", "ttl session:42", 1, 1),
		storeSpec(storeKeysGroup, "pttl", "pttl <key>", "Get key expiry in milliseconds", "pttl session:42", 1, 1),
		storeSpec(storeKeysGroup, "persist", "persist <key>", "Remove key expiry", "persist user:42", 1, 1),
		storeSpec(storeKeysGroup, "object", "object <subcommand> <key>", "Inspect key internals", "object ENCODING user:42", 2, -1),
		storeSpec(storeKeysGroup, "wait", "wait <replicas> <timeout-ms>", "Wait for replica acknowledgements", "wait 1 1000", 2, 2),

		// Specialized structures. These intentionally preserve their official
		// Redis module-style wire vocabulary.
		storeSpec(storeSpecializedGroup, "setbit", "setbit <key> <offset> <0|1>", "Set a bitmap bit", "setbit active-days 25 1", 3, 3),
		storeSpec(storeSpecializedGroup, "getbit", "getbit <key> <offset>", "Get a bitmap bit", "getbit active-days 25", 2, 2),
		storeSpec(storeSpecializedGroup, "bitcount", "bitcount <key> [start end [BYTE|BIT]]", "Count set bits", "bitcount active-days", 1, -1),
		storeSpec(storeSpecializedGroup, "bitop", "bitop <operation> <destination> <key>...", "Apply a bitmap operation", "bitop AND both active-a active-b", 3, -1),
		storeSpec(storeSpecializedGroup, "bitpos", "bitpos <key> <bit> [start [end [BYTE|BIT]]]", "Find the first matching bit", "bitpos active-days 1", 2, -1),
		storeSpec(storeSpecializedGroup, "pfadd", "pfadd <key> <element>...", "Add HyperLogLog elements", "pfadd visitors user:1 user:2", 2, -1),
		storeSpec(storeSpecializedGroup, "pfcount", "pfcount <key>...", "Estimate HyperLogLog cardinality", "pfcount visitors", 1, -1),
		storeSpec(storeSpecializedGroup, "pfmerge", "pfmerge <destination> <source>...", "Merge HyperLogLogs", "pfmerge visitors:all visitors:a visitors:b", 2, -1),
		storeSpec(storeSpecializedGroup, "geoadd", "geoadd <key> [options...] <longitude> <latitude> <member>...", "Add geospatial members", "geoadd cities 34.78 32.08 tel-aviv", 4, -1),
		storeSpec(storeSpecializedGroup, "geodist", "geodist <key> <member1> <member2> [unit]", "Measure distance between members", "geodist cities tel-aviv haifa km", 3, 4),
		storeSpec(storeSpecializedGroup, "geopos", "geopos <key> <member>...", "Get geospatial positions", "geopos cities tel-aviv", 2, -1),
		storeSpec(storeSpecializedGroup, "geohash", "geohash <key> <member>...", "Get geospatial hashes", "geohash cities tel-aviv", 2, -1),
		storeSpec(storeSpecializedGroup, "geosearch", "geosearch <key> <origin and shape...>", "Search geospatial members", "geosearch cities FROMLONLAT 34.78 32.08 BYRADIUS 50 km", 2, -1),
		storeSpec(storeSpecializedGroup, "bf.add", "bf.add <key> <item>", "Add an item to a Bloom filter", "bf.add seen event:42", 2, 2),
		storeSpec(storeSpecializedGroup, "bf.reserve", "bf.reserve <key> <error-rate> <capacity>", "Initialize a Bloom filter", "bf.reserve seen 0.01 100000", 3, 3),
		storeSpec(storeSpecializedGroup, "bf.exists", "bf.exists <key> <item>", "Test a Bloom filter item", "bf.exists seen event:42", 2, 2),
		storeSpec(storeSpecializedGroup, "bf.madd", "bf.madd <key> <item>...", "Add Bloom filter items", "bf.madd seen event:1 event:2", 2, -1),
		storeSpec(storeSpecializedGroup, "bf.mexists", "bf.mexists <key> <item>...", "Test Bloom filter items", "bf.mexists seen event:1 event:2", 2, -1),
		storeSpec(storeSpecializedGroup, "bf.card", "bf.card <key>", "Get Bloom filter cardinality", "bf.card seen", 1, 1),
		storeSpec(storeSpecializedGroup, "bf.info", "bf.info <key>", "Inspect a Bloom filter", "bf.info seen", 1, 1),
		storeSpec(storeSpecializedGroup, "cf.add", "cf.add <key> <item>", "Add an item to a Cuckoo filter", "cf.add seen event:42", 2, 2),
		storeSpec(storeSpecializedGroup, "cf.reserve", "cf.reserve <key> <capacity>", "Initialize a Cuckoo filter", "cf.reserve seen 100000", 2, 2),
		storeSpec(storeSpecializedGroup, "cf.addnx", "cf.addnx <key> <item>", "Add a Cuckoo item only when absent", "cf.addnx seen event:42", 2, 2),
		storeSpec(storeSpecializedGroup, "cf.exists", "cf.exists <key> <item>", "Test a Cuckoo filter item", "cf.exists seen event:42", 2, 2),
		storeSpec(storeSpecializedGroup, "cf.mexists", "cf.mexists <key> <item>...", "Test multiple Cuckoo filter items", "cf.mexists seen event:1 event:2", 2, -1),
		storeSpec(storeSpecializedGroup, "cf.del", "cf.del <key> <item>", "Delete a Cuckoo filter item", "cf.del seen event:42", 2, 2),
		storeSpec(storeSpecializedGroup, "cf.count", "cf.count <key> <item>", "Count a Cuckoo filter item", "cf.count seen event:42", 2, 2),
		storeSpec(storeSpecializedGroup, "cf.info", "cf.info <key>", "Inspect a Cuckoo filter", "cf.info seen", 1, 1),
		storeSpec(storeSpecializedGroup, "cms.initbyprob", "cms.initbyprob <key> <error> <probability>", "Initialize a Count-Min Sketch", "cms.initbyprob frequencies 0.001 0.99", 3, 3),
		storeSpec(storeSpecializedGroup, "cms.initbydim", "cms.initbydim <key> <width> <depth>", "Initialize a Count-Min Sketch by dimensions", "cms.initbydim frequencies 2000 7", 3, 3),
		storeSpec(storeSpecializedGroup, "cms.incrby", "cms.incrby <key> <item> <increment>...", "Increment Count-Min Sketch counters", "cms.incrby frequencies apple 1 pear 2", 3, -1),
		storeSpec(storeSpecializedGroup, "cms.query", "cms.query <key> <item>...", "Query Count-Min Sketch counters", "cms.query frequencies apple pear", 2, -1),
		storeSpec(storeSpecializedGroup, "cms.merge", "cms.merge <destination> <numkeys> <key>... [WEIGHTS weight...]", "Merge Count-Min Sketches", "cms.merge frequencies:all 2 frequencies:a frequencies:b", 3, -1),
		storeSpec(storeSpecializedGroup, "cms.info", "cms.info <key>", "Inspect a Count-Min Sketch", "cms.info frequencies", 1, 1),
		storeSpec(storeSpecializedGroup, "topk.reserve", "topk.reserve <key> <k> [width depth]", "Initialize a Top-K structure", "topk.reserve popular 10", 2, 4),
		storeSpec(storeSpecializedGroup, "topk.add", "topk.add <key> <item>...", "Add Top-K observations", "topk.add popular apple pear", 2, -1),
		storeSpec(storeSpecializedGroup, "topk.incrby", "topk.incrby <key> <item> <increment>...", "Increment Top-K observations", "topk.incrby popular apple 2", 3, -1),
		storeSpec(storeSpecializedGroup, "topk.query", "topk.query <key> <item>...", "Test Top-K membership", "topk.query popular apple pear", 2, -1),
		storeSpec(storeSpecializedGroup, "topk.list", "topk.list <key> [WITHCOUNT]", "List Top-K items", "topk.list popular WITHCOUNT", 1, 2),
		storeSpec(storeSpecializedGroup, "topk.info", "topk.info <key>", "Inspect a Top-K structure", "topk.info popular", 1, 1),
		storeSpec(storeSpecializedGroup, "tdigest.create", "tdigest.create <key> [COMPRESSION value]", "Initialize a t-digest", "tdigest.create latency COMPRESSION 100", 1, -1),
		storeSpec(storeSpecializedGroup, "tdigest.add", "tdigest.add <key> <value>...", "Add t-digest observations", "tdigest.add latency 10.5 12.0 20.1", 2, -1),
		storeSpec(storeSpecializedGroup, "tdigest.reset", "tdigest.reset <key>", "Reset a t-digest while preserving compression", "tdigest.reset latency", 1, 1),
		storeSpec(storeSpecializedGroup, "tdigest.quantile", "tdigest.quantile <key> <quantile>...", "Query t-digest quantiles", "tdigest.quantile latency 0.5 0.95 0.99", 2, -1),
		storeSpec(storeSpecializedGroup, "tdigest.cdf", "tdigest.cdf <key> <value>...", "Query t-digest cumulative distribution", "tdigest.cdf latency 20", 2, -1),
		storeSpec(storeSpecializedGroup, "tdigest.rank", "tdigest.rank <key> <value>...", "Query t-digest ranks", "tdigest.rank latency 20 50", 2, -1),
		storeSpec(storeSpecializedGroup, "tdigest.revrank", "tdigest.revrank <key> <value>...", "Query reverse t-digest ranks", "tdigest.revrank latency 20 50", 2, -1),
		storeSpec(storeSpecializedGroup, "tdigest.byrank", "tdigest.byrank <key> <rank>...", "Get t-digest values by rank", "tdigest.byrank latency 10 100", 2, -1),
		storeSpec(storeSpecializedGroup, "tdigest.byrevrank", "tdigest.byrevrank <key> <rank>...", "Get t-digest values by reverse rank", "tdigest.byrevrank latency 10 100", 2, -1),
		storeSpec(storeSpecializedGroup, "tdigest.trimmed_mean", "tdigest.trimmed_mean <key> <low-quantile> <high-quantile>", "Compute a t-digest trimmed mean", "tdigest.trimmed_mean latency 0.1 0.9", 3, 3),
		storeSpec(storeSpecializedGroup, "tdigest.min", "tdigest.min <key>", "Get the minimum t-digest value", "tdigest.min latency", 1, 1),
		storeSpec(storeSpecializedGroup, "tdigest.max", "tdigest.max <key>", "Get the maximum t-digest value", "tdigest.max latency", 1, 1),
		storeSpec(storeSpecializedGroup, "tdigest.info", "tdigest.info <key>", "Inspect a t-digest", "tdigest.info latency", 1, 1),
		storeSpec(storeSpecializedGroup, "tdigest.merge", "tdigest.merge <destination> <numkeys> <key>... [COMPRESSION value] [OVERRIDE]", "Merge t-digests", "tdigest.merge latency:all 2 latency:a latency:b OVERRIDE", 4, -1),
		storeSpec(storeSpecializedGroup, "geosearchstore", "geosearchstore <destination> <source> <origin and shape...> [STOREDIST]", "Store a geospatial search result", "geosearchstore nearby cities FROMLONLAT 34.78 32.08 BYRADIUS 50 km", 4, -1),

		// FerricStore-native data operations.
		storeSpec(storeNativeGroup, "cas", "cas <key> <expected> <value> [EX seconds]", "Compare and swap a value", "cas version:42 7 8 EX 300", 3, 5),
		storeSpec(storeNativeGroup, "lock", "lock <key> <owner> <ttl-ms>", "Acquire a distributed lock", "lock order:42 worker-1 30000", 3, 3),
		storeSpec(storeNativeGroup, "unlock", "unlock <key> <owner>", "Release a distributed lock", "unlock order:42 worker-1", 2, 2),
		storeSpec(storeNativeGroup, "extend", "extend <key> <owner> <ttl-ms>", "Extend a distributed lock", "extend order:42 worker-1 30000", 3, 3),
		storeSpec(storeNativeGroup, "ratelimit.add", "ratelimit.add <key> <window-ms> <max-count> [count]", "Consume a rate-limit allowance", "ratelimit.add api:user:42 60000 100 1", 3, 4),
		storeSpec(storeNativeGroup, "fetch_or_compute", "fetch_or_compute <key> <ttl-ms> [hint]", "Get a value or reserve its computation", "fetch_or_compute report:42 30000 worker-1", 2, -1),
		storeSpec(storeNativeGroup, "fetch_or_compute_result", "fetch_or_compute_result <key> <token> <value> <ttl-ms>", "Publish a fetch-or-compute result", "fetch_or_compute_result report:42 token result 30000", 4, 4),
		storeSpec(storeNativeGroup, "fetch_or_compute_error", "fetch_or_compute_error <key> <token> <message>", "Publish a fetch-or-compute error", "fetch_or_compute_error report:42 token failed", 3, 3),
	}
}
