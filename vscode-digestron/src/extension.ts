import * as vscode from "vscode";
import { NDJSONClient } from "./ndjsonClient";
import { FocusDocProvider, FOCUS_PACK_URI } from "./focusDoc";

let client: NDJSONClient | null = null;
let lastFocusPackText: string | null = null;
let pinnedRepoRoot: string | null = null;

// Characters to scan on each side of the cursor when searching for dotted identifiers.
const SYMBOL_SEARCH_WINDOW = 80;

function getWorkspaceRoots(): string[] {
  const folders = vscode.workspace.workspaceFolders || [];
  return folders.map(f => f.uri.fsPath);
}

function getActiveRepoRoot(): string | null {
  const editor = vscode.window.activeTextEditor;
  if (!editor) {
    const roots = getWorkspaceRoots();
    return roots.length ? roots[0] : null;
  }
  const file = editor.document.uri.fsPath;
  const folders = vscode.workspace.workspaceFolders || [];
  for (const f of folders) {
    const root = f.uri.fsPath;
    if (file.startsWith(root)) return root;
  }
  const roots = getWorkspaceRoots();
  return roots.length ? roots[0] : null;
}

// Returns pinned root if set, otherwise falls back to active file's workspace folder.
function getRepoRootForRequest(): string | null {
  return pinnedRepoRoot || getActiveRepoRoot();
}

function symbolUnderCursor(): string {
  const editor = vscode.window.activeTextEditor;
  if (!editor) return "";

  const sel = editor.selection;
  if (!sel.isEmpty) return editor.document.getText(sel).trim();

  const pos = sel.active;
  const lineText = editor.document.lineAt(pos.line).text;

  // Look around cursor ±80 chars for a dotted identifier (e.g. foo.bar.baz)
  const start = Math.max(0, pos.character - SYMBOL_SEARCH_WINDOW);
  const end = Math.min(lineText.length, pos.character + SYMBOL_SEARCH_WINDOW);
  const windowText = lineText.slice(start, end);

  const dotted = /[A-Za-z_$][A-Za-z0-9_$]*(?:\.[A-Za-z_$][A-Za-z0-9_$]*)+/g;
  for (const m of windowText.matchAll(dotted)) {
    const tokenStart = start + (m.index ?? 0);
    const tokenEnd = tokenStart + m[0].length;
    if (pos.character >= tokenStart && pos.character <= tokenEnd) {
      return m[0];
    }
  }

  // Fallback to simple word at cursor
  const wordRange = editor.document.getWordRangeAtPosition(pos, /[A-Za-z0-9_.$]+/);
  if (!wordRange) return "";
  return editor.document.getText(wordRange).trim();
}

function getConfig<T>(key: string, def: T): T {
  const cfg = vscode.workspace.getConfiguration("digestron");
  return (cfg.get(key) as T) ?? def;
}

function ensureClient(output: vscode.OutputChannel, repoRoot: string): NDJSONClient {
  if (client && client.isRunning()) return client;

  const binPath = getConfig<string>("binaryPath", "digestron");
  const timeoutMs = getConfig<number>("requestTimeoutMs", 600000);

  client = new NDJSONClient(
    binPath,
    repoRoot,
    (s) => output.appendLine(s),
    timeoutMs
  );
  client.start();
  return client;
}

// Wraps a single request with one auto-restart retry if the server has died.
async function reqWithRestart(
  c: NDJSONClient,
  op: string,
  params: Record<string, unknown>,
  output: vscode.OutputChannel
): Promise<import("./ndjsonClient").DigestronResponse> {
  try {
    return await c.request(op, params);
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    output.appendLine(`[digestron] ${op} request failed, restarting once: ${msg}`);
    c.stop();
    c.start();
    return await c.request(op, params);
  }
}

export function activate(context: vscode.ExtensionContext) {
  const output = vscode.window.createOutputChannel("Digestron");
  output.appendLine("[digestron] extension activated");

  // ── Virtual document provider for focus pack viewer ──────────────────────
  const focusProvider = new FocusDocProvider();
  context.subscriptions.push(
    vscode.workspace.registerTextDocumentContentProvider("digestron", focusProvider)
  );

  async function openFocusDoc() {
    const doc = await vscode.workspace.openTextDocument(FOCUS_PACK_URI);
    await vscode.window.showTextDocument(doc, { preview: false });
  }

  context.subscriptions.push(output);

  // ── digestron.restartServer ───────────────────────────────────────────────
  context.subscriptions.push(vscode.commands.registerCommand("digestron.restartServer", async () => {
    output.show(true);
    if (client) client.stop();
    client = null;
    lastFocusPackText = null;
    vscode.window.showInformationMessage("Digestron server restarted.");
  }));

  // ── digestron.selectRepoRoot ──────────────────────────────────────────────
  context.subscriptions.push(vscode.commands.registerCommand("digestron.selectRepoRoot", async () => {
    const roots = getWorkspaceRoots();
    if (!roots.length) {
      vscode.window.showWarningMessage("No workspace folders.");
      return;
    }
    const pick = await vscode.window.showQuickPick(roots, { placeHolder: "Select Digestron repo root" });
    if (!pick) return;
    pinnedRepoRoot = pick;
    vscode.window.showInformationMessage(`Digestron repo root set: ${pick}`);
    // Restart the server so the next request uses the newly pinned root.
    if (client) client.stop();
    client = null;
    output.show(true);
    output.appendLine(`[digestron] repo root changed to ${pick}; server will restart on next request.`);
  }));

  // ── digestron.ensureIndexed ───────────────────────────────────────────────
  context.subscriptions.push(vscode.commands.registerCommand("digestron.ensureIndexed", async () => {
    output.show(true);
    try {
      const repoRoot = getRepoRootForRequest();
      if (!repoRoot) throw new Error("No workspace folder found");

      const autoIndex = getConfig<boolean>("autoIndex", true);
      const includeTests = getConfig<boolean>("includeTests", false);
      const includeJS = getConfig<boolean>("includeJS", false);

      const c = ensureClient(output, repoRoot);
      const resp = await reqWithRestart(c, "ensureIndexed", {
        repoRoot,
        autoIndex,
        reindexIfStale: autoIndex,
        includeTests,
        includeJS
      }, output);

      if (!resp.ok) {
        vscode.window.showErrorMessage(`Digestron ensureIndexed failed: ${resp.error?.code} ${resp.error?.message}`);
        return;
      }

      const result = resp.result as Record<string, unknown> | undefined;
      const source = result?.source;
      const indexed = result?.indexed;
      vscode.window.showInformationMessage(`Digestron: indexed=${indexed} source=${source}`);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      vscode.window.showErrorMessage(`Digestron: ${msg}`);
    }
  }));

  // ── digestron.impactUnderCursor ───────────────────────────────────────────
  context.subscriptions.push(vscode.commands.registerCommand("digestron.impactUnderCursor", async () => {
    output.show(true);
    try {
      const repoRoot = getRepoRootForRequest();
      if (!repoRoot) throw new Error("No workspace folder found");

      const ref = symbolUnderCursor();
      if (!ref) {
        vscode.window.showWarningMessage("No symbol under cursor / selection.");
        return;
      }

      const focusBudget = getConfig<number>("focusBudgetChars", 9000);
      const snippetsBudget = getConfig<number>("snippetsBudgetChars", 8000);
      const includeTests = getConfig<boolean>("includeTests", false);
      const includeJS = getConfig<boolean>("includeJS", false);
      const autoIndex = getConfig<boolean>("autoIndex", true);

      const c = ensureClient(output, repoRoot);

      const ensure = await reqWithRestart(c, "ensureIndexed", {
        repoRoot,
        autoIndex,
        reindexIfStale: autoIndex,
        includeTests,
        includeJS
      }, output);
      if (!ensure.ok) {
        vscode.window.showErrorMessage(`Digestron ensureIndexed failed: ${ensure.error?.code} ${ensure.error?.message}`);
        return;
      }

      const resp = await reqWithRestart(c, "impact", {
        repoRoot,
        ref,
        radius: 2,
        budgetChars: focusBudget,
        includeSnippets: true,
        snippetsBudgetChars: snippetsBudget
      }, output);

      if (!resp.ok) {
        vscode.window.showErrorMessage(`Digestron impact failed: ${resp.error?.code} ${resp.error?.message}`);
        return;
      }

      const result = resp.result as Record<string, unknown> | undefined;
      const focusText = (result?.focusText as string) ?? "";
      const focus = result?.focus as Record<string, unknown> | undefined;
      const snippets = focus?.snippets as Record<string, unknown> | undefined;
      const snipsText = (snippets?.text as string) ?? "";

      const composed = `${snipsText}\n${focusText}`.trim();

      lastFocusPackText = composed;

      await vscode.env.clipboard.writeText(composed);
      focusProvider.setContent(composed);
      await openFocusDoc();
      vscode.window.showInformationMessage(`Digestron: focus pack for "${ref}" copied to clipboard and opened.`);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      vscode.window.showErrorMessage(`Digestron: ${msg}`);
    }
  }));

  // ── digestron.copyLastFocusPack ───────────────────────────────────────────
  context.subscriptions.push(vscode.commands.registerCommand("digestron.copyLastFocusPack", async () => {
    output.show(true);
    if (!lastFocusPackText) {
      vscode.window.showWarningMessage("No focus pack available yet. Run Impact first.");
      return;
    }
    await vscode.env.clipboard.writeText(lastFocusPackText);
    vscode.window.showInformationMessage("Digestron: last focus pack copied to clipboard.");
  }));

  // ── digestron.openLastFocusPack ───────────────────────────────────────────
  context.subscriptions.push(vscode.commands.registerCommand("digestron.openLastFocusPack", async () => {
    output.show(true);
    if (!lastFocusPackText) {
      vscode.window.showWarningMessage("No focus pack available yet. Run Impact first.");
      return;
    }
    focusProvider.setContent(lastFocusPackText);
    await openFocusDoc();
  }));

  // ── digestron.openFocusPackScratch ────────────────────────────────────────
  context.subscriptions.push(vscode.commands.registerCommand("digestron.openFocusPackScratch", async () => {
    output.show(true);
    if (!lastFocusPackText) {
      vscode.window.showWarningMessage("No focus pack available yet. Run Impact first.");
      return;
    }
    const doc = await vscode.workspace.openTextDocument({ content: lastFocusPackText, language: "markdown" });
    await vscode.window.showTextDocument(doc, { preview: false });
  }));
}

export function deactivate() {
  if (client) client.stop();
  client = null;
}

