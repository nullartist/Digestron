import { DigestronNDJSONClient } from "./digestronClient.js";

type Entry = { client: DigestronNDJSONClient; lastUsed: number };

export class RepoManager {
  private map = new Map<string, Entry>();

  constructor(
    private binPath: string,
    private timeoutMs: number,
    private maxRepos: number,
    private log?: (s: string) => void
  ) {}

  get(repoRoot: string): DigestronNDJSONClient {
    const now = Date.now();

    const ex = this.map.get(repoRoot);
    if (ex) {
      ex.lastUsed = now;
      return ex.client;
    }

    // LRU eviction
    if (this.map.size >= this.maxRepos) {
      let oldestKey: string | null = null;
      let oldest = Infinity;
      for (const [k, v] of this.map) {
        if (v.lastUsed < oldest) {
          oldest = v.lastUsed;
          oldestKey = k;
        }
      }
      if (oldestKey) {
        this.log?.(`[mcp] evict repo client: ${oldestKey}`);
        this.map.get(oldestKey)?.client.stop();
        this.map.delete(oldestKey);
      }
    }

    const client = new DigestronNDJSONClient(this.binPath, repoRoot, this.timeoutMs, this.log);
    client.start();

    this.map.set(repoRoot, { client, lastUsed: now });
    return client;
  }

  stopAll() {
    for (const [, v] of this.map) v.client.stop();
    this.map.clear();
  }
}
