import { ApiError, apiBaseUrl, retryAfterSeconds } from "@/lib/api";

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
