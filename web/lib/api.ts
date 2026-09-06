// ブラウザから見た API のオリジン。セッション Cookie を送るため、fetch には
// credentials: "include" が必要になる。
export function apiBaseUrl(): string {
  return process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
}

// ApiError はステータスコードを呼び出し側に渡す。表示する文言は画面側が決める。
export class ApiError extends Error {
  readonly status: number;
  readonly retryAfterSeconds: number | null;

  constructor(status: number, retryAfterSeconds: number | null) {
    super(`api responded with ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

export function retryAfterSeconds(res: Response): number | null {
  const value = Number(res.headers.get("Retry-After"));
  return Number.isFinite(value) && value > 0 ? value : null;
}
