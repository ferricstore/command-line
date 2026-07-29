# Ferric CLI command design

`ferric` is a non-interactive, one-command-per-process CLI. It borrows familiar
command vocabulary from existing tools without copying their implementations:

- Redis users get the command names they already know under `ferric store`.
- Queue users get `enqueue`, `claim`, and `complete`, with the familiar aliases
  `send`, `receive`, and `ack`.
- Workflow users get `start`, `describe`, `list`, `history`, `signal`, and
  lifecycle commands in the same shape as modern workflow CLIs.
- Service groups follow the `tool service operation` shape used by cloud CLIs.

An interactive Redis/SQL-style shell is intentionally outside the current
scope. Every operation must work well in scripts, CI, and a normal terminal.

## Global behavior

~~~text
ferric [--output auto|json|raw] [--timeout 10s] <service> <operation>
~~~

The selected connection is resolved in this order: hidden `--profile`, a
complete direct environment credential set, `FERRIC_PROFILE`, the profile
selected by `ferric profile use`, then `default`. Most users therefore never
need to know about profiles. Direct environment credentials are atomic and
ephemeral; missing values never fall back to saved metadata or the keyring.

`--output auto` prints scalar results directly and structured results as
indented JSON. `--output json` always emits valid JSON. `--output raw` emits
scalars one per line for shell pipelines. Errors go to stderr and successful
data goes to stdout.

Every network command uses the same timeout, saved-credential resolution,
connection lifecycle, output encoding, and error context. A command opens one
authenticated Go SDK client, performs its work, and closes it. The OS keyring
is read only when the selected authentication provider requires a secret.

Payload-bearing commands accept a positional value or `--file <path>`;
`--file -` reads stdin. `--json` parses the input before sending it. Repeated
metadata uses `--attribute key=value` and may also be supplied as JSON.

## Authentication and local connections

~~~text
ferric auth login
ferric auth status
ferric auth logout

ferric profile list
ferric profile show <name>
ferric profile use <name>
ferric profile delete <name>
~~~

Profile files contain only non-secret metadata. OSS passwords, Enterprise
renewable sessions, and machine API tokens belong in the operating-system
keystore. Enterprise providers are integration boundaries in this public
repository and are exercised with mocks; live Enterprise tests belong in the
Enterprise repository.

CI and containers can bypass persistence with `FERRIC_URL`, `FERRIC_USERNAME`,
and `FERRIC_PASSWORD` or `FERRIC_PASSWORD_FILE`. Enterprise machine builds use
`FERRIC_CONTROL_URL`, `FERRIC_ORGANIZATION`, `FERRIC_CLUSTER`, and
`FERRIC_API_TOKEN` or `FERRIC_API_TOKEN_FILE`. Secret-file variants are
preferred for Docker and Kubernetes mounts.

## Store

The common path preserves Redis command vocabulary while keeping it inside a
FerricStore service namespace:

~~~text
ferric store get user:42
ferric store set user:42 '{"name":"Ada"}'
ferric store hget user:42 name
ferric store lpush jobs job-3 job-2 job-1
ferric store smembers online-users
ferric store zrange leaderboard 0 9 WITHSCORES
ferric store xread STREAMS events 0
~~~

Helpers cover FerricStore's implemented string, hash, list, set, sorted-set,
stream, expiry, bitmap, HyperLogLog, geo, Bloom, Cuckoo, Count-Min Sketch,
Top-K, and t-digest commands. Ferric-native helpers cover compare-and-swap,
locks, rate limiting, fetch-or-compute, and key inspection.

`ferric store command <name> [args...]` is the explicit escape hatch for a new
or specialized server command that does not yet have a polished helper. It is
not a shell and executes exactly one SDK command. Safety-sensitive raw commands
are classified before a connection opens and require `--yes`, for example
`ferric store command FLUSHDB --yes`. For safe commands, a trailing `--yes`
remains an ordinary protocol argument.

Atomic batches use a JSON file or stdin, keeping MULTI/EXEC scriptable without
adding a shell:

~~~text
ferric store transaction commands.json --key shared-slot-key
ferric store transaction - --watch account:42
ferric store transaction administrative-commands.json --yes
~~~

A transaction containing a safety-sensitive administrative command is rejected
before connecting unless `--yes` is present.

## Queue

Queues are the job-oriented view of FerricFlow. The primary vocabulary is:

~~~text
ferric queue enqueue <type> <id> [payload]       # alias: send
ferric queue claim <type> --worker <name>        # alias: receive
ferric queue describe <id>                       # aliases: get, inspect
ferric queue list <type> --partition <key>
ferric queue extend <id> --lease-token ... --fencing-token ...
ferric queue complete <id> --lease-token ... --fencing-token ... # alias: ack
ferric queue retry <id> --lease-token ... --fencing-token ...    # alias: nack
ferric queue fail <id> --lease-token ... --fencing-token ...
ferric queue cancel <id> --fencing-token ...
ferric queue history <id>
ferric queue stats <type>
ferric queue policy get <type>
ferric queue policy set <type> [policy flags]
~~~

`claim` returns the job ID, payload, lease token, fencing token, state, and
partition. The lease and fencing token are deliberately explicit on mutation
commands: together they are Ferric's equivalent of a queue receipt handle and
prevent stale workers from acknowledging newer work.

Queue aliases improve discoverability but help and documentation use Ferric's
native lifecycle terms first.

## Workflow

Workflows expose the full stateful view of FerricFlow:

~~~text
ferric workflow start <type> <id> [payload]
ferric workflow describe <id>                    # aliases: get, inspect
ferric workflow list <type> --partition <key>
ferric workflow search --partition <key> --attribute <name=value> [filters]
ferric workflow query <fql> [--param name=value]
ferric workflow query explain <fql> [--analyze]
ferric workflow query indexes [index-id]
ferric workflow history <id>
ferric workflow signal <id> <signal>
ferric workflow claim <type> --state <state> --worker <name>
ferric workflow transition <id> <from> <to> --lease-token ... --fencing-token ...
ferric workflow complete|retry|fail|cancel <id> [lease flags]
ferric workflow rewind <id> --to-event <event-id>
ferric workflow children <id> --partition <key>
ferric workflow values get <ref>...
ferric workflow policy get|set <type>
~~~

The workflow commands share payload, partition, attributes, time, lease, and
fencing flags with the queue view so users do not have to learn two syntaxes.
Queue-created jobs can be inspected and managed through workflow commands.

FQL-backed collection helpers (`list`, lineage, terminal, failure, and stuck
reads) require an exact `--partition`. `workflow search` additionally requires
at least one `--attribute name=value` or `--state-meta state.name=value`
predicate. These helpers intentionally do not expose cold-projection flags,
because FQL collection paths operate on the bounded live query contract.

`workflow query` is the direct FQL1 surface. The query may be supplied as one
argument or with `--file <path>`; `--file -` reads stdin. Repeated `--param`
values are strings. `--params-json` and `--params-file` accept a JSON object of
typed string, boolean, and numeric values. Query output includes records or the
count result together with page, quality, and resource-usage contracts.
`query explain --analyze` executes an admitted plan without returning records,
and `query indexes` reports the OSS catalog, lifecycle, validation, and
statistics status.

## Workflow schedules

Schedules live under `workflow` because every fire creates or targets a
FerricFlow workflow execution. They remain their own nested group because
operators manage schedule definitions independently from individual runs:

~~~text
ferric workflow schedule create <id> [target and timing flags]
ferric workflow schedule describe <id>
ferric workflow schedule list
ferric workflow schedule pause <id>
ferric workflow schedule resume <id>
ferric workflow schedule trigger <id>                     # alias: fire
ferric workflow schedule fire-due --worker <name>
ferric workflow schedule delete <id> --yes
~~~

Interval schedules may use `--catchup fire_once` to coalesce elapsed
occurrences into one recovery fire. Schedule output includes recurrence,
overlap, catch-up, counters, last outcome, and next-run information from the
typed schedule contract.

## Server and operations

~~~text
ferric server ping [message]
ferric server info [section]
ferric server capabilities
ferric server metrics
ferric server health
ferric server stats
ferric server key-info <key>
ferric server doctor check [--scope <scope>]
ferric server doctor status <job-id>
ferric server doctor list
ferric server doctor cancel <job-id>

ferric server config get|set|get-local|set-local ...
ferric server slowlog get|length|reset
ferric server client id|info|list|tracking-info|redirect
ferric server persistence save|background-save|last-save
ferric server flush --yes
~~~

Cluster, ACL, namespace, quota, and Pub/Sub operations use their own top-level
service groups:

~~~text
ferric cluster health|status|role|stats|slots
ferric cluster keyslot <key>
ferric cluster join|leave|failover|promote|demote ... --yes

ferric acl whoami|list
ferric acl get-user|set-user|delete-user ...
ferric namespace usage <prefix>
ferric quota usage <namespace>
ferric pubsub publish|channels|subscribers|patterns ...
~~~

Workflow governance is nested with the executions it controls:

~~~text
ferric workflow governance overview
ferric workflow governance approval request|get|list|approve|reject
ferric workflow governance circuit get|open|close
ferric workflow governance effect get|reserve|confirm|fail|compensate
ferric workflow governance budget get|list|reserve|commit|release
ferric workflow governance limit get|list|lease|spend|release
~~~

Destructive and safety-sensitive administrative operations require `--yes`;
the CLI never silently confirms an action merely because stdin is redirected.

## Help and completion contract

Every service and operation supports `--help`, contains at least one runnable
example, documents defaults and safety-sensitive fields, and validates inputs
before opening a connection. Cobra provides completion scripts for Bash, Zsh,
Fish, and PowerShell. Completion suggests service names, operations, enum flag
values, and saved profile names where applicable; it never reads secrets.

## Interface references

The command vocabulary was compared with official interfaces that users are
likely to know. These sources informed naming and ergonomics only; no source
code was copied:

- [Redis CLI](https://redis.io/docs/latest/develop/tools/cli/) for one-shot
  data commands, raw pipeline output, help, and completion expectations.
- [RabbitMQ management CLI](https://www.rabbitmq.com/docs/management-cli) and
  [AWS SQS CLI examples](https://docs.aws.amazon.com/cli/latest/userguide/cli_sqs_code_examples.html)
  for queue service/action vocabulary and receipt-like acknowledgements.
- [Temporal CLI](https://github.com/temporalio/cli) for workflow
  start/list/describe/history/signal and schedule vocabulary.
