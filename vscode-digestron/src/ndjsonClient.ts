import * as cp from "child_process";
import * as readline from "readline";

const PROTO_VERSION = "digestron.proto.v0.25";

export type DigestronRequest = {
  v: string;
  id: string;
  op: string;
  params?: Record<string, unknown>;
};

export type DigestronResponse = {
  v: string;
  id: string;
  ok: boolean;
  result?: unknown;
  error?: { code: string; message: string };
};

type Pending = {
  resolve: (v: DigestronResponse) => void;
  reject: (e: Error) => void;
  timer?: NodeJS.Timeout;
};

export class NDJSONClient {
  private proc: cp.ChildProcessWithoutNullStreams | null = null;
  private pending = new Map<string, Pending>();
  private nextId = 1;
  private rl?: readline.Interface;

  constructor(
    private binPath: string,
    private defaultRepoRoot: string,
    private onLog: (s: string) => void,
    private timeoutMs: number
  ) {}

  isRunning() {
    return !!this.proc && !this.proc.killed;
  }

  start() {
    if (this.isRunning()) return;

    this.onLog(`[digestron] starting: ${this.binPath} serve ${this.defaultRepoRoot}`);
    this.proc = cp.spawn(this.binPath, ["serve", this.defaultRepoRoot], {
      stdio: ["pipe", "pipe", "pipe"]
    });

    this.proc.on("error", (err) => {
      this.onLog(`[digestron] process error: ${err.message}`);
    });

    this.proc.on("exit", (code, signal) => {
      this.onLog(`[digestron] exited: code=${code} signal=${signal}`);
      for (const [id, p] of this.pending) {
        p.reject(new Error(`digestron server exited while request ${id} pending`));
      }
      this.pending.clear();
      this.proc = null;
    });

    this.proc.stderr.on("data", (buf: Buffer) => {
      const s = buf.toString("utf8").trimEnd();
      if (s) this.onLog(`[digestron:stderr] ${s}`);
    });

    this.rl = readline.createInterface({ input: this.proc.stdout });
    this.rl.on("line", (line) => this.handleLine(line));
  }

  stop() {
    if (!this.proc) return;
    this.onLog("[digestron] stopping...");
    try {
      this.proc.kill();
    } catch {
      // ignore errors when killing process
    }
    this.proc = null;
  }

  async request(op: string, params: Record<string, unknown> = {}): Promise<DigestronResponse> {
    this.start();
    if (!this.proc) throw new Error("digestron server not running");

    const id = String(this.nextId++);
    const req: DigestronRequest = {
      v: PROTO_VERSION,
      id,
      op,
      params
    };

    const line = JSON.stringify(req) + "\n";

    return new Promise((resolve, reject) => {
      const pending: Pending = { resolve, reject };

      pending.timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`digestron request timed out: op=${op} id=${id}`));
      }, this.timeoutMs);

      this.pending.set(id, pending);
      this.proc!.stdin.write(line, "utf8");
      this.onLog(`[digestron] >> ${line.trimEnd()}`);
    });
  }

  private handleLine(line: string) {
    const s = line.trim();
    if (!s) return;

    this.onLog(`[digestron] << ${s}`);
    let resp: DigestronResponse | null = null;

    try {
      resp = JSON.parse(s) as DigestronResponse;
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      this.onLog(`[digestron] bad json from server: ${msg}`);
      return;
    }

    const p = this.pending.get(resp.id);
    if (!p) return;

    if (p.timer) clearTimeout(p.timer);
    this.pending.delete(resp.id);
    p.resolve(resp);
  }
}
