export const roomTypes = [2, 3] as const;

export type RoomType = (typeof roomTypes)[number];

export type MatchingState = {
  state: "waiting" | "matched";
  topic_id: number;
  room_type: RoomType;
  waiting_since?: string;
  conversation?: {
    id: string;
    started_at: string;
  };
};

// ブラウザから見た API のオリジン。セッション Cookie を送るため、fetch には
// credentials: "include" が必要になる。
function apiBaseUrl(): string {
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

function retryAfterSeconds(res: Response): number | null {
  const value = Number(res.headers.get("Retry-After"));
  return Number.isFinite(value) && value > 0 ? value : null;
}

// issueSession は匿名セッションを用意する。すでに有効な Cookie があれば期限だけ延びる。
export async function issueSession(): Promise<void> {
  const res = await fetch(`${apiBaseUrl()}/api/session`, {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) {
    throw new ApiError(res.status, retryAfterSeconds(res));
  }
}

// joinQueue は待機キューに入る。人数が揃っていればその場でルームが成立する。
export async function joinQueue(topicId: number, roomType: RoomType): Promise<MatchingState> {
  const res = await fetch(`${apiBaseUrl()}/api/matching`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ topic_id: topicId, room_type: roomType }),
  });
  if (!res.ok) {
    throw new ApiError(res.status, retryAfterSeconds(res));
  }
  return (await res.json()) as MatchingState;
}

// readMatchingState は待機の状態を読む。どのキューにもいない場合は null を返す。
export async function readMatchingState(): Promise<MatchingState | null> {
  const res = await fetch(`${apiBaseUrl()}/api/matching`, {
    credentials: "include",
    cache: "no-store",
  });
  if (res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw new ApiError(res.status, retryAfterSeconds(res));
  }
  return (await res.json()) as MatchingState;
}

// leaveQueue は待機をやめる。成立済みのルームは取り消さない。
export async function leaveQueue(): Promise<void> {
  const res = await fetch(`${apiBaseUrl()}/api/matching`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw new ApiError(res.status, retryAfterSeconds(res));
  }
}
