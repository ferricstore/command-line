//go:build integration

package cli

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestIntegrationOSSStoreDataCommands runs each store helper added for the
// server's newer data commands against a live FerricStore.
func TestIntegrationOSSStoreDataCommands(t *testing.T) {
	if os.Getenv("FERRICSTORE_OSS_CLI_TEST") != "1" {
		t.Skip("OSS CLI integration is run by scripts/integration-login-oss.sh")
	}
	options := ossIntegrationOptions(t)
	// One hash tag keeps every key in one slot for the multi-key commands.
	prefix := "{cli-store-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "}:"
	key := func(name string) string { return prefix + name }
	store := func(args ...string) string {
		t.Helper()
		return strings.TrimSpace(runIntegrationCLI(t, options, append([]string{"store"}, args...)...))
	}
	expect := func(want string, args ...string) {
		t.Helper()
		if got := store(args...); !strings.Contains(compactJSON(got), want) {
			t.Fatalf("ferric store %s = %s; want it to contain %s", strings.Join(args, " "), got, want)
		}
	}

	// XCLAIM, XAUTOCLAIM, BITFIELD and others need feature level 11 or 12; a
	// fresh single node starts lower until an operator raises it.
	runIntegrationCLI(t, options, "cluster", "features", "raise", "12", "--yes")

	store("zadd", key("za"), "1", "a", "2", "b", "3", "c")
	store("zadd", key("zb"), "2", "b", "3", "c", "4", "d")
	expect(`"d"`, "zunion", "2", key("za"), key("zb"))
	expect("4", "zunionstore", key("zunion"), "2", key("za"), key("zb"))
	expect(`["b","c"]`, "zinter", "2", key("za"), key("zb"))
	expect("2", "zinterstore", key("zinter"), "2", key("za"), key("zb"))
	expect("2", "zintercard", "2", key("za"), key("zb"))
	expect(`["a"]`, "zdiff", "2", key("za"), key("zb"))
	expect("1", "zdiffstore", key("zdiff"), "2", key("za"), key("zb"))
	expect("2", "zrangestore", key("ztop"), key("za"), "0", "1")

	store("zadd", key("names"), "0", "a", "0", "b", "0", "c", "0", "d")
	expect("2", "zlexcount", key("names"), "[a", "(c")
	expect(`["a","b","c","d"]`, "zrangebylex", key("names"), "-", "+")
	expect(`["d","c"]`, "zrevrangebylex", key("names"), "+", "-", "LIMIT", "0", "2")
	expect("1", "zremrangebylex", key("names"), "[a", "[a")
	expect("1", "zremrangebyrank", key("za"), "0", "0")
	expect("1", "zremrangebyscore", key("zb"), "-inf", "2")
	expect(`"b"`, "zmpop", "2", key("za"), key("zb"), "MIN")
	expect(`"c"`, "bzpopmin", key("zb"), "1")
	expect(`"d"`, "bzpopmax", key("zb"), "1")
	expect(`"c"`, "bzmpop", "1", "1", key("za"), "MAX")

	store("rpush", key("list"), "x", "y")
	expect(`"x"`, "lmpop", "1", key("list"), "LEFT")

	store("set", key("s1"), "ohmytext")
	store("set", key("s2"), "mynewtext")
	expect(`"mytext"`, "lcs", key("s1"), key("s2"))

	store("hset", key("hash"), "f1", "v1", "f2", "v2")
	expect(`"v1"`, "hgetdel", key("hash"), "FIELDS", "1", "f1")
	expect("1", "hsetex", key("hash"), "EX", "300", "FIELDS", "1", "f3", "v3")
	expect(`"v3"`, "hgetex", key("hash"), "PERSIST", "FIELDS", "1", "f3")

	entry := strings.Trim(store("xadd", key("stream"), "*", "field", "1"), `"`)
	store("xgroup", "CREATE", key("stream"), "workers", "0")
	store("xreadgroup", "GROUP", "workers", "worker-1", "STREAMS", key("stream"), ">")
	expect(`"worker-1"`, "xpending", key("stream"), "workers")
	expect(entry, "xclaim", key("stream"), "workers", "worker-2", "0", entry)
	expect(entry, "xautoclaim", key("stream"), "workers", "worker-3", "0", "0-0")
	expect("OK", "xsetid", key("stream"), "9999999999999-0")

	store("set", key("expiring"), "v", "EX", "300")
	expectPositive(t, store("expiretime", key("expiring")))
	expectPositive(t, store("pexpiretime", key("expiring")))
	expect("1", "touch", key("expiring"))
	expect("1", "unlink", key("expiring"))

	store("rpush", key("numbers"), "3", "1", "2")
	expect(`["1","2","3"]`, "sort", key("numbers"))
	expect(`["3","2","1"]`, "sort_ro", key("numbers"), "DESC")

	expect("5", "bitfield", key("bits"), "INCRBY", "u8", "0", "5")
	expect("5", "bitfield_ro", key("bits"), "GET", "u8", "0")

	expect("1", "bf.insert", key("bloom"), "ITEMS", "event:1")
	expect("1", "cf.insert", key("cuckoo"), "ITEMS", "event:1")
	expect("0", "cf.insertnx", key("cuckoo"), "ITEMS", "event:1")
	store("topk.reserve", key("topk"), "3")
	store("topk.add", key("topk"), "apple", "apple")
	expect("2", "topk.count", key("topk"), "apple")
}

func compactJSON(output string) string {
	var value any
	if err := json.Unmarshal([]byte(output), &value); err != nil {
		return output
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return output
	}
	return string(encoded)
}

func expectPositive(t *testing.T, output string) {
	t.Helper()
	value, err := strconv.ParseInt(strings.Trim(output, `"`), 10, 64)
	if err != nil || value <= 0 {
		t.Fatalf("expected a positive integer, got %s (%v)", output, err)
	}
}
