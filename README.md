# Digestron

Digestron builds a structural representation of your codebase and serves impact-aware context slices to LLMs.

## What is Digestron?

Digestron indexes a TypeScript (and, eventually, multi-language) codebase into a **Universal Symbol Graph (USG)** — a language-agnostic structural index that enables:

- **Symbol discovery** – what functions, classes, interfaces, and types exist
- **Impact analysis** – who calls / depends on what
- **Focus Pack generation** – relevant context slices for LLM prompts
- **Structural risk detection** – dynamic dispatch, eval, reflection, etc.

## Quickstart

### Requirements

- **Go ≥ 1.22**
- **Node.js ≥ 18** (required for TypeScript extraction in v0.1)

### Install Node dependencies for the TS extractor

```bash
cd tools/ts-extract
npm install
```

### Build the CLI

```bash
go build -o digestron ./cmd/digestron
```

### Run the doctor (environment check)

```bash
./digestron doctor
```

### Index a TypeScript repository

```bash
./digestron index --root /path/to/your/ts-repo
```

Output is written to `.digestron/usg.v0.1.json` inside the target repo.

### Analyze / Impact

```bash
./digestron analyze --root /path/to/your/ts-repo
./digestron impact  --root /path/to/your/ts-repo --symbol "src/core/order.ts::OrderProcessor.process"
```

## Project layout

```
cmd/digestron/          Go CLI (Cobra)
internal/usg/           USG types + I/O
internal/indexer/       Node child-process runner
tools/ts-extract/       Node.js TypeScript extractor (ts-morph + tsc-api fallback)
docs/                   Specs
```

## Documentation

- [USG v0.1 Spec](docs/SPEC_USG_V0.1.md)
- [TS Extractor v0.1 Spec](docs/SPEC_EXTRACTOR_V0.1.md)
