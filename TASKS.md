# sawdust — tasks

A Parquet reader built from the bytes up, in stages.

**The method:** each stage names the wrong assumption you are likely to hold
going in (**Pain**), the mechanism that corrects it (**Concept**), targeted
**Reading**, and a **Done when** check against an oracle (DuckDB, or bytes you
computed by hand). Build each stage's naive version first where the stage says
so — the failure is the lesson.

**Scaffolding fades on purpose.** Stages 0–5 are fully specified. Stage 6 gives
you a goal and a Done-when; you write the task breakdown. Stage 7 is entirely
yours: the public API, the versioning story, the docs. By then you will know
the domain well enough that a blank page is the right tool.

**Every stage starts with a paper sketch** — types, functions, one sentence
each — reviewed before you write code.

---

## Stage 0 — The file, backwards

Goal: locate every structural landmark in a Parquet file using nothing but
byte offsets. No parsing of contents yet.

### Build the fixtures first

- [X] `go mod init github.com/sevaergdm/sawdust`.
- [X] `cmd/genfix`: a generator that writes fixtures with
      `github.com/parquet-go/parquet-go` into `testdata/`. Use a struct with
      deliberately varied columns: a required `int64`, an optional `*int64`, a
      required `string`, an optional `*string`, a low-cardinality `string`
      (repeats a handful of values), a `float64`, a
      `time.Time` tagged `timestamp(microsecond)`, and a `bool`.
**Only generate what the next stage needs.** Later stages add their own
fixtures, at the point where you know why they exist. Four files are enough to
start:

- [X] `basic.parquet` — 100 rows, whatever options parquet-go picks by default.
      This is the file stages 0, 1 and 2 read.
- [X] `empty.parquet` — 0 rows. A file with a schema and no data. This is the
      case that finds guards placed where they cannot fire.
- [X] `single_row.parquet` — 1 row. The other end of the same test.
- [X] `many_rows.parquet` — a file cut into more than one row group, so stage 3
      has something to count. Measured 2026-08-20: row count does NOT cause a
      split. parquet-go's `DefaultMaxRowsPerRowGroup` is `math.MaxInt64`, so
      500,000 rows still produced one row group in a 22 MB file. The split is a
      writer decision: `parquet.MaxRowsPerRowGroup(n)`, or `writer.Flush()`
      between batches. Set it explicitly and keep the file small — a few
      hundred rows in groups of 100 is the same test in a few KB.
- [X] Values must be synthetic and boring (`"row0000"`, sequential ints). No
      real hostnames, paths, or log text — this repo is public.
- [X] Record what parquet-go actually chose for `basic.parquet`, in the README:
      `SELECT path_in_schema, compression, encodings FROM
      parquet_metadata('testdata/basic.parquet');` You are not controlling these
      yet. You are writing down the defaults so that later, when you do control
      them, you can see what changed.
- [X] Deterministic output: run the generator twice into two different
      directories and compare the bytes (`cmp` / `sha256sum`). They must be
      identical. Thereafter `go run ./cmd/genfix -out testdata && git status
      --short` must print nothing.
- [X] Commit the generator AND its output. Note in the generator's doc comment
      which parquet-go version produced the files.

### Build the reader skeleton

- [X] Open a fixture. Verify the first 4 bytes are `PAR1`; error clearly if not.
- [X] Read the last 8 bytes. The final 4 are the trailing magic; the 4 before
      them are the footer length as a little-endian `uint32`.
- [X] Compute the metadata byte range: it *ends* 8 bytes before EOF and is
      `footerLen` bytes long. Print `filesize`, `footerLen`, `metadataStart`.
- [X] Sanity-guard the arithmetic: a file shorter than 12 bytes, or a
      `footerLen` that would put `metadataStart` before byte 4, is corrupt.
      Say so rather than slicing out of range.
- [X] Confirm the footer length with a second, independent tool — do not try to
      read hex by eye. `tail -c 8 <file> | od -An -tu4 -N4` prints it in decimal
      (`-t u4` does the little-endian conversion for you), and
      `tail -c 4 <file>` prints the trailing magic as text. Two tools agreeing
      is the validation. `xxd`'s text column shows a dot for every byte that is
      not a printable character, which in a Parquet file is nearly all of them —
      it can confirm `PAR1` and nothing else.

### Verify

- [X] `SELECT * FROM parquet_file_metadata('testdata/basic.parquet');` — you
      cannot check `footerLen` against DuckDB, but you *can* check that your
      metadata range is plausible and that `num_rows` etc. exist inside it.
- [X] Try your tool on a non-Parquet file (any `.go` file). It must refuse
      politely, not panic.

**Pain:** you cannot stream a Parquet file from the front. Everything —
schema, column locations, statistics — is at the *end*. A naive
"read forward and parse" instinct is wrong before the first byte.

**Concept:** Parquet is a footer-first format. Row data is written first
because it is written streaming; the index describing it can only be written
once the offsets are known. This is why Parquet needs seekable storage and why
a truncated file is unreadable even if 99% of the rows arrived.

**Reading:** apache/parquet-format `README.md`, "File format" section.

**Done when:** your tool prints the metadata byte range for every fixture,
rejects a non-Parquet file, and you can explain out loud why a Parquet file
cannot be read from a pipe.

---

## Stage 1 — The Thrift compact protocol (test-first)

Goal: turn the footer's bytes into values. This is the densest byte-level stage
in the project and the reason the rest becomes possible.

**This stage is test-first.** Write the test with hand-encoded byte literals
BEFORE the decoder. You will need to hand-encode a few numbers anyway to
understand the format; putting those bytes in a test is free.

### Build — one function or method at a time, each with its own test

Order matters: every item uses the ones above it. Wire type codes, for
reference throughout: bool-true=1, bool-false=2, i8=3, i16=4, i32=5, i64=6,
double=7, binary=8 (binary AND string), list=9, set=10, map=11, struct=12
(structs AND unions), uuid=13.

The field header is described in the spec's **"Struct encoding"** section, not
in a section of its own.

- [X] Read the compact-protocol spec end to end before coding. It is short.
- [X] `Decoder` struct in `internal/thrift` — `buf []byte`, `pos int`. The
      cursor everything else reads from.
- [X] `Varint() (uint64, error)` — unsigned LEB128. Test: `01`→1, `7f`→127,
      `80 01`→128, `ac 02`→300, `df 89 03`→50399, plus empty buffer, truncated
      (`80`), and eleven continuation bytes.
- [X] `fromZigzag(n uint64) int64` and `toZigzag(n int64) uint64` — pure
      arithmetic, no buffer involved. Only `fromZigzag` is needed for reading;
      `toZigzag` exists so a test can round-trip without a hardcoded table.
- [X] Tests for the zigzag pair: the table (0→0, 1→−1, 2→1, 3→−2, 4→2, 16→8,
      `1<<63`→4611686018427387904) plus a round-trip over a range of values
      and the int64 extremes.
- [X] `Int64() (int64, error)` — `Varint` then `fromZigzag`. This single method
      covers wire types i16, i32 AND i64: all three are zigzag varints with no
      per-width difference. Test `04`→2, `03`→−2, `10`→8, `c0 0c`→800, and
      `80`→error (the varint error must propagate, not be swallowed).
- [X] `Bytes() ([]byte, error)` — an unsigned `Varint` giving a byte count,
      then that many raw bytes. Covers wire type 8, which is BOTH binary and
      string; Thrift does not distinguish them. Test the zero-length case and
      a claimed length that runs past the end of the buffer.
- [X] A string accessor, or a deliberate decision not to have one and to
      convert at the call site instead. Either is fine; decide, don't drift.
- [X] `FieldHeader(lastFieldID int64) (fieldID int64, typeCode byte, err error)`
      — reads one byte: high nibble is the delta to add to `lastFieldID`, low
      nibble is the wire type. A whole byte of `00` is the struct-stop marker;
      report it by returning type code 0, not an error (a stop is the expected
      end of every struct, not a failure — give it a name like `TypeStop`). A
      high nibble of 0 in a non-zero byte means the long form: the field id
      follows as a zigzag varint, which is a call to `Int64`. No loop in this
      method — the caller loops until it sees the stop. No state on the
      Decoder: the previous field id goes in as a parameter and the new one
      comes back, so the caller holds it in a local variable. Test: `15` from
      0 → field 1, type 5; `19` from 1 → field 2, type 9; `48` from 0 → field
      4, type 8; `05 28` from 1 → field 20, type 5, pos +2 (the long form,
      which real fixtures won't contain); `00` → stop.
- [X] Bool fields: the value lives IN the type code — 1 is true, 2 is false —
      so there are no value bytes to read. Whatever reads field values must
      handle this before trying to read anything from the buffer.
- [X] `ListHeader() (count int, elemType byte, err error)` — one byte: high
      nibble is the element count, low nibble the element type. A high nibble
      of 15 means the count is a varint that follows instead. Test both forms;
      `9c` → 9 elements of type 12.
- [X] `Skip(typeCode byte) error` — consume one field's value without
      interpreting it, using only its type code. This is what lets you walk
      past the ~40 fields you don't handle, and survive fields added by writers
      newer than your code. Skipping needs NO field ids: you only need to know
      how many bytes to consume, and you never read the values, so their
      identity is irrelevant.
      A field's value carries no length prefix, so the only way past a struct
      or a list is to walk through it. Fixed-size types are just `pos +=`
      (i8 1, double 8, uuid 16, bool-as-field 0). Variable-size types must be
      read to find their size: i16/i32/i64 a varint, binary a length then that
      many bytes, a list its header then each element, a struct its fields
      until the stop byte.
      **This needs recursion** — `Skip` calls itself for each element of a list
      and each field of a struct. (An earlier version of this task said a flat
      loop with a depth counter would do. That was wrong: a depth counter
      cannot track how many elements remain at each level, and `FileMetaData`
      field 2 is `list<SchemaElement>` — a list of structs.) Real footers nest
      about five deep.
      From the spec's Boolean section: a bool *field* costs 0 bytes (its value
      is in the type code) but a bool *list element* is sent as an i8, so 1
      byte each. Skipping the two is not the same.
      Test by skipping a known field and asserting `pos` landed exactly on the
      next header byte.

**Where the Parquet knowledge goes:** `internal/thrift` must never mention
Parquet. It knows varints, headers, type codes and nothing else. The knowledge
that field 3 of `FileMetaData` is `num_rows` belongs in the root `sawdust`
package, which drives the Decoder. Keeping that line clean is exactly what
lets the thrift package be tested with hand-typed bytes.

### Traps to hit deliberately

- [X] Decode a struct while ignoring the field-id delta (treat every header's
      high nibble as an absolute id). Note how quickly field ids drift wrong.
- [X] Decode a bool field by reading a following byte. Observe that there is no
      following byte — the *type code itself* carries the value inside a
      struct. This one silently corrupts every field after it.
- [X] Nesting is deliberately NOT a stage 1 problem. Stage 1 decodes three
      top-level fields of `FileMetaData` and skips everything else, so no
      nested struct's field numbering ever becomes yours. Revisit when stage 2
      reads the schema list — and with the field id passed as a parameter, each
      caller keeps its own, so there may be nothing to add.

### Verify

- [X] Table-driven tests over hand-built byte slices, with `cmp.Diff`.

This step is new production code plus a test, not a test alone. It lives in the
root `sawdust` package — this is the part that knows field 3 means `num_rows`.

- [X] `metadata.go` in the root package: a `FileMetaData` struct with
      `Version`, `NumRows`, `CreatedBy`. It gains `Schema` in stage 2 and
      `RowGroups` in stage 3, so give it its own file.
- [X] A function taking the footer bytes and returning a `FileMetaData`. Bytes
      in, struct out — so it can be tested with hand-built bytes as well as
      real ones. The caller (`main`) already has `ReadFooter`; it allocates
      `footer.Length` bytes and `ReadAt`s them from `footer.Start`.
- [X] The loop: a `Decoder` over those bytes, `lastFieldID` starting at 0, then
      `FieldHeader` → break on `TypeStop` → switch on field id (`Int64` for 1
      and 3, `Text` for 6, `Skip(typ)` for all the rest) → update
      `lastFieldID`.
- [X] Presence tracking for required fields. In `parquet.thrift` fields 1, 2, 3
      and 4 are `required`, `created_by` (6) is `optional`. A footer with no
      field 3 must error — and you cannot test for that by checking whether
      `NumRows` is zero, because `empty.parquet` legitimately has 0 rows. Track
      presence separately. Same NULL-versus-0 distinction as your observability
      schema.
- [X] Test against the real fixtures. `go test` runs with the package directory
      as the working directory, so `testdata/basic.parquet` works as a relative
      path. Expected: basic 100, empty 0, single_row 1, many_rows 300; all four
      `created_by` = `github.com/parquet-go/parquet-go version 0.32.0(build )`.
- [X] Test with hand-built bytes too: a footer missing field 3, one carrying an
      unknown high field id that must be skipped cleanly, and a truncated one.
- [X] Optional: have the CLI print `num_rows` and `created_by` under the offsets
      it already prints, so you can eyeball a file against DuckDB by hand.
- [X] Fuzz or table-test truncated input: every decoder path must error on a
      short buffer, never panic. Run `go test -fuzz` on the varint reader for a
      minute if you want the habit.

**Pain:** the footer is not self-describing. There are no field names in the
bytes — only numeric ids that are stored as *deltas*, and types that sometimes
encode the value. One misread byte silently shifts every subsequent field, and
the error surfaces far away as a nonsense value.

**Concept:** Thrift compact protocol trades self-description for size. This is
the same trade you already know from column stores: the schema lives once, out
of band, and the payload carries no redundancy. It is also why `parquet.thrift`
is the only authority on which number means which field.

**Reading:** the compact-protocol spec; `parquet.thrift` (skim the whole file
once, then focus on `FileMetaData`).

**Done when:** `num_rows` and `created_by` decoded from a real file match
DuckDB, every decoder errors cleanly on truncated input, and the tests were
written before the code.

---

## Stage 2 — The schema tree

Goal: decode `FileMetaData.schema` into a tree, and render it as something
comparable to a SQL schema.

### Build

- [ ] `internal/format`: Go structs mirroring `FileMetaData` and
      `SchemaElement`, with the field ids in comments. Decode fields you need;
      `skip` the rest.
- [ ] Decode the `schema` list. Note it is *flat* — a depth-first pre-order
      walk, where each element's `num_children` tells you how many of the
      following elements are its children. Element 0 is the root and has no type.
- [ ] Reconstruct the tree from that flat list. A recursive
      "consume next element and then its `num_children` subtrees" function is
      the natural shape.
- [ ] Decode `type` (BOOLEAN=0, INT32=1, INT64=2, INT96=3, FLOAT=4, DOUBLE=5,
      BYTE_ARRAY=6, FIXED_LEN_BYTE_ARRAY=7) and `repetition_type`
      (REQUIRED=0, OPTIONAL=1, REPEATED=2) as named enums, not raw ints.
- [ ] Decode `converted_type` and enough of `logicalType` to distinguish a
      UTF-8 string from opaque bytes, and a microsecond timestamp from a plain
      int64.
- [ ] Compute and store, for every leaf, its `max_definition_level` and
      `max_repetition_level`: walk from root to leaf, +1 for each OPTIONAL
      ancestor (definition) and +1 for each REPEATED ancestor (repetition).
      You will need these in Stage 5; deriving them here is cheaper than
      retrofitting.
- [ ] `sawdust schema <file>` prints the tree with type, repetition, logical
      type, and both max levels.

### Verify

- [ ] `SELECT * FROM parquet_schema('f.parquet');` — same elements, same order,
      same `num_children`, same types.
- [ ] `DESCRIBE SELECT * FROM 'f.parquet';` — your leaf list matches its column
      list, your OPTIONAL flags match its nullability, and your microsecond
      timestamp column shows as `TIMESTAMP` there.
- [ ] Run it against a real file from the observability agent (all seven
      sources). The `journal` schema is the interesting one: many optionals,
      one string carrying JSON.
- [ ] For a flat schema with only required and optional leaves, assert
      `max_definition_level` is 0 for required and 1 for optional. Any other
      value means your ancestor walk is wrong.

**Pain:** you expect a tree and find a flat list with a child *count* per
element — the structure is implied by ordering, so an off-by-one in the
recursion produces a schema that is wrong but still looks plausible.

**Concept:** the schema is a pre-order serialization of a tree, and the pair
(max definition level, max repetition level) is the entire mechanism by which a
columnar format represents nulls and nesting without storing rows. Required
columns have max-def 0, which means — as you will see in Stage 5 — they store
no null-tracking data at all.

**Reading:** parquet-format `README.md` on nested encoding; the Dremel paper's
first three pages if you want the origin story.

**Done when:** `sawdust schema` output matches `parquet_schema` and `DESCRIBE`
on every fixture and on all seven real sources.

---

## Stage 3 — Row groups, column chunks, statistics

Goal: the report that makes this project immediately useful. No value decoding
yet — this stage is pure metadata, and it answers real storage questions.

### Build

- [ ] Decode `RowGroup` (fields: `columns`, `total_byte_size`, `num_rows`,
      `sorting_columns`, `file_offset`, `total_compressed_size`, `ordinal`).
- [ ] Decode `ColumnChunk` and its nested `ColumnMetaData`: `type`,
      `encodings`, `path_in_schema`, `codec`, `num_values`,
      `total_uncompressed_size`, `total_compressed_size`, `data_page_offset`,
      `dictionary_page_offset`, `statistics`, `encoding_stats`.
- [ ] Decode `Statistics`: `null_count`, `distinct_count`, `min_value`,
      `max_value` — and note that min/max are raw `binary`, encoded per the
      column's physical type. Interpreting them requires the schema, so pass it
      in. Also note fields 1 and 2 are the *deprecated* min/max; know which
      your files use.
- [ ] `sawdust stat <file>`: one row per column chunk with path, type, codec,
      encodings, num_values, compressed and uncompressed bytes, the ratio, and
      null_count.
- [ ] Add a per-file summary: total rows, row group count, rows per row group,
      and the columns sorted by compressed bytes descending.

### Measure (record in the README table)

- [ ] Run `sawdust stat` over a day of real observability data. Which columns
      dominate the bytes? Is it the JSON `fields` column, as you would guess?
- [ ] Compare compression ratio per column. Which columns compress 20:1 and
      which barely compress? Form a hypothesis about why *before* looking, then
      check it against the encodings list.
- [ ] What encodings does parquet-go actually use for your columns? Find out
      here rather than assuming — this determines exactly which decoders Stage 4
      and 5 need to implement.
- [ ] Row group sizes across your files: how many rows per row group does a
      typical flush produce? Relate that to your flush thresholds (10k/60s for
      journal, age-driven for pollers).
- [ ] Are any columns' statistics *useful* for pruning — i.e. do min/max ranges
      on `ts` barely overlap between row groups, or do they all span the same
      window?

### Verify

- [ ] `SELECT * FROM parquet_metadata('f.parquet');` — DuckDB gives one row per
      column chunk with `row_group_id`, `path_in_schema`, `compression`,
      `encodings`, `stats_min`/`stats_max`, `stats_null_count`,
      `total_compressed_size`, `data_page_offset`, `dictionary_page_offset`.
      Every field you decode must match.
- [ ] `null_count` for a column must equal
      `SELECT count(*) - count(col) FROM 'f.parquet';`
- [ ] `min_value`/`max_value` decoded through the schema must equal
      `SELECT min(col), max(col) FROM 'f.parquet';` — including for the string
      and timestamp columns, which is where naive byte interpretation breaks.
- [ ] Sum of `total_compressed_size` across chunks should be within a few
      hundred bytes of the file size (the gap is page headers, magic and
      footer). Explain the gap rather than shrugging at it.

**Pain:** statistics look like they should be typed values and are raw bytes
whose meaning depends on the column's physical type and logical annotation. A
min/max on a UTF-8 string is a byte comparison, which is not the collation your
database uses.

**Concept:** this metadata *is* predicate pushdown. Row group min/max is why a
query engine can skip 90% of a file without decompressing anything, and null
counts are why `IS NULL` can be answered from the footer. It is also why the
small-files problem hurts: every file pays a full footer, and per-file metadata
cost is fixed regardless of how few rows it holds.

**Reading:** `parquet.thrift` (`RowGroup`, `ColumnChunk`, `ColumnMetaData`,
`Statistics`); DuckDB's Parquet metadata docs.

**Done when:** every field of `sawdust stat` matches `parquet_metadata`,
decoded min/max match SQL `min()`/`max()` including strings and timestamps,
and the README holds a measured answer to "what dominates my storage".

---

## Stage 4 — Pages, decompression, and one column of numbers

Goal: get actual values out. Scope deliberately narrow: one required `int64`
column, no nulls, no dictionary.

### New fixtures for this stage

- [ ] `zstd.parquet` — the same rows as `basic.parquet`, compressed with ZSTD.
      Two files with identical rows and different compression let you prove your
      decompression step is right: the decoded values must match exactly.
- [ ] A file with enough rows that one column's values do not fit in a single
      page. You will not know how many rows that takes until you look, so
      generate, inspect, and adjust.

### Build

- [ ] Seek to a chunk's `data_page_offset` and decode a `PageHeader` — which is
      Thrift compact again, uncompressed, immediately before each page's
      payload. Fields: `type` (DATA_PAGE=0, DICTIONARY_PAGE=2, DATA_PAGE_V2=3),
      `uncompressed_page_size`, `compressed_page_size`, and one of the
      per-type sub-headers.
- [ ] Note there is no page count and no page index in `ColumnMetaData`: you
      walk pages by consuming `compressed_page_size` bytes and decoding the
      next header, until you have seen `num_values` values.
- [ ] Decompress a page payload with ZSTD (`klauspost/compress/zstd` is fine —
      the learning target is the format, not the compressor). Assert the
      decompressed length equals `uncompressed_page_size`; a mismatch means
      your byte range was wrong.
- [ ] Decode PLAIN-encoded INT64: fixed 8 bytes each, little-endian. Do this
      for a required column so there are no levels in the way.
- [ ] Handle both page versions: in V1 the levels and values are compressed
      *together* in one payload; in V2 the level bytes are stored uncompressed
      before the compressed values, with their lengths given in the header
      (`definition_levels_byte_length`, `repetition_levels_byte_length`). Find
      out which version your fixtures and real files use.
- [ ] `sawdust cat <file> <column>` prints the values.

### Traps to hit deliberately

- [ ] Decompress `compressed_page_size` bytes starting at the page *header*
      offset rather than after it. Read the error ZSTD gives you and learn what
      "wrong offset" looks like.
- [ ] Assume one page per chunk. Generate a fixture with enough rows to force
      several pages and watch a single-page reader silently return partial data.
      That silence is the point: nothing errors, you just get fewer rows.
- [ ] Use `uncompressed_page_size` as the amount to read from the file.

### Verify

- [ ] `SELECT sum(c), count(c), min(c), max(c) FROM 'f.parquet';` must equal the
      same aggregates over your decoded slice. A checksum over the whole column
      catches ordering bugs that `sum` alone would hide — so also compare the
      first and last 10 values in order.
- [ ] Decode the same column from `basic.parquet` and `zstd.parquet` and assert
      the two value slices are identical.
- [ ] Decode from `many_rows.parquet` and confirm you get all rows, in row group
      order.
- [ ] `single_row.parquet` and `empty.parquet` must both work — the empty file
      is the guard-placement test: a zero-row file has a schema and no pages.

**Pain:** a page's bytes are three concatenated regions with no separators, the
region sizes come from two different places depending on page version, and
decompression is the only thing that will tell you when you got it wrong — via
an error that says nothing about offsets.

**Concept:** pages are the unit of I/O and compression; row groups are the unit
of parallelism and pruning; column chunks are the contiguous per-column bytes
within a row group. Compression happens *per page*, which is why a dictionary
built per chunk and a codec applied per page interact the way they do.

**Reading:** parquet-format `README.md` on page layout; `parquet.thrift`
(`PageHeader`, `DataPageHeader`, `DataPageHeaderV2`).

**Done when:** decoded int64 values match DuckDB exactly — count, order, and
aggregates — across all fixture variants including multi-page, multi-row-group,
single-row and empty.

---

## Stage 5 — Nulls, dictionaries, strings, timestamps

Goal: reconstruct whole rows. This is where the format's cleverness lives.

### New fixtures for this stage

- [ ] `optionals_all_null.parquet` — every row leaves the optional fields unset,
      so those columns are null all the way down.
- [ ] `optionals_never_null.parquet` — every row sets them, so those columns have
      no nulls at all.
- [ ] These need a parameter on `buildRows` controlling whether the optional
      fields get filled, not a second `row` type. Same struct, different data.

### Build

- [ ] The RLE/bit-packing hybrid decoder. Each run starts with a varint header:
      the low bit selects the mode, the remaining bits are the length. Bit
      clear → an RLE run of `header >> 1` repeats of a single bit-packed value;
      bit set → `(header >> 1) * 8` bit-packed values. Values are packed
      LSB-first at a fixed bit width.
- [ ] Definition levels for an optional leaf: bit width is
      `ceil(log2(maxDefLevel + 1))`, so 1 for a flat optional column. A level
      equal to `maxDefLevel` means present; anything less means null at that
      ancestor depth. **Only `num_values - nullCount` actual values are stored**
      — this is the part that breaks a naive reader.
- [ ] Confirm the mirror case: a REQUIRED column stores no definition levels at
      all. Assert it on a fixture.
- [ ] Handle the V1 level framing: when levels are RLE-encoded in a V1 data
      page, a 4-byte little-endian length precedes the RLE stream. V2 has no
      such prefix — the lengths are in the header.
- [ ] Dictionary pages: decode the `DICTIONARY_PAGE` at
      `dictionary_page_offset` (PLAIN-encoded values), then decode the data
      page's indices, which are RLE/bit-packed with the **bit width stored as
      the first byte** of the value region. Map indices back to dictionary
      entries.
- [ ] PLAIN BYTE_ARRAY: each value is a 4-byte little-endian length followed by
      bytes. Apply the schema's UTF-8 annotation to decide `string` vs `[]byte`.
- [ ] INT64 microsecond timestamps → `time.Time` in UTC, using the logical type
      to distinguish millis/micros/nanos and adjusted-to-UTC or not.
- [ ] Doubles (IEEE 754 LE) and booleans (bit-packed, LSB-first).
- [ ] A row iterator: assemble one struct/map per row across all leaf columns of
      a row group.
- [ ] `sawdust cat <file>` with no column argument emits all rows as
      newline-delimited JSON.

### Traps to hit deliberately

- [ ] Zip values to rows positionally, ignoring definition levels. On
      `optionals_all_null.parquet` and a mixed-null column, watch values land on
      the wrong rows — every row after the first null is shifted.
- [ ] Assume the dictionary applies to the whole column chunk and that every
      data page uses it. Check `encoding_stats`: a chunk can fall back to PLAIN
      partway through when the dictionary grows too large.
- [ ] Read the dictionary index bit width from the schema instead of from the
      page's first byte.

### Verify

- [ ] `COPY (SELECT * FROM 'f.parquet') TO 'oracle.csv';` then have your reader
      emit the same rows in the same order and diff. Row-for-row equality is the
      Done-when; anything less hides a shift bug.
- [ ] `optionals_all_null.parquet` and `optionals_never_null.parquet` must both
      round-trip, and `count(col)` from your output must equal DuckDB's.
- [ ] For the low-cardinality column, assert your decoded distinct set equals
      `SELECT DISTINCT col FROM 'f.parquet';` and that the dictionary-encoded
      and plain-encoded fixtures produce identical values.
- [ ] Timestamps: `SELECT epoch_us(ts) FROM ...` compared against your
      `UnixMicro()`. Exact equality — no rounding tolerance.
- [ ] Then the real test: a real `journal` file from the agent, with its many
      optionals and its JSON `fields` column, row-for-row against a DuckDB
      export.

**Pain:** values and rows are not the same count. A column with nulls stores
fewer values than rows, and the only thing that says where the gaps go is a
separate bit-packed stream — which itself has two different framings depending
on page version.

**Concept:** definition levels are how a columnar format represents absence
without storing it, and the reason `NULL` costs approximately one bit per row
rather than one value per row. This is the mechanism behind the nullable-pointer
design you already chose in the agent, seen from below.

**Reading:** parquet-format `Encodings.md` (RLE and dictionary sections, in
full); the parquet-go source if a framing detail stays ambiguous after the spec.

**Done when:** full row reconstruction is byte-for-byte identical to a DuckDB
CSV export, on every fixture and on at least one real file per source.

---

## Stage 6 — Pruning, and the first stage you plan yourself

**You write this stage's task breakdown.** Here is the goal and the Done-when;
the decomposition into checkboxes, hints and traps is yours. Bring the plan for
review before you start building, the same way you would bring a design sketch.

**Goal:** use the metadata from Stage 3 to avoid work in Stage 4/5. Given a
simple predicate on one column (`ts > X`, `unit = 'y'`), skip row groups whose
statistics prove they cannot match, and report how many bytes and pages you
avoided reading.

**Done when:** for a real query against a real day of data, `sawdust` reports a
measurable reduction in bytes read versus the no-predicate path, the returned
rows are identical to DuckDB's answer for the same predicate, and you can state
the case where statistics-based pruning does nothing for your data and why.

Things worth deciding in the plan (not instructions — decisions):

- What is the predicate's type — a struct, a function, a tiny expression tree?
- Where does pruning live: inside the row iterator, or as a filter the caller
  composes? This is an API decision that Stage 7 will inherit.
- Do you go further than row groups into the optional ColumnIndex/OffsetIndex
  structures for page-level pruning, or stop at row groups and say why?
- How do you *prove* the saving — count bytes read at the file-read boundary,
  or count pages decoded? Which number would convince a skeptic?

---

## Stage 7 — The public surface

**This stage is yours end to end**: goal, tasks, and design. No shape is given,
by design — the point is that you now know this domain well enough to design
its interface.

What it has to end up covering:

- A public API someone else could use without reading the implementation:
  what is exported, what stays internal, what the zero values mean, what
  happens on malformed input.
- Documented behaviour: godoc on every exported symbol, a runnable
  `Example` or two, and a README that shows the three things a user most
  wants to do.
- A versioning story: `v0.x` tags, what you consider breaking, and a note on
  what you would have to change to reach `v1`.
- Integration: `machine-observability` imports `sawdust` for an `inspect` or
  `verify` command. Real usage is the design review — anything awkward at the
  call site is a design flaw, not a caller problem.

**Done when:** the agent repo depends on a tagged sawdust version and uses it
for something you actually run, and the API survived that contact without a
breaking change you had to make immediately afterwards.

---

## Stretch

Pick by curiosity, not order.

- [ ] **Write** Parquet, not just read it. Round-trip your own output through
      DuckDB. This is where you learn why writers buffer whole row groups.
- [ ] ColumnIndex / OffsetIndex — the page-level statistics structures at the
      end of the file, which is what makes fine-grained pruning possible.
- [ ] Bloom filters (`bloom_filter_offset`): equality pruning where min/max
      can't help.
- [ ] `DELTA_BINARY_PACKED` and `DELTA_BYTE_ARRAY`. Measure them against PLAIN
      on your `ts` and `cursor` columns — both are near-sorted, which is the
      case delta encoding exists for.
- [ ] `mmap` vs `pread` for footer + chunk access; measure, don't assume.
- [ ] A vectorized aggregation over your own decoder: `SELECT unit, count(*)
      ... GROUP BY unit` with a hand-written hash table, benchmarked against
      DuckDB. Expect to lose by a lot, then find out where the time goes.
- [ ] Read a file written by a *different* implementation (pyarrow, DuckDB's own
      COPY TO). Formats have dialects; find one.
