# Digestron

> Large Language Models are powerful.
>
> But they are structurally blind.
>
> They see text.
>
> They do not see systems.
>
> Digestron builds a structural representation of your codebase
> and serves impact-aware context slices to LLMs.

---

## The Problem

Modern LLMs are excellent at autocompleting code.
They are poor at reasoning about the *structure* of the systems that code belongs to.

Without structural awareness, LLMs suffer from:

- **Context blindness** — they answer about a function without knowing what calls it
- **Structural regressions** — they refactor a method without seeing its callers break
- **Re-implementation** — they write a utility that already exists three modules away
- **Token waste** — they receive entire files when a single focused slice would suffice
- **Broken contracts** — they change a signature without tracing downstream impact

**Concrete example.** You ask an LLM to optimize `OrderProcessor.process`.
It does so efficiently. It does not know that `OrderProcessor.process` is called by
17 different modules, 4 of which pass arguments in a shape the new signature no longer accepts.
Three days later, production breaks.

Digestron prevents this.

---

## The Solution

Digestron builds a **Universal Symbol Graph (USG)** — a language-agnostic structural index of your codebase.

From this graph, Digestron can:

- **Discover symbols** — functions, classes, interfaces, types, with exact locations
- **Trace impact** — who calls what, who inherits from what, who instantiates what
- **Detect structural risk** — dynamic dispatch, computed property calls, eval patterns
- **Generate Focus Packs** — compact, LLM-ready context slices anchored to a seed symbol

The result: instead of pasting a 4,000-line file into a prompt, you paste a 300-token
focus pack that contains exactly the callers, callees, contracts, and risk flags
the model needs to reason correctly.

---

## Architecture Overview

```
Editor / LLM prompt
        ↓
  Focus Request  (digestron impact <symbol>)
        ↓
  Digestron Engine
        ↓
  Universal Symbol Graph  (.digestron/usg.v0.1.json)
        ↓
  Structured Context Pack  (text + JSON subgraph)
```

The USG is built once per indexing run and incrementally reusable.
The engine resolves symbols, traces call edges, and ranks context by structural proximity.

---

## Quickstart

### Requirements

- **Go ≥ 1.22** — builds the CLI
- **Node.js ≥ 18** — required for TypeScript extraction in v0.1

### Install Node dependencies for the TS extractor

```bash
cd tools/ts-extract && npm install
```

### Build the CLI

```bash
go build -o digestron ./cmd/digestron
```

### Check the environment

```bash
./digestron doctor /path/to/your/ts-repo
```

### Index a TypeScript repository

```bash
./digestron index /path/to/your/ts-repo
```

Output is written to `.digestron/usg.v0.1.json` inside the target repo.

### Search for a symbol

```bash
./digestron analyze OrderProcessor /path/to/your/ts-repo
```

### Get the impact Focus Pack for a symbol

```bash
./digestron impact OrderProcessor.process /path/to/your/ts-repo
./digestron impact OrderProcessor.process /path/to/your/ts-repo --radius 3 --budget-chars 12000
./digestron impact OrderProcessor.process /path/to/your/ts-repo --json
```

---

## Example Output

### `digestron analyze OrderProcessor`

```
Searching "OrderProcessor" in /path/to/repo
Found 2 match(es):

  1. [class]  src/core/order.ts::OrderProcessor
     id: sym_1f2a...
     sig: class OrderProcessor
     loc: src/core/order.ts:10

  2. [method] src/core/order.ts::OrderProcessor.process
     id: sym_77b1...
     sig: (order: Order) => Promise<Result>
     loc: src/core/order.ts:35
```

### `digestron impact OrderProcessor.process`

```
=== Focus Pack: src/core/order.ts::OrderProcessor.process (radius=2) ===
structuralConfidence: 0.82

[SEED] src/core/order.ts::OrderProcessor.process  [method]
  signature: (order: Order) => Promise<Result>
  loc: src/core/order.ts:35

[CALLERS] 4 found:
  src/api/order.ts::createOrder  [function]  @src/api/order.ts:22
  src/api/order.ts::retryOrder   [function]  @src/api/order.ts:58
  src/workers/fulfillment.ts::FulfillmentWorker.run  [method]  @src/workers/fulfillment.ts:89
  src/batch/processor.ts::BatchProcessor.flush  [method]  @src/batch/processor.ts:140

[CALLEES] 3 found:
  src/core/payment.ts::PaymentService.charge  [method]  @src/core/payment.ts:44
  src/core/inventory.ts::InventoryService.reserve  [method]  @src/core/inventory.ts:31
  src/core/order.ts::OrderProcessor.validate  [method]  @src/core/order.ts:120

[ENTRY POINTS] reachable:
  [node] src/server.ts

[RISK FLAGS] 1 nearby
  [dynamic_dispatch] @src/core/order.ts:78 — Computed property call obj[handlerName](...)

budget: 612/8000 chars used
```

---

## Structural Confidence Score

Every USG carries a `structuralConfidence` score, computed as:

```
structuralConfidence =
    (resolved_edges   / total_edges)   * 0.70
  + (resolved_symbols / total_symbols) * 0.20
  + (1 - dynamic_ratio)                * 0.10
```

This score reflects how much of the call graph was statically resolved versus
inferred or left dynamic. A score of `0.82` means the engine has high confidence
in the structural picture. A score below `0.60` warrants inspection of dynamic
patterns (eval, computed dispatch, heavy reflection).

The score is not a quality gate. It is a calibration signal.

---

## Philosophy

Digestron is not:

- A linter
- A compiler
- A language server
- A code generator
- A documentation tool

Digestron is:

**A structural reasoning engine.**

It makes the invisible visible: the shape of the system, the weight of a function,
the blast radius of a change. It does not tell you what to do with that information.
It ensures the model you're working with is not reasoning in the dark.

---

## Project Layout

```
cmd/digestron/          Go CLI (Cobra)
internal/usg/           USG types + I/O
internal/indexer/       Node child-process runner
internal/search/        Symbol search
internal/focus/         Focus Pack / impact slice algorithm
tools/ts-extract/       Node.js TypeScript extractor (ts-morph + tsc-api fallback)
docs/                   Specifications
```

---

## Documentation

- [USG v0.1 Spec](docs/SPEC_USG_V0.1.md)
- [TS Extractor v0.1 Spec](docs/SPEC_EXTRACTOR_V0.1.md)

---

## Roadmap

| Version | Focus |
|---------|-------|
| **v0.1** | TypeScript support, USG, CLI, Focus Pack |
| **v0.2** | VSCode extension — Focus Pack on demand |
| **v0.3** | Multi-language (Python, Go, Java) |
| **v1.0** | LLM-native tooling — structured context protocol |

---

## Why Node for TypeScript?

TypeScript analysis requires the TypeScript compiler's type checker to resolve
call targets, infer types, and distinguish overloads. No lightweight alternative
achieves the same accuracy.

Digestron uses Node.js as a controlled child process for this extraction step.
The Go engine orchestrates it, validates the output, and owns the USG lifecycle.

This is a deliberate trade-off: precision over purism.

