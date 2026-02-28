import * as cp from "node:child_process";
import * as readline from "node:readline";

export type DigestronResponse = {
  v: string;
  id: string;
  ok: boolean;
  result?: any;
  error?: { code: string; message: string };
};

export class DigestronNDJSONClient {
  private proc: cp.ChildProcessWithoutNullStreams | null = null;
  private rl?: readline.Interface;
  private nextId = 1;
  private pending = new Map<string, { resolve: (v: DigestronResponse) => void; reject: (e: Error) => void; t?: NodeJS.Timeout }>();

  constructor(
    private binPath: string,
    private repoRoot: string,
    private timeoutMs: number,
    private onLog?: (s: string) => void
  ) {}

  start() {
    if (this.proc && !this.proc.killed) return;

    this.onLog?.(`[digestron] spawn: ${this.binPath} serve ${this.repoRoot}`);
    this.proc = cp.spawn(this.binPath, ["serve", this.repoRoot], { stdio: ["pipe", "pipe", "pipe"] });

    this.proc.on("exit", (code, sig) => {
      this.onLog?.(`[digestron] exited: code=${code} sig=${sig}`);
      for (const [id, p] of this.pending) p.reject(new Error(`digestron exited; pending id=${id}`));
      this.pending.clear();
      this.proc = null;
    });

    this.proc.on("error", (err) => {
      this.onLog?.(`[digestron] error: ${err.message}`);
    });

    this.proc.stderr.on("data", (b) => this.onLog?.(`[digestron:stderr] ${b.toString("utf8").trimEnd()}`));

    this.rl = readline.createInterface({ input: this.proc.stdout });
    this.rl.on("line", (line) => this.onLine(line));
  }

  stop() {
    if (!this.proc) return;
    try { this.proc.kill(); } catch {}
    this.proc = null;
  }

  async request(op: string, params: Record<string, any>): Promise<DigestronResponse> {
    this.start();
    if (!this.proc) throw new Error("digestron not running");

    const id = String(this.nextId++);
    const req = { v: "digestron.proto.v0.25", id, op, params };
    const line = JSON.stringify(req) + "\n";

    return new Promise((resolve, reject) => {
      const t = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`digestron timeout op=${op} id=${id}`));
      }, this.timeoutMs);

      this.pending.set(id, { resolve, reject, t });
      this.proc!.stdin.write(line, "utf8");
      this.onLog?.(`[digestron] >> ${line.trimEnd()}`);
    });
  }

  async requestWithRestart(op: string, params: Record<string, any>): Promise<DigestronResponse> {
    try {
      return await this.request(op, params);
    } catch (e) {
      this.onLog?.(`[digestron] request failed, restart once: ${(e as any)?.message || e}`);
      this.stop();
      this.start();
      return await this.request(op, params);
    }
  }

  private onLine(line: string) {
    const s = line.trim();
    if (!s) return;
    this.onLog?.(`[digestron] << ${s}`);

    let resp: DigestronResponse;
    try {
      resp = JSON.parse(s);
    } catch {
      return;
    }

    const p = this.pending.get(resp.id);
    if (!p) return;

    if (p.t) clearTimeout(p.t);
    this.pending.delete(resp.id);
    p.resolve(resp);
  }
}
