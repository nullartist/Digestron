# Digestron — TypeScript Extractor (`tools/ts-extract`) v0.1

## Purpose
`ts-extract` produces a "raw structural model" of a TypeScript codebase:
- modules/import graph
- symbols with locations and signatures
- callsites (resolved when possible)
- inheritance edges
- instantiation sites
- entrypoints (heuristics)
- risk flags (heuristics)

It prioritizes correctness of labeling:
- resolved when TypeChecker confirms
- inferred for safe heuristics
- dynamic for unknown/dynamic dispatch

---

## Invocation
`ts-extract` is executed by Digestron (Go) as a child process.

Recommended:
- input via stdin JSON
- output via stdout JSON
- diagnostics to stderr (optional)

Example:
```bash
node tools/ts-extract/src/index.mjs < request.json
```

---

## Input (stdin JSON)

```json
{
  "repoRoot": "/abs/path",
  "tsconfigPaths": ["./tsconfig.json"],
  "includeTests": false,
  "maxFiles": 200000,
  "emit": {
    "modules": true,
    "symbols": true,
    "calls": true,
    "inherits": true,
    "instantiates": true,
    "entryPoints": true,
    "riskFlags": true
  }
}
```

Notes:

* If `tsconfigPaths` is empty, extractor should attempt autodetection from `repoRoot`.
* `maxFiles` is a safety limit.
* `includeTests` toggles whether files under typical test patterns are included.

---

## Output (stdout JSON)

```json
{
  "ok": true,
  "toolVersion": "ts-extract.v0.1",
  "engine": "ts-morph|tsc-api",
  "diagnostics": [
    { "level": "warn", "code": "TS_CONFIG_MULTI", "message": "Multiple tsconfig detected." }
  ],
  "raw": {
    "modules": [],
    "symbols": [],
    "calls": [],
    "inherits": [],
    "instantiates": [],
    "entryPoints": [],
    "riskFlags": [],
    "stats": {}
  }
}
```

If failure:

```json
{
  "ok": false,
  "toolVersion": "ts-extract.v0.1",
  "engine": "ts-morph|tsc-api",
  "diagnostics": [
    { "level": "error", "code": "EXTRACT_FAIL", "message": "..." }
  ]
}
```

---

## Engines

### Primary: ts-morph

Used for:

* symbol discovery
* locations
* signatures and basic types
* import graph
* AST traversal for callsites and instantiations

### Fallback: TypeScript Compiler API (tsc-api)

Used when:

* ts-morph project fails to resolve
* call resolution coverage is too low
* ts-morph crashes or hits memory limits

Extractor must report `engine` used.

---

## Confidence Classification

* `resolved`: TypeChecker could resolve target symbol or external signature.
* `inferred`: heuristic but safe (e.g., `new ClassName()` resolves class symbol by identifier).
* `dynamic`: computed property access, reflection patterns, unknown function expression calls.

Extractor must never mark `resolved` without TypeChecker confirmation.

---

## Risk Flag Heuristics (v0.1)

* `dynamic_dispatch`: obj[someExpr](...), map.get(k)(...)
* `eval_like`: eval(), new Function()
* `event_emitter`: emitter.on(...), addEventListener(...), process.on(...)
* `reflection`: Reflect.*, proxy traps
* `di_container`: container.get(...), inject(...)
* `monkey_patch`: assignment to prototype, overwriting imported members (heuristic)

These are "signals", not guaranteed bugs.
