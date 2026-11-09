import { ApiError, apiBaseUrl, retryAfterSeconds } from "@/lib/api";
import type { Attributes } from "@/lib/attributes";

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

// issueSessionは匿名セッションを用意する。すでに有効なCookieがあれば期限が延び、
// 性別と年代の答えが置き換わる。
export async function issueSession(attributes: Attributes): Promise<void> {
  const res = await fetch(`${apiBaseUrl()}/api/session`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(attributes),
  });
  if (!res.ok) {
    throw new ApiError(res.status, retryAfterSeconds(res));
  }
}

// joinQueueは待機キューに入る。人数が揃っていればその場でルームが成立する。
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

// readMatchingStateは待機の状態を読む。どのキューにもいない場合はnullを返す。
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

// leaveQueueは待機をやめる。成立済みのルームは取り消さない。
export async function leaveQueue(): Promise<void> {
  const res = await fetch(`${apiBaseUrl()}/api/matching`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw new ApiError(res.status, retryAfterSeconds(res));
  }
}

// QueueStatusは1つのキュー（トピックとルーム人数の組）に待っている人がいるかどうか。
// 人数はAPIが返さないため持たない。
export type QueueStatus = {
  topic_id: number;
  room_type: RoomType;
  waiting: boolean;
};

function isQueueStatus(value: unknown): value is QueueStatus {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const status = value as QueueStatus;
  return (
    typeof status.topic_id === "number" &&
    roomTypes.includes(status.room_type) &&
    typeof status.waiting === "boolean"
  );
}

// fetchQueueStatusesは有効なトピックのキューごとに、待っている人がいるかを読む。
// セッションを持つ前に呼ぶため、Cookieは送らない。
export async function fetchQueueStatuses(): Promise<QueueStatus[]> {
  const res = await fetch(`${apiBaseUrl()}/api/matching/queues`, {
    credentials: "omit",
    cache: "no-store",
  });
  if (!res.ok) {
    throw new ApiError(res.status, retryAfterSeconds(res));
  }

  const body: unknown = await res.json();
  const queues = typeof body === "object" && body !== null && "queues" in body ? body.queues : null;
  if (!Array.isArray(queues) || !queues.every(isQueueStatus)) {
    throw new Error("queue status list is malformed");
  }
  return queues;
}
