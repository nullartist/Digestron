# Digestron — Universal Symbol Graph (USG) v0.1

## Purpose
USG is a language-agnostic structural index of a codebase.
It enables:
- Symbol discovery (what exists)
- Impact analysis (who calls/depends on what)
- Focus Pack generation for LLM context
- Structural risk detection

USG v0.1 targets TypeScript first, but the schema is designed to extend to other languages.

---

## File Location
Generated output is written to:

- `.digestron/usg.v0.1.json`

---

## Top-level Schema

```json
{
  "version": "usg.v0.1",
  "root": "/abs/path/to/repo",
  "generatedAt": "ISO-8601",
  "language": ["ts"],
  "modules": [ ... ],
  "symbols": [ ... ],
  "edges": { "calls": [ ... ], "inherits": [ ... ], "instantiates": [ ... ] },
  "entryPoints": [ ... ],
  "riskFlags": [ ... ],
  "stats": { ... }
}
```

---

## Module

### Fields

* `id`: stable string id (hash)
* `path`: repo-relative path (posix)
* `language`: `"ts"` in v0.1
* `imports`: list of repo-relative module paths or specifiers
* `exports`: list of symbol IDs that are exported by this module

```json
{
  "id": "mod_...",
  "path": "src/core/order.ts",
  "language": "ts",
  "imports": ["src/core/payment", "zod"],
  "exports": ["sym_...", "sym_..."]
}
```

---

## Symbol

### Fields

* `id`: stable string id (hash)
* `qname`: fully-qualified name (file + symbol path)
* `name`: simple name
* `kind`: one of:

  * `function`, `class`, `interface`, `type`, `variable`, `method`, `constructor`
* `moduleId`: module id
* `signature`: short printable signature (may be empty if unknown)
* `loc`: source location

```json
{
  "id": "sym_...",
  "qname": "src/core/order.ts::OrderProcessor.process",
  "name": "process",
  "kind": "method",
  "moduleId": "mod_...",
  "signature": "(order: Order) => Promise<Result>",
  "loc": { "file":"src/core/order.ts", "startLine":35, "startCol":2, "endLine":78, "endCol":3 }
}
```

---

## Edges

### CallEdge

Represents a call site (resolved when possible).

Fields:

* `fromSymbolId`
* `toSymbolId` (nullable if external/unknown)
* `toExternal` (nullable; used for external calls)
* `loc`: `{file,line,col}`
* `confidence`: `resolved | inferred | dynamic`

```json
{
  "fromSymbolId":"sym_from",
  "toSymbolId":"sym_to",
  "toExternal": null,
  "loc":{"file":"src/a.ts","line":50,"col":8},
  "confidence":"resolved"
}
```

External call:

```json
{
  "fromSymbolId":"sym_from",
  "toSymbolId": null,
  "toExternal": { "module":"node:crypto", "name":"randomUUID" },
  "loc":{"file":"src/a.ts","line":62,"col":10},
  "confidence":"resolved"
}
```

### InheritanceEdge

Fields:

* `childSymbolId`
* `parentSymbolId`
* `confidence`

```json
{ "childSymbolId":"sym_child", "parentSymbolId":"sym_parent", "confidence":"resolved" }
```

### InstantiationSite

Fields:

* `symbolId` (class/constructor symbol)
* `loc`
* `confidence`

```json
{ "symbolId":"sym_class", "loc":{"file":"src/api.ts","line":12,"col":15}, "confidence":"resolved" }
```

---

## EntryPoints

Heuristic list of likely entrypoints.

Fields:

* `file`
* `symbolId` (nullable)
* `kind`: `node | web | test | cli`

```json
{ "file":"src/server.ts", "symbolId":"sym_main", "kind":"node" }
```

---

## RiskFlags

Fields:

* `loc`
* `kind`: `dynamic_dispatch | event_emitter | reflection | di_container | eval_like | monkey_patch`
* `note`: short explanation

```json
{
  "loc":{"file":"src/router.ts","line":90,"col":3},
  "kind":"dynamic_dispatch",
  "note":"Computed property call obj[handlerName](...)"
}
```

---

## Stats

Required fields:

* `totalModules`, `totalSymbols`
* `callsTotal`, `callsResolved`, `callsInferred`, `callsDynamic`
* `symbolCoverageRatio`
* `resolvedEdgeRatio`
* `dynamicRatio`
* `structuralConfidence`

StructuralConfidence v0.1:

```
resolvedEdgeRatio = callsResolved / callsTotal
dynamicRatio      = callsDynamic  / callsTotal
symbolCoverage    = symbolCoverageRatio

structuralConfidence =
  0.7*resolvedEdgeRatio +
  0.2*symbolCoverage +
  0.1*(1 - dynamicRatio)
```

---

## Guarantees (v0.1)

* Digestron never fabricates relationships.
* Unresolved/dynamic relationships must be explicitly labeled.
* Output must be usable by both humans and LLMs.
