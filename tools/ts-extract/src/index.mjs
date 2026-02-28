import { extractWithTsMorph } from "./tsmorph.mjs";
import { extractWithTSC } from "./tscapi.mjs";

function diag(level, code, message) {
  return { level, code, message };
}

function safeJsonParse(s) {
  try { return JSON.parse(s); } catch { return null; }
}

async function main() {
  const inputText = await new Promise((resolve) => {
    let d = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (c) => (d += c));
    process.stdin.on("end", () => resolve(d));
  });

  const req = safeJsonParse(inputText);
  if (!req?.repoRoot) {
    const out = {
      ok: false,
      toolVersion: "ts-extract.v0.1",
      engine: "none",
      diagnostics: [diag("error", "BAD_INPUT", "Missing repoRoot in request.")],
    };
    process.stdout.write(JSON.stringify(out));
    process.exit(0);
  }

  let diagnostics = [];
  try {
    const raw = await extractWithTsMorph(req, diagnostics);
    process.stdout.write(JSON.stringify({
      ok: true,
      toolVersion: "ts-extract.v0.1",
      engine: "ts-morph",
      diagnostics,
      raw
    }));
    return;
  } catch (e) {
    diagnostics.push(diag("warn", "TSMORPH_FAIL", String(e?.message || e)));
  }

  try {
    const raw = await extractWithTSC(req, diagnostics);
    process.stdout.write(JSON.stringify({
      ok: true,
      toolVersion: "ts-extract.v0.1",
      engine: "tsc-api",
      diagnostics,
      raw
    }));
  } catch (e) {
    diagnostics.push(diag("error", "EXTRACT_FAIL", String(e?.message || e)));
    process.stdout.write(JSON.stringify({
      ok: false,
      toolVersion: "ts-extract.v0.1",
      engine: "tsc-api",
      diagnostics
    }));
  }
}

main();
