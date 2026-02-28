import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";

import { RepoManager } from "./repoManager.js";

const BIN = process.env.DIGESTRON_BIN ?? "digestron";
const TIMEOUT = Number(process.env.DIGESTRON_TIMEOUT_MS ?? "600000");
const MAX_REPOS = Number(process.env.DIGESTRON_MAX_REPOS ?? "5");

const log = (s: string) => {
  // Keep logs minimal; MCP clients sometimes surface this
  // eslint-disable-next-line no-console
  console.error(s);
};

const repos = new RepoManager(BIN, TIMEOUT, MAX_REPOS, log);

const server = new McpServer({
  name: "digestron-mcp",
  version: "0.1.0"
});

/**
 * Tool: ensureIndexed
 */
server.tool(
  "digestron.ensureIndexed",
  {
    repoRoot: z.string().min(1),
    autoIndex: z.boolean().default(true),
    reindexIfStale: z.boolean().default(true),
    includeTests: z.boolean().default(false),
    includeJS: z.boolean().default(false)
  },
  async ({ repoRoot, autoIndex, reindexIfStale, includeTests, includeJS }) => {
    const c = repos.get(repoRoot);
    const resp = await c.requestWithRestart("ensureIndexed", {
      repoRoot,
      autoIndex,
      reindexIfStale,
      includeTests,
      includeJS
    });

    if (!resp.ok) {
      return {
        content: [{ type: "text", text: `ensureIndexed error: ${resp.error?.code} ${resp.error?.message}` }],
        isError: true
      };
    }

    return {
      content: [
        { type: "text", text: JSON.stringify(resp.result, null, 2) }
      ]
    };
  }
);

/**
 * Tool: impact (with snippets)
 */
server.tool(
  "digestron.impact",
  {
    repoRoot: z.string().min(1),
    ref: z.string().min(1),
    radius: z.number().int().min(1).max(6).default(2),
    focusBudgetChars: z.number().int().min(1000).max(50000).default(9000),
    includeSnippets: z.boolean().default(true),
    snippetsBudgetChars: z.number().int().min(1000).max(50000).default(8000)
  },
  async ({ repoRoot, ref, radius, focusBudgetChars, includeSnippets, snippetsBudgetChars }) => {
    const c = repos.get(repoRoot);

    // Make it plug&play: ensureIndexed first (autoIndex true by default)
    const ensured = await c.requestWithRestart("ensureIndexed", {
      repoRoot,
      autoIndex: true,
      reindexIfStale: true,
      includeTests: false,
      includeJS: false
    });
    if (!ensured.ok) {
      return {
        content: [{ type: "text", text: `ensureIndexed failed: ${ensured.error?.code} ${ensured.error?.message}` }],
        isError: true
      };
    }

    const resp = await c.requestWithRestart("impact", {
      repoRoot,
      ref,
      radius,
      budgetChars: focusBudgetChars,
      includeSnippets,
      snippetsBudgetChars
    });

    if (!resp.ok) {
      return {
        content: [{ type: "text", text: `impact error: ${resp.error?.code} ${resp.error?.message}` }],
        isError: true
      };
    }

    // Return both: human text + machine JSON (as text for MCP clients)
    const focusText = resp.result?.focusText ?? "";
    const focusJSON = resp.result?.focus ?? {};
    const snippetsText = focusJSON?.snippets?.text ?? "";

    const composed = `${snippetsText}\n${focusText}`.trim();

    return {
      content: [
        { type: "text", text: composed },
        { type: "text", text: "\n---\nFOCUS_JSON\n" + JSON.stringify(focusJSON, null, 2) }
      ]
    };
  }
);

/**
 * Tool: snippets
 */
server.tool(
  "digestron.snippets",
  {
    repoRoot: z.string().min(1),
    budgetChars: z.number().int().min(500).max(50000).default(8000),
    requests: z.array(
      z.object({
        file: z.string().min(1),
        startLine: z.number().int().optional(),
        endLine: z.number().int().optional(),
        line: z.number().int().optional(),
        contextLines: z.number().int().optional(),
        label: z.string().optional(),
        priority: z.number().int().optional()
      })
    ).min(1)
  },
  async ({ repoRoot, budgetChars, requests }) => {
    const c = repos.get(repoRoot);
    const resp = await c.requestWithRestart("snippets", { repoRoot, budgetChars, requests });

    if (!resp.ok) {
      return {
        content: [{ type: "text", text: `snippets error: ${resp.error?.code} ${resp.error?.message}` }],
        isError: true
      };
    }

    return {
      content: [{ type: "text", text: resp.result?.text ?? "" }]
    };
  }
);

process.on("SIGINT", () => {
  repos.stopAll();
  process.exit(0);
});
process.on("SIGTERM", () => {
  repos.stopAll();
  process.exit(0);
});

const transport = new StdioServerTransport();
await server.connect(transport);
