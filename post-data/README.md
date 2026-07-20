# post-data

Post one or more objects to MinIO with a custom timestamp and optional metadata.

It's meant for generating test/sample data against a MinIO cluster: seeding a
bucket with objects dated in the past, backfilling metadata for lifecycle/ILM
testing, or bulk-loading random-sized objects for load testing.

## Build

```
make build
```

## Usage

```
post-data --alias myminio -o bucket/path/to/key -d "hello world"
```

The object (`-o`) is always `bucket/key`. When using `--alias`, you don't
need to repeat the alias name in `-o` - `mybucket/key` is enough. If you do
include it (`myalias/mybucket/key`), the alias prefix is recognized and
stripped automatically, so both forms behave identically.

If `--data` is omitted, content is read from stdin:

```
echo "hello world" | post-data -o bucket/path/to/key
```

## Options

| Flag             | Short | Default      | Description                                                                          |
| ---------------- | ----- | ------------ | ------------------------------------------------------------------------------------ |
| `--alias`        |       |              | mc alias name (reads endpoint/credentials from `~/.mc/config.json`)                  |
| `--endpoint`     | `-e`  |              | MinIO endpoint (host:port or full URL)                                               |
| `--access-key`   | `-a`  |              | MinIO access key                                                                     |
| `--secret-key`   | `-s`  |              | MinIO secret key                                                                     |
| `--object`       | `-o`  |              | bucket/object key, optionally prefixed with `alias/` (required)                      |
| `--data`         | `-d`  |              | Data content (text string); reads stdin if omitted                                   |
| `--timestamp`    | `-t`  | `now`        | Timestamp: `now`, `-Nd` (N days ago), `+Nd`/`Nd` (N days from now), or RFC3339       |
| `--meta`         | `-m`  |              | Metadata as `key=value` (repeatable)                                                 |
| `--content-type` |       | `text/plain` | Content-Type of the object                                                           |
| `--versioning`   |       | `true`       | Enable versioning when auto-creating a bucket                                        |
| `--count`        | `-n`  | `1`          | Number of objects to post (>1 appends a sequential suffix to the object key)         |
| `--random-data`  |       | `false`      | Fill each object with random bytes instead of using `--data`                         |
| `--max-size`     |       | `1KiB`       | Maximum size for random data, e.g. `512`, `4KiB`, `2MiB` (used with `--random-data`) |
| `--name-prefix`  |       |              | Prefix prepended to each generated object name, e.g. `log-` → `log-001`, `log-002`   |
| `--insecure`     |       | `false`      | Force HTTP (overrides alias URL scheme)                                              |

## Notes

- **Credentials**: pass `--endpoint`/`--access-key`/`--secret-key` explicitly,
  or use `--alias` to resolve them from `~/.mc/config.json` (set
  `MC_CONFIG_DIR` to point elsewhere). Any explicit flag overrides the value
  from the alias. `--insecure` forces HTTP regardless of the alias's scheme.
- **Bucket auto-creation**: if the target bucket doesn't exist, it's created
  automatically, with versioning enabled unless `--versioning=false`.
- **Multiple objects** (`--count`/`-n`): the object key from `-o` is treated as
  a prefix, and each object gets a zero-padded sequence number (width based on
  the total count), optionally preceded by `--name-prefix`. Example: `-o
mybucket/logs/ -n 100 --name-prefix log-` produces `logs/log-001` ...
  `logs/log-100`.
- **Random data** (`--random-data`): each object gets its own random size,
  uniformly chosen between 1 byte and `--max-size`, and random byte content.
  Useful for load/capacity testing without a fixed payload.

## Timestamp

`--timestamp`/`-t` is the reason this tool exists: it lets you post an object
that looks like it was written at some other point in time, which is useful
for testing lifecycle rules, tiering, replication, or anything else that
reads an object's age.

### Accepted formats

| Value       | Meaning                                                       |
| ----------- | ------------------------------------------------------------- |
| `now`       | Current time (the default)                                    |
| `-Nd`       | N days in the past, e.g. `-7d` = 7 days ago                   |
| `Nd`, `+Nd` | N days in the future, e.g. `30d` or `+30d` = 30 days from now |
| RFC3339     | An exact timestamp, e.g. `2024-01-15T10:00:00Z`               |

### Where the timestamp goes

Each upload carries the resolved timestamp in two different places:

1. **`SourceMTime`** - a low-level PutObject option MinIO uses internally
   (e.g. for replication/mirroring source timestamps), sent over the wire as
   the `X-Minio-Source-Mtime` request header. It isn't exposed as ordinary
   object metadata, so you can't query it back directly.
2. **`x-posted-timestamp`** - an ordinary user-metadata key this tool adds to
   every object, holding the same timestamp in RFC3339/UTC. This is the one
   you'll actually see with `mc stat` or an S3 `HeadObject`, so it's the
   practical way to confirm what timestamp was posted.

### Batch behavior (`--count` > 1)

Whether every object in a batch gets the _same_ timestamp or each gets its
_own_ depends on whether you passed `--timestamp` explicitly:

- **`--timestamp` omitted** (default): the flag is left at `now`, so each
  object is stamped with a fresh, live timestamp at the moment it's
  individually uploaded. Object 1 and object 100 will have slightly different
  timestamps, a few milliseconds/seconds apart.
- **`--timestamp` passed explicitly** (including `-t now`): the timestamp is
  resolved once, before the upload loop starts, and every object in the batch
  shares that exact same value.

## Examples

Post a single object using credentials directly:

```
post-data -e localhost:9000 -a minioadmin -s minioadmin -o mybucket/hello.txt -d "hello"
```

Post using an mc alias, with metadata and a backdated timestamp:

```
post-data --alias myminio -o mybucket/hello.txt -d "hello" -t -7d -m env=test -m author=jkandasa
```

Post an object with an exact, pinned timestamp:

```
post-data --alias myminio -o mybucket/hello.txt -d "hello" -t 2024-01-15T10:00:00Z
```

Post an object dated 30 days in the future (e.g. to test a future-dated
lifecycle transition):

```
post-data --alias myminio -o mybucket/hello.txt -d "hello" -t +30d
```

Post 100 random-sized objects with a common name prefix, each stamped with
its own live "now" timestamp:

```
post-data --alias myminio -o mybucket/logs/ -n 100 --random-data --max-size 4KiB --name-prefix log-
```

Post 50 objects that all share one fixed timestamp, 90 days in the past, by
passing `--timestamp` explicitly:

```
post-data --alias myminio -o mybucket/logs/ -n 50 --random-data --name-prefix log- -t -90d
```

Verify the posted timestamp afterward:

```
mc stat myminio/mybucket/hello.txt
# look for the X-Amz-Meta-X-Posted-Timestamp entry under "Metadata"
```
