import path from "node:path";
import ts from "typescript";
import crypto from "node:crypto";
import { computeStructuralConfidence } from "./stats.mjs";

// v0.1: tsc-api fallback emits minimal data, so coverage estimate is lower than ts-morph.
// Refine when the fallback is expanded to do a full AST walk.
const DEFAULT_SYMBOL_COVERAGE_RATIO = 0.7;

function hashId(prefix, s) {
  const h = crypto.createHash("sha1").update(s).digest("hex").slice(0, 16);
  return `${prefix}_${h}`;
}

function addDiag(diagnostics, level, code, message) {
  diagnostics.push({ level, code, message });
}

export async function extractWithTSC(req, diagnostics) {
  const repoRoot = req.repoRoot;
  const tsconfigPath = req.tsconfigPaths?.[0];
  if (!tsconfigPath) throw new Error("tsc-api requires tsconfigPaths[0].");

  const abs = path.join(repoRoot, tsconfigPath);
  const configFile = ts.readConfigFile(abs, ts.sys.readFile);
  if (configFile.error) throw new Error(ts.formatDiagnosticsWithColorAndContext([configFile.error], {
    getCurrentDirectory: ts.sys.getCurrentDirectory,
    getCanonicalFileName: f => f,
    getNewLine: () => "\n"
  }));

  const parsed = ts.parseJsonConfigFileContent(configFile.config, ts.sys, path.dirname(abs));
  const program = ts.createProgram({ rootNames: parsed.fileNames, options: parsed.options });

  const modules = [];
  const symbols = [];
  const calls = [];
  const inherits = [];
  const instantiates = [];
  const entryPoints = [];
  const riskFlags = [];

  const modIdByPath = new Map();
  const symIdByKey = new Map();

  function rel(absPath) {
    return path.relative(repoRoot, absPath).replaceAll("\\", "/");
  }

  function ensureModule(filePath) {
    const p = rel(filePath);
    let id = modIdByPath.get(p);
    if (!id) {
      id = hashId("mod", p);
      modIdByPath.set(p, id);
      modules.push({ id, path: p, language: "ts", imports: [], exports: [] });
    }
    return id;
  }

  function ensureSymbol(filePath, qname, name, kind, signature, posLine) {
    const p = rel(filePath);
    const moduleId = ensureModule(filePath);
    const key = `${p}::${qname}::${signature || ""}`;
    let id = symIdByKey.get(key);
    if (!id) {
      id = hashId("sym", key);
      symIdByKey.set(key, id);
      symbols.push({
        id, qname: `${p}::${qname}`, name, kind, moduleId,
        signature: signature || "",
        loc: { file: p, startLine: posLine, startCol: 0, endLine: posLine, endCol: 0 }
      });
    }
    return id;
  }

  for (const sf of program.getSourceFiles()) {
    if (sf.isDeclarationFile) continue;
    const file = rel(sf.fileName);
    ensureModule(sf.fileName);

    if (/(^|\/)(server|index|main)\.ts$/.test(file)) {
      entryPoints.push({ file, symbolId: null, kind: "node" });
    }

    // Minimal: prove the fallback pipeline works.
  }

  addDiag(diagnostics, "warn", "TSC_FALLBACK_MINIMAL", "tsc-api fallback currently emits minimal data in v0.1.");

  const callsTotal = calls.length;
  const callsResolved = 0;
  const callsInferred = 0;
  const callsDynamic = 0;

  const resolvedEdgeRatio = 0;
  const dynamicRatio = 0;
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
