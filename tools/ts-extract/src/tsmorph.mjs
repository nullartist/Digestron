import path from "node:path";
import fs from "node:fs";
import crypto from "node:crypto";
import { Project, SyntaxKind } from "ts-morph";
import { computeStructuralConfidence } from "./stats.mjs";

// v0.1 placeholder: we enumerate all symbols from the AST, so coverage is high.
// Refine in a future version when we can compare against a reference symbol set.
const DEFAULT_SYMBOL_COVERAGE_RATIO = 0.9;

function hashId(prefix, s) {
  const h = crypto.createHash("sha1").update(s).digest("hex").slice(0, 16);
  return `${prefix}_${h}`;
}

function rel(repoRoot, absPath) {
  return path.relative(repoRoot, absPath).replaceAll("\\", "/");
}

function detectTsconfigs(repoRoot) {
  const p = path.join(repoRoot, "tsconfig.json");
  return fs.existsSync(p) ? ["./tsconfig.json"] : [];
}

export async function extractWithTsMorph(req, diagnostics) {
  const repoRoot = req.repoRoot;
  const tsconfigPaths = (req.tsconfigPaths?.length ? req.tsconfigPaths : detectTsconfigs(repoRoot));
  if (!tsconfigPaths.length) {
    throw new Error("No tsconfigPaths provided and tsconfig.json not found.");
  }

  const tsconfigAbs = path.join(repoRoot, tsconfigPaths[0]);
  const project = new Project({ tsConfigFilePath: tsconfigAbs });

  const sourceFilesAll = project.getSourceFiles()
    .filter(sf => !sf.isFromExternalLibrary() && !sf.getFilePath().endsWith(".d.ts"));

  if (sourceFilesAll.length > (req.maxFiles || 200000)) {
    throw new Error(`Too many files: ${sourceFilesAll.length} > maxFiles`);
  }

  let sourceFiles = sourceFilesAll;
  if (req.sampleFiles && req.sampleFiles > 0) {
    const score = (p) => {
      const f = p.toLowerCase();
      let s = 0;
      if (f.endsWith("/index.ts") || f.endsWith("/main.ts") || f.endsWith("/server.ts")) s += 50;
      s -= f.split("/").length;
      return s;
    };
    sourceFiles = [...sourceFiles].sort(
      (a, b) => score(rel(repoRoot, b.getFilePath())) - score(rel(repoRoot, a.getFilePath()))
    ).slice(0, req.sampleFiles);
    diagnostics.push({ level: "info", code: "SAMPLE_MODE", message: `Processing sampleFiles=${req.sampleFiles}` });
  }

  // Modules
  const modules = [];
  const moduleIdByPath = new Map();

  function ensureModule(sf) {
    const p = rel(repoRoot, sf.getFilePath());
    let id = moduleIdByPath.get(p);
    if (!id) {
      id = hashId("mod", p);
      moduleIdByPath.set(p, id);
      modules.push({ id, path: p, language: "ts", imports: [], exports: [] });
    }
    return id;
  }

  // Symbols
  const symbols = [];
  const symbolIdByQName = new Map();

  function ensureSymbol({ file, qname, name, kind, moduleId, signature, loc }) {
    const stableKey = `${file}::${qname}::${signature || ""}`;
    const id = hashId("sym", stableKey);
    if (!symbolIdByQName.has(qname)) {
      symbolIdByQName.set(qname, id);
      symbols.push({ id, qname, name, kind, moduleId, signature: signature || "", loc });
    }
    return symbolIdByQName.get(qname);
  }

  // Edges
  const calls = [];
  const inherits = [];
  const instantiates = [];
  const riskFlags = [];
  const entryPoints = [];

  for (const sf of sourceFiles) {
    const moduleId = ensureModule(sf);
    const file = rel(repoRoot, sf.getFilePath());

    // imports
    const mod = modules.find(m => m.id === moduleId);
    for (const imp of sf.getImportDeclarations()) {
      const spec = imp.getModuleSpecifierValue();
      mod.imports.push(spec);
    }

    // entrypoint heuristics (v0.1 simple)
    if (/(^|\/)(server|index|main)\.ts$/.test(file)) {
      entryPoints.push({ file, symbolId: null, kind: "node" });
    }

    // classes
    for (const cls of sf.getClasses()) {
      const clsName = cls.getName() || "<anonymous>";
      const clsQName = `${file}::${clsName}`;
      const loc = {
        file,
        startLine: cls.getStartLineNumber(),
        startCol: 0,
        endLine: cls.getEndLineNumber(),
        endCol: 0
      };
      const clsSymId = ensureSymbol({
        file,
        qname: clsQName,
        name: clsName,
        kind: "class",
        moduleId,
        signature: `class ${clsName}`,
        loc
      });

      const base = cls.getBaseClass();
      if (base) {
        const baseName = base.getName() || "<base>";
        const baseQName = `${file}::${baseName}`;
        const baseSymId = ensureSymbol({
          file,
          qname: baseQName,
          name: baseName,
          kind: "class",
          moduleId,
          signature: `class ${baseName}`,
          loc
        });
        inherits.push({ childSymbolId: clsSymId, parentSymbolId: baseSymId, confidence: "inferred" });
      }

      // methods
      for (const m of cls.getMethods()) {
        const mName = m.getName();
        const mQName = `${file}::${clsName}.${mName}`;
        const sig = m.getType().getText(m);
        const mLoc = { file, startLine: m.getStartLineNumber(), startCol: 0, endLine: m.getEndLineNumber(), endCol: 0 };
        const mSymId = ensureSymbol({
          file, qname: mQName, name: mName, kind: "method", moduleId,
          signature: sig, loc: mLoc
        });

        // call expressions inside method
        const callExprs = m.getDescendantsOfKind(SyntaxKind.CallExpression);
        for (const ce of callExprs) {
          const expr = ce.getExpression();
          // dynamic dispatch heuristic: obj[expr](...)
          if (expr.getKind() === SyntaxKind.ElementAccessExpression) {
            riskFlags.push({
              loc: { file, line: ce.getStartLineNumber(), col: 0 },
              kind: "dynamic_dispatch",
              note: "ElementAccess call obj[expr](...)"
            });
            calls.push({
              fromSymbolId: mSymId,
              toSymbolId: null,
              toExternal: null,
              loc: { file, line: ce.getStartLineNumber(), col: 0 },
              confidence: "dynamic"
            });
            continue;
          }

          // Try resolve via symbol declarations
          const exprSym = expr.getSymbol();
          const decls = exprSym?.getDeclarations();
          const decl = decls?.[0];
          if (decl) {
            const declSf = decl.getSourceFile();
            const declFile = rel(repoRoot, declSf.getFilePath());
            const declModuleId = ensureModule(declSf);

            const declName = exprSym.getName() || expr.getText();
            const declQName = `${declFile}::${declName}`;
            const declSig = decl.getType?.()?.getText(decl) ?? "";
            const declLoc = { file: declFile, startLine: decl.getStartLineNumber(), startCol: 0, endLine: decl.getEndLineNumber(), endCol: 0 };

            const toId = ensureSymbol({
              file: declFile,
              qname: declQName,
              name: declName,
              kind: "function",
              moduleId: declModuleId,
              signature: declSig,
              loc: declLoc
            });

            calls.push({
              fromSymbolId: mSymId,
              toSymbolId: toId,
              toExternal: null,
              loc: { file, line: ce.getStartLineNumber(), col: 0 },
              confidence: "resolved"
            });
          } else {
            calls.push({
              fromSymbolId: mSymId,
              toSymbolId: null,
              toExternal: null,
              loc: { file, line: ce.getStartLineNumber(), col: 0 },
              confidence: "inferred"
            });
          }
        }
      }
    }

    // new expressions (instantiation)
    const news = sf.getDescendantsOfKind(SyntaxKind.NewExpression);
    for (const ne of news) {
      const expr = ne.getExpression();
      const name = expr.getText();
      const qname = `${file}::${name}`;
      const symId = ensureSymbol({
        file,
        qname,
        name,
        kind: "class",
        moduleId,
        signature: `class ${name}`,
        loc: { file, startLine: ne.getStartLineNumber(), startCol: 0, endLine: ne.getStartLineNumber(), endCol: 0 }
      });
      instantiates.push({
        symbolId: symId,
        loc: { file, line: ne.getStartLineNumber(), col: 0 },
        confidence: "inferred"
      });
    }
  }

  // Stats
  const callsTotal = calls.length;
  const callsResolved = calls.filter(c => c.confidence === "resolved").length;
  const callsInferred = calls.filter(c => c.confidence === "inferred").length;
  const callsDynamic = calls.filter(c => c.confidence === "dynamic").length;

  const resolvedEdgeRatio = callsTotal ? callsResolved / callsTotal : 0;
  const dynamicRatio = callsTotal ? callsDynamic / callsTotal : 0;

  // v0.1: naive symbol coverage placeholder; refine when reference symbol set is available.
  const symbolCoverageRatio = DEFAULT_SYMBOL_COVERAGE_RATIO;

  const structuralConfidence = computeStructuralConfidence(resolvedEdgeRatio, symbolCoverageRatio, dynamicRatio);

  return {
    modules,
    symbols,
    calls,
    inherits,
    instantiates,
    entryPoints,
    riskFlags,
    stats: {
      totalModules: modules.length,
      totalSymbols: symbols.length,
      callsTotal,
      callsResolved,
      callsInferred,
      callsDynamic,
      symbolCoverageRatio,
      resolvedEdgeRatio,
      dynamicRatio,
      structuralConfidence
    }
  };
}
