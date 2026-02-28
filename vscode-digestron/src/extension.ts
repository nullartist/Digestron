import * as vscode from "vscode";
import { NDJSONClient } from "./ndjsonClient";

let client: NDJSONClient | null = null;
let lastFocusPackText: string | null = null;

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

function symbolUnderCursor(): string {
  const editor = vscode.window.activeTextEditor;
  if (!editor) return "";
  const sel = editor.selection;
  if (!sel.isEmpty) return editor.document.getText(sel).trim();

  const wordRange = editor.document.getWordRangeAtPosition(sel.active, /[A-Za-z0-9_.$]+/);
  if (!wordRange) return "";
  return editor.document.getText(wordRange).trim();
}

function getConfig<T>(key: string, def: T): T {
  const cfg = vscode.workspace.getConfiguration("digestron");
  return (cfg.get(key) as T) ?? def;
}

function ensureClient(output: vscode.OutputChannel): NDJSONClient {
  if (client && client.isRunning()) return client;

  const binPath = getConfig<string>("binaryPath", "digestron");
  const repo = getActiveRepoRoot();
  if (!repo) throw new Error("No workspace folder found");

  const timeoutMs = getConfig<number>("requestTimeoutMs", 600000);

  client = new NDJSONClient(
    binPath,
    repo,
    (s) => output.appendLine(s),
    timeoutMs
  );
  client.start();
  return client;
}

export function activate(context: vscode.ExtensionContext) {
  const output = vscode.window.createOutputChannel("Digestron");
  output.appendLine("[digestron] extension activated");

  context.subscriptions.push(output);

  context.subscriptions.push(vscode.commands.registerCommand("digestron.restartServer", async () => {
    output.show(true);
    if (client) client.stop();
    client = null;
    lastFocusPackText = null;
    vscode.window.showInformationMessage("Digestron server restarted.");
  }));

  context.subscriptions.push(vscode.commands.registerCommand("digestron.ensureIndexed", async () => {
    output.show(true);
    try {
      const repoRoot = getActiveRepoRoot();
      if (!repoRoot) throw new Error("No workspace folder found");

      const autoIndex = getConfig<boolean>("autoIndex", true);
      const includeTests = getConfig<boolean>("includeTests", false);
      const includeJS = getConfig<boolean>("includeJS", false);

      const c = ensureClient(output);
      const resp = await c.request("ensureIndexed", {
        repoRoot,
        autoIndex,
        reindexIfStale: autoIndex,
        includeTests,
        includeJS
      });

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

  context.subscriptions.push(vscode.commands.registerCommand("digestron.impactUnderCursor", async () => {
    output.show(true);
    try {
      const repoRoot = getActiveRepoRoot();
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

      const c = ensureClient(output);

      const ensure = await c.request("ensureIndexed", {
        repoRoot,
        autoIndex,
        reindexIfStale: autoIndex,
        includeTests,
        includeJS
      });
      if (!ensure.ok) {
        vscode.window.showErrorMessage(`Digestron ensureIndexed failed: ${ensure.error?.code} ${ensure.error?.message}`);
        return;
      }

      const resp = await c.request("impact", {
        repoRoot,
        ref,
        radius: 2,
        budgetChars: focusBudget,
        includeSnippets: true,
        snippetsBudgetChars: snippetsBudget
      });

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
      vscode.window.showInformationMessage(`Digestron: copied focus pack for "${ref}" to clipboard.`);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      vscode.window.showErrorMessage(`Digestron: ${msg}`);
    }
  }));

  context.subscriptions.push(vscode.commands.registerCommand("digestron.copyLastFocusPack", async () => {
    output.show(true);
    if (!lastFocusPackText) {
      vscode.window.showWarningMessage("No focus pack available yet. Run Impact first.");
      return;
    }
    await vscode.env.clipboard.writeText(lastFocusPackText);
    vscode.window.showInformationMessage("Digestron: last focus pack copied to clipboard.");
  }));
}

export function deactivate() {
  if (client) client.stop();
  client = null;
}
