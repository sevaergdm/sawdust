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

## The decoding recipe (reference for stages 2–5)

**On the wire there is only one kind of struct.** `FileMetaData`,
`SchemaElement`, `TimestampType`, `RowGroup` — every one of them is "field
headers until a stop byte". The bytes don't say which struct they are. The
names, the field meanings and which ids matter live entirely in
`parquet.thrift`. Unions are encoded identically to structs, so they decode
with the same loop.

So: write one struct decoder and you have the shape of all of them. Only the
field ids in the switch change.

Every field in `parquet.thrift` has a declared type, and that type tells you
which primitive to call:

| thrift declares | in the bytes | you call |
|---|---|---|
| `i16` `i32` `i64` | zigzag varint | `d.Int64()` |
| `SomeEnum` | i32 on the wire | `d.Int64()`, then convert |
| `i8` | one raw byte (NOT a varint) | — |
| `bool` (struct field) | nothing — value is in the type code | `d.Bool(fieldType)` |
| `bool` (list element) | one byte: 1 true, 2 false | — |
| `string` `binary` | length, then that many bytes | `d.Text()` / `d.Bytes()` |
| `double` | 8 bytes, IEEE 754 little-endian | — |
| `SomeStruct` | field headers until stop | your own read function |
| `SomeUnion` | same as a struct | same loop, exactly one field set |
| `list<T>` `set<T>` | ListHeader, then N of T | `d.ListHeader()`, then loop |
| `map<K,V>` | MapHeader, then N pairs | `d.MapHeader()`, then loop |
| `optional X` | may be absent entirely | pointer field, nil when the case never fires |
| `required X` | must be present | presence bool, checked after the loop |

**Four rules, applied recursively.** Look up the field's declared type:

1. **Primitive** → call the matching method.
2. **Struct or union** → a read function with its own header loop and its own
   local `lastFieldID`, applying these rules to its own fields.
3. **List or map** → read the header, then apply these rules to each element.
4. **Optional** → pointer. **Required** → presence bool.

One decoder walks the whole footer once, front to back. Functions take turns
driving it; pass `*thrift.Decoder` (never a copy) so the cursor stays shared.
A read function returns with the cursor just past its own stop byte.

## The test-tier rule (decide once, not per test)

| what's under test | input | expected values come from |
|---|---|---|
| a pure function over Go values | literals, no files | hand-computed |
| a decoder over bytes | hand-built byte literals | the spec, or hand-decoded |
| end to end | real fixtures in `testdata/` | DuckDB |

Three standing rules on top of that:

- **One test function per unit under test.** If a failure could come from two
  different functions, split it. `BuildTree` takes a `[]SchemaElement`, so it
  gets literals and no file — a decoder bug then fails only the decoder's test.
- **Expected values never come from your own code's output.** If you ran it and
  wrote down what it printed, the test proves only that the code agrees with
  itself.
- **Hand-built inputs are for cases real files cannot produce** — a missing
  required field, a corrupt length, an unknown field id, a `num_children` that
  lies. Not for cases a fixture already covers.

And when a value has no oracle at all — the max definition and repetition
levels are the first such case — say so in the task and write the hand-computed
expectation down, rather than leaving it to be re-derived later.

---

## Stage 2 — The schema tree

Goal: decode `FileMetaData.schema` into a tree, and render it as something
comparable to a SQL schema.

### Build — one deliverable at a time, each with its own test

Everything here is Parquet knowledge, so it stays in the root `sawdust` package
— expanding `metadata.go`, NOT under `internal/`. Stage 7 needs these types
exported.

You are **extending** stage 1, not rebuilding it: `FileMetadata` gains a schema
field, and case 2 in `ReadFileMetadata` stops being a `Skip`.

- [X] A `SchemaElement` struct with **exported** fields, like `FileMetadata`.
      Field ids from `parquet.thrift` (line 202) in comments. Needed now:
      thrift `type` (1) → Go `Type`, `repetition_type` (3) → `RepetitionType`,
      `name` (4) → `Name`, `num_children` (5) → `NumChildren`. The thrift names
      are lowercase; the Go fields are not — which also sidesteps `type` being
      a reserved word. `converted_type` (6) and `logicalType` (10) come later
      in the stage. Optional fields are pointers: `num_children` is absent for
      leaves, and absent means 0.
- [X] Named types for the enums rather than raw ints: `PhysicalType`
      (BOOLEAN=0, INT32=1, INT64=2, INT96=3, FLOAT=4, DOUBLE=5, BYTE_ARRAY=6,
      FIXED_LEN_BYTE_ARRAY=7) and `FieldRepetitionType` (REQUIRED=0,
      OPTIONAL=1, REPEATED=2). `PhysicalType` rather than `Type` so the field
      reads `Type PhysicalType` and pairs sensibly with `LogicalType` later —
      these are parquet-format's own "physical types". A `String()` method on
      each means printing shows names instead of numbers.
- [X] A function decoding ONE `SchemaElement` from a decoder. Same field-header
      loop shape as `ReadFileMetadata` — worth noticing the repetition, and
      deciding whether to factor it out. Don't factor on the second instance;
      decide once there are three.
- [X] Decode `FileMetaData` field 2 into a `[]SchemaElement`: a `ListHeader`,
      then that many calls to the above. Add presence tracking — `schema` is
      `required`, so an absent field 2 is an error.
- [X] **Checkpoint before going further:** compare the flat list against
      `SELECT * FROM parquet_schema('testdata/basic.parquet');` — same count,
      same order, same names, same types, same `num_children`. Get that green
      before touching the tree or the logical types.
- [X] **New fixture first: `nested.parquet`.** Your existing fixtures are all
      depth 1 — every column hangs directly off the root — so the tree and the
      levels would have nothing to be checked against. A second struct type in
      `genfix` with a nested struct field, an optional nested struct field, and
      a `[]string` gives this (verified 2026-08-23):

      ```
      outer          4 children
      ├─ id          REQUIRED  leaf
      ├─ in          REQUIRED  2 children   (group, no type)
      │  ├─ a        REQUIRED
      │  └─ b        REQUIRED
      ├─ opt_in      OPTIONAL  2 children
      │  ├─ a        REQUIRED
      │  └─ b        REQUIRED
      └─ tags        REPEATED  leaf
      ```

      Why it earns its place: the ROOT's children are elements 1, 2, 5 and 8 —
      NOT 1, 2, 3, 4. A naive "children are the next N elements" reading breaks
      immediately, and a depth-1 fixture can never reveal that. It also gives
      duplicate leaf names (`a` and `b` twice, which is why stage 3's
      `path_in_schema` is a list, not a name), and two identical-looking
      elements with different definition levels (`in.a` is 0, `opt_in.a` is 1).
      Note parquet-go writes `[]string` as a plain REPEATED leaf, not a
      three-level LIST group — so `tags` stays flat but gives you a max
      repetition level of 1.
- [X] Reconstruct the tree from the flat list. It is a depth-first pre-order
      walk: each element's `num_children` says how many of the elements that
      *follow* it are its children — where "follow" means after each preceding
      subtree has been fully consumed. Element 0 is the root and has no type.
      The natural shape is recursive — consume one element, then consume that
      many subtrees, returning the updated index so the caller knows where its
      next child starts. `Skip` is your precedent. Keep `SchemaElement` as the
      faithful mirror of the thrift and add a separate `SchemaNode`
      (element + children) for the derived shape.
      Two guards: check the index is in range before reading an element (a
      `num_children` claiming more children than exist must error, not panic),
      and after the top-level call the index must equal `len(list)` — leftover
      elements mean the counts don't add up. That second check is what catches
      an off-by-one that would otherwise produce a plausible-looking wrong
      tree.
- [X] **`converted_type`** — no new shape. A `case 6` in `readSchemaElement`:
      `d.Int64()`, convert to `ConvertedType`, take the address. Then extend
      `basicSchema` and `nestedSchema` in the tests, since the decoded output
      genuinely changes.
      Oracle (run it, don't take my word for any value):
      `duckdb -c "select name, converted_type from parquet_schema('testdata/basic.parquet')"`

- [ ] **`logicalType`** — the one new decoding shape in this stage. Work it in
      this order.

      **Step 1 — see what is actually in your files.**
      ```sh
      duckdb -c "select name, logical_type from parquet_schema('testdata/basic.parquet')"
      ```
      Three distinct annotations come back: `StringType()`,
      `TimestampType(isAdjustedToUTC=1, unit=TimeUnit(MICROS=MicroSeconds()))`,
      and `IntType(bitWidth=@, isSigned=1)`. (`@` is ASCII 64 — DuckDB is
- [X] **`logicalType`** — the one new decoding shape in this stage. Work it in

      **Step 2 — look those up in `parquet.thrift`.**
      `LogicalType` is declared with the keyword `union`, not `struct`. A union
      means **exactly one of its fields is set** — a column's annotation is one
      thing, never two. On the wire it is encoded identically to a struct (field
      headers then a stop byte); the difference is that only one field header
      appears, which is a promise the writer keeps rather than something the
      bytes enforce.
      It lists 19 variants; you have met three. Find their definitions and note
      the shape of each:
      - `StringType` — an empty struct
      - `TimestampType` — two fields, one of which is another union
      - `IntType` — two fields

      **Step 3 — count the functions.** Rule 2 of the recipe: "struct or union
      → a read function with its own header loop and its own local
      `lastFieldID`". So every non-empty struct or union in that list needs one,
      and an empty struct needs nothing but a stop-byte read. Counting them
      tells you how big this item is before you write any of it. No choice
      here — it falls out of step 2.

      **Step 4 — Go has no union type, so pick a representation.**
      The struct-of-pointers version (`String *StringType; Timestamp
      *TimestampType`, exactly one non-nil) is the obvious approximation, but
      nothing stops both being set, and it cannot distinguish "a variant I don't
      model" from "no annotation at all".
      **Decision taken 2026-08-24: a sealed interface.**
      ```go
      type LogicalType interface{ isLogicalType() }

      type StringType struct{}
      func (StringType) isLogicalType() {}

      type TimestampType struct {
          IsAdjustedToUTC bool
          Unit            TimeUnit
      }
      func (TimestampType) isLogicalType() {}

      type IntType struct {
          BitWidth int64
          IsSigned bool
      }
      func (IntType) isLogicalType() {}

      type UnknownType struct{ FieldID int64 }   // a variant we don't decode
      func (UnknownType) isLogicalType() {}
      ```
      Why: "exactly one" becomes a compiler guarantee rather than a comment, and
      the unexported marker method means only this package can add variants.
      Read it with a type switch, not nil checks.
      Two consequences:
      - `SchemaElement.LogicalType` is `LogicalType`, **not** `*LogicalType` —
        an interface value is already nilable, so absent needs no extra pointer.
      - The other 16 variants need no decisions at all: the default branch does
        `Skip(fieldType)` and returns `UnknownType{FieldID: fieldID}`. Every
        union member is a struct, so `Skip` consumes whatever the payload was.
        Adding a real variant later is one type plus one case.

      One case the loop makes possible and you must answer: a union whose bytes
      are just a stop byte, with no field set at all. That breaks the
      "exactly one" rule. Decide whether `readLogicalType` errors or returns
      nil.

      **Step 5 — write them.** All of them are the same field-header loop you
      have written four times. The field ids you need:
      - `LogicalType`: 1 = STRING, 8 = TIMESTAMP, 10 = INTEGER; skip the other 16
      - `TimestampType`: 1 = `isAdjustedToUTC` (bool — `d.Bool(fieldType)`, no
        value bytes), 2 = `unit`
      - `TimeUnit`: a union of three empty structs — 1 MILLIS, 2 MICROS,
        3 NANOS. Nothing to read but the field id, then its stop byte.

      Called from a new `case 10` in `readSchemaElement`, in `schema.go`.

- [X] **Max definition and repetition levels.** These are properties of a
      *leaf* (only leaves have column chunks), so the natural output is a flat
      list of columns rather than fields sprinkled on the tree:
      ```go
      type Column struct {
          Path               []string   // e.g. ["opt_in", "a"]
          Element            SchemaElement
          MaxDefinitionLevel int
          MaxRepetitionLevel int
      }

      func Columns(root SchemaNode) []Column   // in schema.go
      ```
      One walk from the root, carrying the path and both running counts down
      each branch, emitting a `Column` at every leaf.
      **definition** counts every field in the path that is OPTIONAL **or**
      REPEATED (a repeated field can be empty, so it needs a level to say so);
      **repetition** counts only the REPEATED ones. The root contributes
      nothing — it has no repetition type.
      `Path` is not decoration: stage 3 matches column chunks to columns by
      `path_in_schema`, which is exactly this, and it is the only way to tell
      `inner.a` from `opt_in.a`.
      **There is no DuckDB oracle for the levels** — no `parquet_schema` column
      exposes them. The check is hand-computed from the rule above. For
      `nested.parquet`:
      | path | def | rep |
      |---|---|---|
      | id | 0 | 0 |
      | inner.a | 0 | 0 |
      | inner.b | 0 | 0 |
      | opt_in.a | 1 | 0 |
      | opt_in.b | 1 | 0 |
      | tags | 1 | 1 |

      And for `basic.parquet`: every leaf 0/0 except `even_row_number` and
      `opt_rand_id`, which are 1/0.
      Note `Columns` returns 6 entries for `nested.parquet`, not 9 — groups are
      not columns.

- [ ] **CLI restructure — a design decision, and the first real one since
      stage 0.** There is no subcommand machinery: `cmd/sawdust/main.go` reads a
      `-path` flag and prints five lines. But four task items across stages 2–5
      assume `sawdust schema`, `sawdust stat` and `sawdust cat`. So decide the
      shape once, here, rather than four times.
      Two options:
      - **Subcommands** — `sawdust schema <file>`. Switch on `os.Args[1]`, with
        a `flag.FlagSet` per subcommand. More machinery up front; extends to
        `stat` and `cat` with no further thought.
      - **A mode flag** — `sawdust -mode schema -path X`. Less restructuring
        now, gets awkward at three or four modes.
      Whichever you pick, three things move:
      - The open / `Stat` / `ReadFooter` / allocate / `ReadAt` /
        `ReadFileMetadata` sequence is common to every subcommand — and is
        already duplicated between `main` and two test files. Six lines, four
        callers. Extract it.
      - The current five-line output becomes one subcommand of its own
        (`footer`, or `info`).
      - `main` becomes dispatch plus error handling, and nothing else.
      Worth noting where that extracted helper is heading: a
      `sawdust.ReadFile(path) (FileMetadata, error)` in the library is exactly
      the convenience wrapper stage 7 wants, and what machine-observability
      would call. Deciding now whether it lives in `cmd` or in the package is
      the one part of this that isn't purely mechanical.

- [X] **The schema printer.** No new decoding. Indent by depth from the tree,
      and print `Type`, `RepetitionType` and `ConvertedType` straight with `%v`
      — they are all `fmt.Stringer`s, and a nil pointer prints `<nil>` rather
      than panicking.
      A `String()` on the `LogicalType` variants is worth adding so they print
      like DuckDB's column. `UnknownType` already has one; `TimestampType`
      needs to render its unit, and `IntType` its width and signedness. Those
      are value receivers on structs with no pointers inside, so there is
      nothing for `fmt` to fail on.
      The levels live on `Column`, not on the tree, so either print the leaf
      rows from `Columns()` or look each leaf up by path while walking.

### Verify

- [X] `SELECT * FROM parquet_schema('f.parquet');` — same elements, same order,
      same `num_children`, same types.
- [X] `DESCRIBE SELECT * FROM 'f.parquet';` — this is the oracle for **logical
      type resolution**, not for structure. Measured 2026-08-25: `DESCRIBE`
      reassembles groups back into SQL `STRUCT` types, so on `nested.parquet` it
      returns FOUR columns (`inner` and `opt_in` shown as
      `struct(a bigint, b varchar)`), matching the root's children rather than
      the six leaves. So there are three distinct oracles, checking three
      different things:
      | query | shows | count on nested |
      |---|---|---|
      | `parquet_schema` | the tree as stored, groups included | 9 |
      | `parquet_metadata` | one row per leaf column chunk | 6 |
      | `DESCRIBE` | DuckDB's reconstructed SQL view | 4 |
      What to check here is that each annotation resolves the way DuckDB
      resolves it: `BYTE_ARRAY` + `StringType` → `varchar`; `INT64` +
      `TimestampType{isAdjustedToUTC: true, micros}` → `timestamp with time
      zone` (the "with time zone" comes from `isAdjustedToUTC`); a REPEATED
      leaf → `varchar[]`. Your own output prints the PHYSICAL type, so the two
      are supposed to differ — the check is that the chain is consistent, not
      that the strings match.
- [X] Run it against a real file from the observability agent (all seven
      sources). The `journal` schema is the interesting one: many optionals,
      one string carrying JSON.
- [X] For a flat schema with only required and optional leaves, assert
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

**This is where the scaffolding fades.** Four more structs, all decoded by the
loop you have now written five times. So `RowGroup` is worked out for you as a
model, and you fill in the same form for the other three before writing any
code. Nothing here needs a new technique — only the recipe table and the form.

**The form.** For each struct, five lines:

1. **Struct and field ids** — from `parquet.thrift`.
2. **Per field: what to call** — a lookup in the decoding recipe table.
3. **What the function returns** — and its signature.
4. **Required vs optional** — presence bools vs pointer fields.
5. **One expected value** — from the oracle, with the command that produced it.

---

#### Worked model: `RowGroup` (thrift line 1050)

**Fields:**
```
1: required list<ColumnChunk> columns
2: required i64              total_byte_size
3: required i64              num_rows
4: optional list<SortingColumn> sorting_columns
5: optional i64              file_offset
6: optional i64              total_compressed_size
7: optional i16              ordinal
```

**Per field:** 1 → `ListHeader` + loop calling `readColumnChunk`. 2, 3, 5, 6, 7
→ `d.Int64()`. 4 → `Skip` (you don't need sorting columns).

**Signature:** `readRowGroup(d *thrift.Decoder) (RowGroup, error)` in a new
`rowgroup.go`. Called from a `case 4` in `ReadFileMetadata`, which is a
`ListHeader` plus a loop — identical to the `case 2` you already wrote.

**Modelled:** 1, 2, 3 (required → presence bools); 5, 6, 7 (optional →
pointers). **Skipped by the default branch:** 4 (`sorting_columns`) — it is a
`list<SortingColumn>`, another struct to model, and nothing in stage 3 needs it.

Note "optional in the thrift" and "modelled by you" are separate axes. A field
can be optional and simply not modelled, in which case it has no struct field
and therefore no pointer — it just falls to `Skip`.

**One expected value:**
```sh
duckdb -c "select num_rows, num_row_groups from parquet_file_metadata('testdata/many_rows.parquet')"
```
`many_rows.parquet` has 3 row groups of 100 rows, so `len(RowGroups)` is 3 and
each `NumRows` is 100.

#### Worked model `ColumnChunk` (thrift line 992)

** Fields:**
```
1: optional string          file_path
2: required i64             file_offset (deprecated, expect 0 from writers)
3: optional ColumnMetadata  meta_data
```

**Per field:** 1, 2, 3 -> `FieldHeader` + loop w/switch. `d.Text()` for 1, `Int64()` for 2, `readColumnMetadata` for 3. default -> `Skip`

**Signature:** `readColumnChunk(d *thirft.Decoder) (ColumnChunk, error)` possibly in a new columns.go file

**Required:** 2 - presence bool. **Optional:** 1, 3 - pointers

**One expected value:**
```sh
duckdb -c "select row_group_id, path_in_schema, file_offset from parquet_metadata('testdata/basic.parquet')"
```
`basic.parquet` has 8 columns all in one row group. `file_offset` is always 0 because the field is deprecated and writers should set it to 0.

#### Worked model `ColumnMetaData` (thrift line 909)

**Fields:**
```
1.  required Type               type
2.  required <list>Encoding     encodings
3.  required <list>string       path_in_schema
4.  required CompressionCodec   codec
5.  required i64                num_values
6.  required i64                total_uncompressed_size
7.  required i64                total_compressed_size
9.  required i64                data_page_offset
11. optional i64                dictionary_page_offset
12. optional Statistics         statistics
```

**Per field:**

all -> `FieldHeader` + loop w/switch

1. `d.Int64()` -> PhysicalType
2. `ListHeader` -> `d.Int64()` -> new Encoding enum (thrift 586-660)
3. `ListHeader` -> `d.Text()`
4. `d.Int64()` -> new CompressionCodec enum (thrift 671-680)
5. `d.Int64()`
6. `d.Int64()`
7. `d.Int64()`
9. `d.Int64()`
11. `d.Int64()`
12. `readStatistics`

**Signature:** `readColumnMetadata(d *Decoder) (ColumnMetadata, error)` either in the same columns.go file or a common file for all  

**Required:** 1-9 (excl. 8) - presence bool. **Optional:** 11, 12 - pointers

**One expected value:**

```sh
duckdb -c "select type, encodings, path_in_schema, compression, num_values, total_uncompressed_size, total_compressed_size, data_page_offset, dictionary_page_offset from parquet_metadata('testdata/basic.parquet')"

```
┌────────────┬──────────────────────────────┬─────────────────┬──────────────┬────────────┬─────────────────────────┬───────────────────────┬──────────────────┬────────────────────────┐
│    type    │          encodings           │ path_in_schema  │ compression  │ num_values │ total_uncompressed_size │ total_compressed_size │ data_page_offset │ dictionary_page_offset │
│  varchar   │           varchar            │     varchar     │   varchar    │   int64    │          int64          │         int64         │      int64       │         int64          │
├────────────┼──────────────────────────────┼─────────────────┼──────────────┼────────────┼─────────────────────────┼───────────────────────┼──────────────────┼────────────────────────┤
│ INT64      │ PLAIN                        │ row_number      │ UNCOMPRESSED │        100 │                     876 │                   876 │                4 │                   NULL │
│ INT64      │ PLAIN, RLE                   │ even_row_number │ UNCOMPRESSED │        100 │                     497 │                   497 │              880 │                   NULL │
│ BYTE_ARRAY │ DELTA_LENGTH_BYTE_ARRAY      │ rand_id         │ UNCOMPRESSED │        100 │                     830 │                   830 │             1377 │                   NULL │
│ BYTE_ARRAY │ RLE, DELTA_LENGTH_BYTE_ARRAY │ opt_rand_id     │ UNCOMPRESSED │        100 │                     387 │                   387 │             2207 │                   NULL │
│ BYTE_ARRAY │ DELTA_LENGTH_BYTE_ARRAY      │ category        │ UNCOMPRESSED │        100 │                     442 │                   442 │             2594 │                   NULL │
│ DOUBLE     │ PLAIN                        │ rand_float      │ UNCOMPRESSED │        100 │                     876 │                   876 │             3036 │                   NULL │
│ INT64      │ PLAIN                        │ ts              │ UNCOMPRESSED │        100 │                     876 │                   876 │             3912 │                   NULL │
│ BOOLEAN    │ PLAIN                        │ is_odd          │ UNCOMPRESSED │        100 │                      59 │                    59 │             4788 │                   NULL │
└────────────┴──────────────────────────────┴─────────────────┴──────────────┴────────────┴─────────────────────────┴───────────────────────┴──────────────────┴────────────────────────┘

#### Worked model `Statistics` (thrift line 267)

**Fields:**
```
3. optional i64     null_count
4. optional i64     distinct_count
5. optional binary  max_value
6. optional binary  min_value
```

**Per field:** 3, 4 -> `d.Int64()`; 5,6 -> `d.Bytes()`

**Signature:** `readStatistics(d *Decoder) (Statistics, error)` either in the same columns.go file or a common file for all

**Optional:** 3-6 - pointers

**One expected value:**

```sh
duckdb -c "select stats_null_count, stats_distinct_count, stats_max_value, stats_min_value from parquet_metadata('testdata/basic.parquet')"
```

┌──────────────────┬──────────────────────┬────────────────────────┬────────────────────────┐
│ stats_null_count │ stats_distinct_count │    stats_max_value     │    stats_min_value     │
│      int64       │        int64         │        varchar         │        varchar         │
├──────────────────┼──────────────────────┼────────────────────────┼────────────────────────┤
│                0 │                 NULL │ 100                    │ 1                      │
│               50 │                 NULL │ 100                    │ 2                      │
│                0 │                 NULL │ id-0100                │ id-0001                │
│               67 │                 NULL │ opt-0099               │ opt-0003               │
│                0 │                 NULL │ foo                    │ bar                    │
│                0 │                 NULL │ 0.9973219642829593     │ 0.0014109740758089743  │
│                0 │                 NULL │ 2026-01-01 00:01:40+00 │ 2026-01-01 00:00:01+00 │
│                0 │                 NULL │ true                   │ false                  │
└──────────────────┴──────────────────────┴────────────────────────┴────────────────────────┘

---

#### Your turn: fill in the form for these three

Bring the filled forms before writing code. If a form is right, write it
unassisted; if not, we have caught it at the cheap stage.

- [X] **`ColumnChunk`** (thrift line 992). Nine fields. You need 1, 2, 3;
      skip 4–9 (offset/column index pointers and encryption). Note field 3 is a
      nested struct, not a list.
- [X] **`ColumnMetaData`** (thrift line 909). Seventeen fields — the biggest
      struct in the project, and the one that makes `sawdust stat` possible.
      You need 1–7, 9, 11, 12. Two fields are `list<enum>` and one is
      `list<string>`, so this is the first place `ListHeader` feeds something
      other than structs.
- [X] **`Statistics`** (thrift line 267). Nine fields; you need 3, 4, 5, 6.
      Fields 1 and 2 are the DEPRECATED min/max — check which your files
      actually write before deciding whether to decode them. Fields 5 and 6 are
      raw `binary` whose meaning depends on the column's physical type, so
      interpreting them needs the schema; decoding them does not.
- [X] `sawdust stat <file>`: one row per column chunk with path, type, codec,
      encodings, num_values, compressed and uncompressed bytes, the ratio, and
      null_count.
- [X] Add a per-file summary: total rows, row group count, rows per row group,
      and the columns sorted by compressed bytes descending.

### Measure (record in the README table)

- [X] Run `sawdust stat` over a day of real observability data. Which columns
      dominate the bytes? Is it the JSON `fields` column, as you would guess?
      Answer: `fields` dominates as expected since it will tend to be the largest field, and most varied
- [X] Compare compression ratio per column. Which columns compress 20:1 and
      which barely compress? Form a hypothesis about why *before* looking, then
      check it against the encodings list.
      Answer: None compress 20:1, but boot_id and seqnum_id have the highest values. This is can be explained by the high number 
        of repeating values. Similarly, fields does have decent compression because of repeating keys
- [X] What encodings does parquet-go actually use for your columns? Find out
      here rather than assuming — this determines exactly which decoders Stage 4
      and 5 need to implement.
      Answer: PLAIN, DELTA_LENGTH_BYTE_ARRAY, RLE
- [X] Row group sizes across your files: how many rows per row group does a
      typical flush produce? Relate that to your flush thresholds (10k/60s for
      journal, age-driven for pollers).
      Answer: journal has around 100 rows per row group, I checked cpu which has around 500. Imporant caveat is that the last days this machine hasn't been used that much. But 10000 is likely too large.
- [X] Are any columns' statistics *useful* for pruning — i.e. do min/max ranges
      on `ts` barely overlap between row groups, or do they all span the same
      window?

### Verify

- [X] `SELECT * FROM parquet_metadata('f.parquet');` — DuckDB gives one row per
      column chunk with `row_group_id`, `path_in_schema`, `compression`,
      `encodings`, `stats_min`/`stats_max`, `stats_null_count`,
      `total_compressed_size`, `data_page_offset`, `dictionary_page_offset`.
      Every field you decode must match.
- [X] `null_count` for a column must equal
      `SELECT count(*) - count(col) FROM 'f.parquet';`
- [X] `min_value`/`max_value` decoded through the schema must equal
      `SELECT min(col), max(col) FROM 'f.parquet';` — including for the string
      and timestamp columns, which is where naive byte interpretation breaks.
- [X] Sum of `total_compressed_size` across chunks, plus the footer length,
      plus 12 bytes of magic and length field, should leave only page headers
      unaccounted for — roughly 50 bytes per chunk. Check the arithmetic
      closes rather than shrugging at the gap.
      **Do NOT expect the footer to be small.** Measured 2026-08-26 on a real
      41-row journal file: 13740 bytes total = 9258 chunk data + 3640 footer +
      12 magic + 830 page headers. The footer was 26% of the file, because
      `Statistics` embeds min/max as raw bytes and 16 columns × 2 values
      includes whole JSON documents (`fields`) and long journald tokens
      (`cursor`). An earlier version of this item said "within a few hundred
      bytes", which is only true for a narrow schema with short values.
      Worth carrying back to machine-observability: a small age-driven flush
      pays a large fixed metadata cost, which is an argument for 5.4's
      compaction independent of the file-count math.

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

- [X] `zstd.parquet` — the same rows as `basic.parquet`, compressed with ZSTD.
      Two files with identical rows and different compression let you prove your
      decompression step is right: the decoded values must match exactly.
- [X] A file with enough rows that one column's values do not fit in a single
      page. You will not know how many rows that takes until you look, so
      generate, inspect, and adjust.

### Build

Two facts to have in hand before starting — neither is a task:

`ColumnMetaData` carries no page count and no page index. The only way to
enumerate a chunk's pages is to walk its bytes.

Parquet applies **two independent transforms** to a column, and they are easy to
conflate. An *encoding* turns values into bytes (`PLAIN`, `RLE`,
`DELTA_BINARY_PACKED`, `DELTA_LENGTH_BYTE_ARRAY`) and is Parquet's own spec. A
*compression codec* then squeezes those bytes (`ZSTD`, `GZIP`, `SNAPPY`,
`UNCOMPRESSED`) and is not Parquet-specific at all. Writing is
encode-then-compress; reading is decompress-then-decode. Every item below is one
stage of that pipeline, which is why they chain rather than substitute.

- [X] Given a column chunk, produce its pages — each one's header plus the bytes
      of its payload. The header is Thrift compact again, uncompressed, sitting
      immediately before the payload it describes.
- [X] Given one page, produce its uncompressed value bytes. Note that in a V2
      page the level bytes are stored *uncompressed* at the front, so the
      payload is not uniformly compressed and the result should be the values
      only. `klauspost/compress/zstd` is fine — the learning target is the
      format, not the compressor.
- [ ] Given uncompressed PLAIN INT64 bytes, produce numbers. Fixed 8 bytes per
      value, little-endian, back to back — no separators, no length prefix.
      This is the first of a family (other physical types, then RLE and the
      delta encodings in stage 5), so it wants its own file rather than a home
      in `page.go`.
      - [X] Reject a byte count that isn't a multiple of 8
      - [X] **Done when:** the decompressed bytes of `row_number` come back as
            1..100
- [X] Chain those three into one call that takes a column chunk and returns
      every value in it, in order. This is what `cat` needs, and it is the first
      place per-page results must be *accumulated* rather than returned
      directly. Two decisions fall out of it: what the intermediate results'
      lifetime should be, and where the zstd decoder gets created, given that
      one page is the wrong lifetime for it.
      - [X] **Done when:** a chunk spanning several pages returns the same
            values as the single-page equivalent
- [X] `sawdust cat <file> <column>` prints the values.
      - [X] **Done when:** its output matches
            `duckdb -c "SELECT <col> FROM 'f.parquet'"`

Deferred, not skipped: V1 data pages compress levels and values *together* in
one payload, where V2 keeps the levels uncompressed at the front. Every fixture
here and everything parquet-go writes is V2, so there is nothing to test V1
against yet. Revisit when a file from elsewhere needs it.

### Traps to hit deliberately

- [X] Decompress `compressed_page_size` bytes starting at the page *header*
      offset rather than after it. Read the error ZSTD gives you and learn what
      "wrong offset" looks like.
- [ ] Assume one page per chunk. Generate a fixture with enough rows to force
      several pages and watch a single-page reader silently return partial data.
      That silence is the point: nothing errors, you just get fewer rows.
- [ ] Use `uncompressed_page_size` as the amount to read from the file.

### Verify

- [X] `SELECT sum(c), count(c), min(c), max(c) FROM 'f.parquet';` must equal the
      same aggregates over your decoded slice. A checksum over the whole column
      catches ordering bugs that `sum` alone would hide — so also compare the
      first and last 10 values in order.
- [X] Decode the same column from `basic.parquet` and `zstd.parquet` and assert
      the two value slices are identical.
- [X] Decode from `many_rows.parquet` and confirm you get all rows, in row group
      order.
- [X] `single_row.parquet` and `empty.parquet` must both work — the empty file
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

- [X] `optionals_all_null.parquet` — every row leaves the optional fields unset,
      so those columns are null all the way down.
- [X] `optionals_never_null.parquet` — every row sets them, so those columns have
      no nulls at all.
- [X] These need a parameter on `buildRows` controlling whether the optional
      fields get filled, not a second `row` type. Same struct, different data.
- [X] A dictionary-encoded fixture. None of the current fixtures use
      `RLE_DICTIONARY` — `stat` shows `PLAIN`, `RLE` and
      `DELTA_LENGTH_BYTE_ARRAY` throughout — so the dictionary items below have
      nothing to run against until this exists. Getting parquet-go to choose a
      dictionary may take a writer option, a low-cardinality column, or both;
      `stat`'s encodings column tells you whether you succeeded. Pair it with a
      plain-encoded file holding the same values, the way `zstd.parquet` pairs
      with `basic.parquet`.

### Build

Facts to have in hand, none of them tasks:

**Values and rows are different counts.** A page's header gives both
`num_values` and `num_nulls`; only `num_values - num_nulls` values are actually
stored. Nothing in the value bytes marks where the gaps go — a separate stream
does.

**The RLE/bit-packing hybrid framing.** Each run begins with a varint header.
Its low bit selects the mode; the remaining bits are the length. Bit clear means
an RLE run of `header >> 1` repeats of one bit-packed value. Bit set means
`(header >> 1) * 8` bit-packed values. Values are packed LSB-first at a fixed
bit width.

**Bit width for definition levels** is `ceil(log2(maxDefLevel + 1))` — 1 bit for
a flat optional column. Note what the formula gives for a required column:
`maxDefLevel` is 0, so the width is 0 and no levels are stored at all. The
required case isn't a special case; it falls out of the same rule.

**The dictionary index bit width is data, not metadata.** It's the first byte of
the data page's value region, not something derivable from the schema.

Ordered so each item has what it needs from the one before:

- [X] Given RLE/bit-packed bytes and a bit width, produce integers. This is the
      foundation for two separate things — definition levels and dictionary
      indices — so it takes a bit width as input and knows nothing about either.
      - [X] **Done when:** it round-trips the definition-level stream of
            `even_row_number` into 100 levels of which 50 are 1
- [X] Given a page's definition levels and its stored values, produce
      `num_values` slots with the nulls in the right places. This is the step
      that reunites the two counts above, and getting it wrong shifts every
      value after the first null.
      - [X] **Done when:** `even_row_number` comes back as alternating
            value/null across all 100 rows
- [X] Decide how a column read returns different Go types. Stage 4 deferred this
      while `int64` was the only case; this stage has four more (byte array,
      double, boolean, and dictionary-indirected versions of each). The
      chunk-reading call from stage 4 has one return type and now needs several.
      Weigh a per-type method, a sealed interface like `LogicalType`, generics,
      and `any` — this is a design decision, not a lookup, and it shapes every
      item below it.
- [X] Given PLAIN BYTE_ARRAY bytes, produce values. Each is a 4-byte
      little-endian length followed by that many bytes.
- [X] Given PLAIN DOUBLE and BOOLEAN bytes, produce values. Doubles are IEEE 754
      little-endian; booleans are bit-packed LSB-first, so the byte count is not
      the value count.
- [X] Given a column chunk whose pages are dictionary-encoded, produce values.
      Two composition problems here, not one: the chunk's first page is a
      `DICTIONARY_PAGE` that must be decoded before any data page can be
      interpreted, and the chunk start moves from `data_page_offset` to
      `dictionary_page_offset`. The data pages then hold indices, which go
      through the RLE decoder and get mapped back to dictionary entries.
      - [X] **Done when:** a dictionary-encoded column and a plain-encoded
            column holding the same data decode to identical values
- [X] Given DELTA_LENGTH_BYTE_ARRAY bytes, produce values. **This is the
      largest remaining gap:** parquet-go writes it by default for every byte
      array, so 30 of the 94 chunks across your fixtures use it and are
      unreadable today. `plain.parquet` and `dict.parquet` are the only files
      whose byte-array columns you can read at all.
      The encoding is two regions: a DELTA_BINARY_PACKED block of lengths,
      followed by every value's bytes concatenated with no separators. So it
      needs the delta-binary-packed decoder first, even though no fixture uses
      that encoding on its own.
      DELTA_BINARY_PACKED is the most involved encoding in the format. Its
      header is four varints (block size, miniblocks per block, total count,
      first value). Each block then carries a zigzag min-delta, one bit-width
      byte per miniblock, and bit-packed deltas. Values come back by cumulative
      sum from the first value. Read `Encodings.md` in full before starting;
      this one is not derivable from the shape of the bytes.
      - [X] **Done when:** `category` from `basic.parquet` decodes identically
            to `category` from `plain.parquet` — the same pairing that verified
            the dictionary path
- [ ] Separate text byte arrays from opaque ones. Today every `ByteArrayValues`
      is rendered as text whether annotated or not — correct by accident for
      these fixtures, wrong for a column holding real binary. `1b 5b 32 4a` is
      the ANSI escape for "clear screen"; printed raw it wipes the terminal.
      Same post-`collect` shape the timestamp conversion uses.
      Detection must prefer `LogicalType` and fall back to `ConvertedType`,
      because either can be absent: modern writers emit `StringType` **and**
      `ConvertedUTF8`, older files carry only `ConvertedUTF8`, newer ones may
      carry only `StringType`. Checking `ConvertedType` first would misread a
      legacy annotation sitting beside a newer, more specific logical type.
      - [X] A fixture holding both kinds. A Go `[]byte` field produces an
            unannotated BYTE_ARRAY; a `string` field produces an annotated one.
            Add a third value to the existing `-kind` flag with its own struct,
            the way `nested` works — not a field on `row`, which is the struct
            behind eight fixtures and would change all their bytes.
      - [X] `StringValues []*string` variant
      - [X] Capture the annotation in `ReadColumn`'s schema lookup, beside where
            `logicalType` is already captured for timestamps
      - [X] Convert after `collect`: annotated → `StringValues`, unannotated →
            `ByteArrayValues` unchanged
      - [X] `cat` gains a case for each — text for `StringValues`, **hex** for
            `ByteArrayValues`, because hex cannot emit control characters
      - [X] **Done when:** the two columns of the new fixture print as text and
            as hex respectively

- [X] Given INT64 values and the leaf's logical type, produce `time.Time` in
      UTC. The logical type distinguishes millis, micros and nanos, and says
      whether the value is already UTC-adjusted.
- [ ] Repetition levels, depth 1 — **missing from this list until now**, though
      `nested.parquet` has had a repeated column since stage 0.

      The problem: `tags` is a `[]string`, so a row holds zero, one or two
      values. 100 rows produce 133 value slots, of which 100 hold values and 33
      mark rows whose list is empty. Definition levels alone cannot say which
      row a value belongs to, so reading it today returns a flat sequence with
      no grouping — 133 values that cannot be assigned to rows.

      Two streams, answering different questions, needed together. `def` says
      whether a slot holds a value. `rep` says whether a slot starts a new row
      or continues the previous one. Measured on `tags`:

      ```
      slot  rep  def   meaning
         0    0    1   new row, first value: tag-1-0
         1    0    1   new row, first value: tag-2-0
         2    1    1   same row, next value: tag-2-1
         3    0    0   new row, EMPTY list
      ```

      Slots 1 and 2 both have `def=1`, so definition levels alone treat them
      identically. Only `rep` separates "row 2's first tag" from "row 2's
      second tag". And slot 3 (`rep=0, def=0`) is an empty list, not a null —
      `applyDefinitionLevels` would emit a nil there, which is a different fact.

      Both streams use the same RLE/bit-packing hybrid, at widths derived from
      `MaxRepetitionLevel` and `MaxDefinitionLevel`, so `decodeRLE` already
      handles the bytes. What is missing is reading and using the rep stream.
      Note `RepetitionLevelsByteLength` comes **first** in `Page.Data`, before
      the definition levels.

      - [X] One variant, not one per element type:
            `ListValues{Elements ColumnValues; Offsets []int}` — row `i` spans
            `Elements[Offsets[i]:Offsets[i+1]]`, so `len(Offsets)` is
            `numRows + 1` and an empty list is `Offsets[i] == Offsets[i+1]`.
            This is how Arrow represents lists. Because `Elements` is itself a
            `ColumnValues`, the type already expresses arbitrary nesting depth
            without change — only the assembly function is depth-limited.
      - [X] One function, not two. Return the flat values **and** the offsets:
            `applyLevels[T](repLevels, defLevels []int64, values []T,
            maxRepLevel, maxDefLevel int64) ([]*T, []int, error)`.
            For a non-repeated column every rep level is 0, so offsets come out
            as `[0,1,2,…n]` and `ReadColumn` ignores them — which means this
            *replaces* `applyDefinitionLevels` rather than duplicating it.
            `repLevels` may be nil when `maxRepLevel` is 0.
      - [X] Guard `cursor == len(values)` and `len(offsets)-1 == numRows` at the
            end. The second is a new cross-check: `numRows` is a separate header
            field from `numValues`, and for every column read so far they have
            been equal, so it has never had anything to catch.
      - [X] `ReadColumn` branches on `MaxRepetitionLevel`: 0 wraps the flat
            values as today, 1 wraps both in `ListValues`, above 1 errors.
      - [X] `cat` gains a `ListValues` case. Match DuckDB's rendering —
            `[tag-2-0, tag-2-1]` — so the diff works without post-processing.
      - [X] **Done when:** `tags` from `nested.parquet` groups as
            `[tag-1-0]`, `[tag-2-0, tag-2-1]`, `[]`, `[tag-4-0]`, matching
            DuckDB row for row.

      Deferred to Stretch, deliberately rather than arbitrarily: nesting deeper
      than one level. See the Stretch entry for what it costs.

- [ ] `sawdust cat <file>` with no column argument emits one JSON object per
      row. That format is called NDJSON (newline-delimited JSON, also JSON
      Lines): one object per line, no wrapping array, no commas between lines,
      so each line parses on its own.

      ```
      {"id":1,"inner.a":10,"opt_in.a":null,"tags":["tag-1-0"]}
      {"id":2,"inner.a":20,"opt_in.a":200,"tags":["tag-2-0","tag-2-1"]}
      {"id":3,"inner.a":30,"opt_in.a":null,"tags":[]}
      ```

      **This lives in `cmd/sawdust`, not the library.** It boxes every value
      into `any`, which is what was rejected for `ReadColumn`'s return type —
      acceptable only because JSON stringifies everything anyway. A typed row
      API would be a real library feature and a stage 7 question.

      **Two facts about the shapes you already have.** A flat column is already
      row-aligned: `Int64Values` is `[]*int64` with one entry per row, so index
      `i` is row `i`. A `ListValues` is not — `tags` has 100 elements and 100
      rows, but row 1 owns elements 1 and 2, and that mapping lives in
      `Offsets`. That is the cost of having chosen one list variant over one per
      element type, and it surfaces here.

      So the work splits in two, and keeping them separate is what makes it
      manageable:

      - [X] **Per column: produce one `[]any` with one entry per row.** Two jobs
            in one pass. *Type erasure* — `[]*int64` and `[]*string` both become
            `[]any` — needed for every column so they can share a map and be
            indexed without a type switch per cell. *Applying offsets* — only
            for `ListValues`, where each row's slice of elements collapses into
            a single `[]any` entry. Handle `ListValues` before the type switch,
            recursing on `Elements` first to get one entry per element, then
            cutting that with the offsets. Name it for what it does to a single
            column — `perRow` or `alignToRows`, not `toRowValues`, which implies
            it makes records.
      - [X] **Then zip: for row `i`, read index `i` from every column.** Four
            lines, because the step above removed all the per-column indexing
            rules. Build a `map[string]any` and encode it; `encoding/json` sorts
            map keys, so field order is deterministic for free.
      - [X] Assert every column's slice has length `NumRows` before zipping. A
            miscounted column would otherwise pair wrong values silently — the
            same failure the `applyLevels` row-count guard prevents one layer
            down.
      - [X] **`[]byte` marshals to base64, not hex.** `{"raw":"ABsB/w=="}`
            rather than `{"raw":"001b01ff"}`, which contradicts what
            `cat <file> raw` prints. Convert with `hex.EncodeToString` during
            the per-column pass. `time.Time` needs nothing — it marshals to
            RFC3339 already.
      - [X] Nulls are JSON `null` here, not the string `"NULL"`. JSON has a real
            null and it cannot be confused with a string, so the two commands
            can legitimately differ — unlike the flat-versus-list case, where
            both were plain text.
      - [X] Repeating the six-line nil-preserving loop in every switch arm is
            the obvious first draft. One small generic helper taking a
            conversion function collapses each arm to a line. It is
            deduplication, not a new idea, and the same shape as `cat`'s
            existing value printer.
      - [X] **Done when:** NDJSON for `nested.parquet` matches a DuckDB export
            row for row — six leaves per record, `opt_in.*` null on odd rows,
            `tags` an array with row 3 empty.

**Null renders as the literal `NULL`, in flat columns and inside lists alike.**
Chosen over a blank line because blank is ambiguous: an empty string and an
empty byte array are legitimate values that render identically to a null, and
`raw.parquet` contains both. `NULL` collides only with a string whose value is
literally "NULL", which is the rarer case. No unquoted representation is fully
unambiguous — that is what CSV quoting exists for.

Comparisons against DuckDB must configure the oracle to match rather than the
other way round:
`COPY (SELECT …) TO 'f.csv' (HEADER false, NULLSTR 'NULL')`. Verified to produce
byte-identical output for `even_row_number`. A plain `::varchar` cast will not
match — it emits blank for NULL, DuckDB's own timestamp format rather than
RFC3339, and `\x`-escaped bytes rather than hex.

Deferred, not skipped: V1 data pages frame their levels differently — a 4-byte
little-endian length precedes each RLE stream, where V2 puts the lengths in the
header. Everything parquet-go writes is V2, so there is nothing here to test V1
against. Same deferral as stage 4.

### Traps to hit deliberately

- [X] Zip values to rows positionally, ignoring definition levels. On
      `optionals_all_null.parquet` and a mixed-null column, watch values land on
      the wrong rows — every row after the first null is shifted.
- [X] Assume the dictionary applies to the whole column chunk and that every
      data page uses it. Check `encoding_stats`: a chunk can fall back to PLAIN
      partway through when the dictionary grows too large.
- [X] Read the dictionary index bit width from the schema instead of from the
      page's first byte.

### Verify

- [X] `COPY (SELECT * FROM 'f.parquet') TO 'oracle.csv';` then have your reader
      emit the same rows in the same order and diff. Row-for-row equality is the
      Done-when; anything less hides a shift bug.
- [X] `optionals_all_null.parquet` and `optionals_never_null.parquet` must both
      round-trip, and `count(col)` from your output must equal DuckDB's.
- [X] Assert the mirror case on a fixture: a REQUIRED column stores no
      definition levels at all. `row_number` in any fixture already shows
      `definition_levels_byte_length: 0` — make it an assertion rather than an
      observation.
- [X] For the low-cardinality column, assert your decoded distinct set equals
      `SELECT DISTINCT col FROM 'f.parquet';` and that the dictionary-encoded
      and plain-encoded fixtures produce identical values.
- [X] Timestamps: `SELECT epoch_us(ts) FROM ...` compared against your
      `UnixMicro()`. Exact equality — no rounding tolerance.
- [X] Then the real test: a real `journal` file from the agent, with its many
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

**This stage's breakdown was worked out in conversation on 2026-09-04**, from
the goal and Done-when below. The four decisions are answered further down with
their reasoning; the checkboxes derive from those answers. Original intent was
that you write it cold — that turned out to be a missing rung, since five
fully-specified stages had been read and none written.

**Goal:** use the metadata from Stage 3 to avoid work in Stage 4/5. Given a
simple predicate on one column (`ts > X`, `unit = 'y'`), skip row groups whose
statistics prove they cannot match, and report how many bytes and pages you
avoided reading.

**This is an experiment, not a feature.** You would reach for DuckDB to query a
file, so sawdust does not need a query API. What it needs is to demonstrate
pruning and measure it. That framing is why the predicate stays a struct and
why the API question is deferred to Stage 7 — nothing here is load-bearing for
a caller.

### Facts to have in hand, none of them tasks

**Pruning never assumes or validates ordering.** It reads min and max from the
footer — two values per row group — and compares them to the predicate. No loop
over data. Sortedness is not a precondition; it is what makes pruning
*effective*. Clustered data gives narrow non-overlapping ranges, so the
comparison eliminates a lot. Scattered data gives wide overlapping ranges, so it
eliminates nothing. Either way the check costs two comparisons.

**The rule, per operator.** Skip the group when:

```
col > X    →  max <= X
col < X    →  min >= X
col = X    →  X < min  or  X > max
```

Measured on `many_rows.parquet` with `row_number > 250`: groups have max 100,
200, 300, so two of three are skipped and 1751 of 2627 bytes for that column are
never read.

**Strings work identically** — same rule, `bytes.Compare` instead of `<`.
Measured on the same file:

```
category = "buzz"      0% avoided   value inside every group's [bar .. foo]
category = "zzz"     100% avoided   value above every group's max
rand_id  = "id-0250"  67% avoided   ranges disjoint, value in one
```

What defeats pruning is overlapping ranges, not the column's type. A
low-cardinality randomly-distributed column has every group spanning the full
range, so any value that *actually exists* is inside every range.

Byte comparison is not collation — `'Z'` is 90 and `'a'` is 97, so uppercase
sorts first and nothing about locale or case-insensitivity applies. A predicate
a database answers case-insensitively prunes differently here.

**Four physical types, not six.** `StringType` and `TimestampType` are logical
refinements. Statistics store min/max as the *physical* type's bytes, so `ts`'s
max is 8 little-endian bytes of microseconds and decodes exactly like
`row_number`. Across all thirteen fixtures the physical types are `INT64`,
`DOUBLE`, `BOOLEAN`, `BYTE_ARRAY`.

**Pruning may only skip when it can prove no row matches.** Absence of evidence
is not evidence of absence. That makes "read it" the default and every skip a
positive claim.

**Filtering and pruning are different things.** Pruning skips *reading* groups
that cannot match — an optimization; omit it entirely and the answer is still
correct, just slower. Filtering drops non-matching rows from what you did read —
correctness; without it the answer is wrong however good the pruning. A
surviving group holds a mix: `row_number > 250` keeps group 2, which contains
rows 201–300 of which 50 match.

### Build — ordered so each item has what it needs

- [ ] **The predicate type.** A struct: one column, one operator, one value.
      `{Column string; Op Op; Value any}` with `Gt`, `Lt`, `Eq`. Cannot express
      `a > 1 AND b = 'x'`, which is correct for an experiment and would be
      gold-plating for a caller that does not exist.
- [ ] **Coerce the value once, up front,** into the column's physical
      representation — before touching any row group. `ReadColumn` already
      resolves the column and captures `logicalType`, so that is where it goes.
      A `time.Time` becomes micros/millis/nanos per the `TimeUnit`; a `string`
      becomes `[]byte`; an `int64` and a `bool` stay. Then every per-group check
      is a pure comparison with no type logic in it.
      - [ ] A type mismatch — a string for an INT64 column — errors here, before
            any I/O, with a message naming both types
      - [ ] **Done when:** a `time.Time` predicate on `ts` and an `int64`
            predicate on `row_number` both reach the comparison as int64
- [ ] **Four comparators**, one per physical type, each answering "can this
      group be skipped". `BYTE_ARRAY` is the easiest — min/max are already raw
      bytes, so `bytes.Compare` against the coerced value with no decoding.
      `INT64` and `DOUBLE` decode 8 bytes first; `BOOLEAN` is a single byte.
      The set of types pruning supports is exactly the set `ReadColumn`
      supports, because both need the same per-type decode — so an unsupported
      type fails in `ReadColumn` before pruning is reached.
- [ ] **Absent statistics mean read the group.** `Statistics` is a pointer and
      `MinValue`/`MaxValue` can each be nil. The nil check must produce "read
      it" *directly* — not fall through to a comparison on a zero default, which
      would make a nil max compare as 0, satisfy `max <= X`, and skip the group
      *because* its statistics were missing.
      - [ ] **Done when:** a column with no statistics returns every row
- [ ] **The row filter — build this first and get it correct alone.** Walk the
      decoded values, keep the ones satisfying the predicate, preserve file
      order. One pass, works for every operator. Not a sort: sorting changes the
      order DuckDB returns, only works for one-sided range predicates, and
      visits every element more than once to do less.
      - [ ] **Done when:** filtered rows are identical to
            `SELECT … WHERE …` from DuckDB, with no pruning enabled at all —
            this is the known-good baseline everything after compares against
- [ ] **Then the pruning**, skipping row groups the comparators reject.
      - [ ] **Done when:** the returned rows are *still* identical to DuckDB,
            and `BytesRead()` is lower than the unpruned path. If correctness
            breaks here you have a known-good path to diff against, which is why
            the filter came first.
- [ ] **The measurement.** `countingReaderAt` installed by `OpenFile`,
      `BytesRead()` on `File`. See the answered decision below for why the
      wrapper rather than a field incremented per call site.
      - [ ] **Done when:** the same query run with and without the predicate
            reports two different byte counts, and the difference matches the
            skipped groups' `TotalCompressedSize`
- [ ] **The finding.** Run it against a real day of journal data and write down
      what happened. The expected answer is that pruning saves nothing, because
      one flush is one file with one row group and rows arrive in `ts` order
      only by accident. That is the result, not a failure — and it is the
      measured justification for machine-observability §5.4's requirement that
      compaction sort by `ts` and set an explicit row group size.
      - [ ] **Done when:** the README states the measured saving on a real file
            and explains why it is what it is

### Traps to hit deliberately

- [ ] Prune with a nil `MaxValue` defaulted to zero and watch a group vanish
      because its statistics were absent. This is the one that returns wrong
      answers silently.
- [ ] Prune without filtering and compare row counts against DuckDB — 100 rows
      where 50 were asked for.
- [ ] Compute the saving from `TotalCompressedSize` of the surviving groups
      instead of from `BytesRead()`, then break the pruning so it reads a group
      it decided to skip. The calculated number will not notice.
- [ ] Predicate on `category` with a value that exists in the data and watch 0%
      pruned. Then use `'zzz'` and watch 100%. Same column, same code.

### Verify

- [ ] Row-for-row against DuckDB for at least one predicate per physical type
- [ ] `BytesRead()` with no predicate equals the whole-file read path
- [ ] A predicate matching nothing returns no rows and reads only the footer
- [ ] A predicate matching everything returns every row and reads everything —
      pruning must not skip a group it cannot rule out

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

  **Answered 2026-09-04: stop at row groups, and it is a dependency rather than
  a closed decision.** The structures do exist — parquet-go writes both for
  every chunk (`ColumnIndexOffset` and `OffsetIndexOffset`, fields 5 and 7 of
  `ColumnChunk`, which `readColumnChunk` currently skips). So the question is
  worth, not availability.

  Reasons to stop for now: page-level pruning is the *same mechanism* at finer
  granularity — compare a predicate against min/max, skip what cannot match —
  so it teaches nothing row-group pruning does not. It costs two new Thrift
  structs with the same required-field machinery as the other six. And it is
  purely additive later; nothing about row-group pruning changes to add it.

  **What would change the answer, and how to know.** Page statistics only
  subdivide anything when a chunk holds several pages. Every file measured so
  far is single-page per chunk — including the real 41-row `journal_test.parquet`
  — but that is because the files are small, not because Parquet looks like
  that. The arithmetic that decides it:

  ```
  parquet-go DefaultPageBufferSize = 256 KB
  journal_test.parquet, bytes per row:  fields 107, cursor 36, message 27, boot_id 6
  → fields stays single-page until a row group holds ~2,450 rows
  → cursor  until ~7,300;  boot_id until ~44,000
  ```

  So this depends on a decision not yet made in machine-observability: §5.4's
  compaction sets an explicit row group size. At 100,000 rows per group,
  `fields` gets roughly 40 pages per chunk and page-level pruning becomes
  genuinely useful. At 1,000 rows per group nothing is ever multi-page and it
  never helps.

  **Revisit trigger:** after compaction lands, measure pages per chunk on a
  compacted day. `ReadPages` already returns the page count, but `stat` reports
  chunks rather than pages — surfacing that is a small addition and the
  measurement that settles this. `multi_page.parquet` is the fixture to develop
  against, since it is the only one with multi-page chunks.

  Do NOT decide this from the fixtures. They are single-page because they hold
  100–300 rows.
- How do you *prove* the saving — count bytes read at the file-read boundary,
  or count pages decoded? Which number would convince a skeptic?

  **Answered 2026-09-04: bytes measured at the reader, via an always-on
  internal wrapper.**

  Bytes rather than pages, because bytes are the actual I/O and the thing that
  costs money on object storage. Page counts are a proxy.

  **Measured rather than calculated.** Summing `TotalCompressedSize` over the
  surviving row groups reports what you *intended* to read. It is derived from
  the same metadata that drove the pruning decision, so it cannot catch a bug
  in that decision — if you decide to skip a group but a code path reads it
  anyway, the calculated number still says you skipped it. Only a count taken
  at the reader disagrees. Same circularity as taking a test's expected value
  from the code under test.

  **The design:** `OpenFile` installs a `countingReaderAt` around the
  `*os.File` and `File` exposes `BytesRead() int64`. The wrapper satisfies
  `io.ReaderAt`, holds the real reader, and adds the returned byte count on the
  way through:

  ```go
  func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
      n, err := c.r.ReadAt(p, off)
      c.n += int64(n)
      return n, err
  }
  ```

  Single entry point, signature unchanged, API grows by one method. Cost is one
  `int64` add per `ReadAt`, which fires once per chunk plus three times for the
  footer.

  **Why the wrapper rather than a `bytesRead` field incremented at each call
  site.** Externally identical; internally the increment lives in one place
  instead of four, so a read added later is counted automatically. That is not
  hypothetical — the deferred page-index work adds two new reads per chunk, and
  under the per-call-site version whoever writes it has to remember two more
  increments or the measurement silently understates in the direction that
  flatters the pruning.

  **What this closes off:** an always-on internal wrapper solves counting, not
  wrapping in general. Injecting retry logic, read logging, or simulated I/O
  failures would still want an exported `NewFile(r io.ReaderAt, size int64)`.
  Not needed now; that constructor would also let tests stop reaching into the
  unexported `reader` field, which is the thing likely to pull it in later.

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

- [ ] **Repetition levels, arbitrary depth.** parquet-go writes depth 2 with a
      `list` struct tag — `[][]string` produces the leaf
      `nested.list.element.list.element` with `maxDef 2, maxRep 2` — so this is
      reachable, not theoretical. Two things are needed beyond depth 1.

      One offsets array per repetition level instead of a single one, driven by
      the rule that rep level `r` means "start a new element at nesting depth
      `r`, keep everything shallower". That part is mechanical.

      The harder part is that definition levels must say *where* the absence is.
      At depth 2, `def < maxDef` could mean the outer list is empty, the inner
      list is empty, or the element is null — three different results
      distinguished by which value `def` takes. Interpreting that needs, per
      leaf, the definition level at which each ancestor becomes defined, which
      means extending `Column` with per-ancestor metadata rather than just the
      two maxima. It is derivable from the walk `collectColumns` already does.

      This is Dremel record assembly, and it is the last genuinely hard
      algorithm in the format — a stack machine rather than a loop. Rough scale:
      the depth-1 version is around 25 lines; this is 60–80 plus the schema
      metadata.

      Nothing else depends on it. `ListValues` already nests, so the data model
      does not change — only the assembly function.
      - [ ] A `-kind` fixture using a `[][]string` field with the `list` tag
      - [ ] **Done when:** a depth-2 column round-trips against DuckDB

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
