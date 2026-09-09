import { apiBaseUrl } from "@/lib/api";

// 本文の長さの上限。これを超えるメッセージをサーバーはtoo_longで拒否する。
export const messageMaxLength = 2000;

export const roomErrorCodes = [
  "invalid_frame",
  "empty_body",
  "too_long",
  "blocked",
  "rate_limited",
  "unavailable",
] as const;

export type RoomErrorCode = (typeof roomErrorCodes)[number];

export type RoomConversation = {
  id: string;
  topicId: number;
  roomType: number;
  startedAt: string;
};

// RoomEventはサーバーから届くフレーム。クライアントが送るのはメッセージだけで、
// その形はclientFrameにある。
export type RoomEvent =
  | { type: "joined"; conversation: RoomConversation; participant: number; present: number[] }
  | { type: "participant_joined"; participant: number; present: number[] }
  | { type: "participant_left"; participant: number; present: number[] }
  | { type: "message"; participant: number; body: string; sentAt: string }
  | { type: "ended"; reason: string }
  | { type: "error"; code: RoomErrorCode; message: string };

// roomSocketUrlは会話につなぐWebSocketのURLを組み立てる。オリジンを別に指定
// できるようにしつつ、指定が無ければAPIと同じところにつなぐ。
export function roomSocketUrl(conversationId: string): string {
  const base = process.env.NEXT_PUBLIC_WS_BASE_URL ?? apiBaseUrl();
  const url = new URL(`/ws/rooms/${encodeURIComponent(conversationId)}`, base);
  url.protocol = url.protocol === "https:" || url.protocol === "wss:" ? "wss:" : "ws:";
  return url.toString();
}

// clientFrameはメッセージを1通送るフレームを組み立てる。
export function clientFrame(body: string): string {
  return JSON.stringify({ type: "message", body });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function participantOf(frame: Record<string, unknown>): number | null {
  const value = frame.participant;
  return typeof value === "number" && Number.isInteger(value) && value >= 1 ? value : null;
}

function presentOf(frame: Record<string, unknown>): number[] {
  const value = frame.present;
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((number): number is number => typeof number === "number");
}

function conversationOf(frame: Record<string, unknown>): RoomConversation | null {
  const value = frame.conversation;
  if (
    !isRecord(value) ||
    typeof value.id !== "string" ||
    typeof value.topic_id !== "number" ||
    typeof value.room_type !== "number" ||
    typeof value.started_at !== "string"
  ) {
    return null;
  }
  return {
    id: value.id,
    topicId: value.topic_id,
    roomType: value.room_type,
    startedAt: value.started_at,
  };
}

// parseRoomEventは1フレームを読む。読めないフレームにはnullを返し、画面は
// そのフレームを無かったものとして扱う。
export function parseRoomEvent(data: string): RoomEvent | null {
  let frame: unknown;
  try {
    frame = JSON.parse(data);
  } catch {
    return null;
  }
  if (!isRecord(frame) || typeof frame.type !== "string") {
    return null;
  }

  switch (frame.type) {
    case "joined": {
      const conversation = conversationOf(frame);
      const participant = participantOf(frame);
      if (conversation === null || participant === null) {
        return null;
      }
      return { type: "joined", conversation, participant, present: presentOf(frame) };
    }
    case "participant_joined":
    case "participant_left": {
      const participant = participantOf(frame);
      if (participant === null) {
        return null;
      }
      return { type: frame.type, participant, present: presentOf(frame) };
    }
    case "message": {
      const participant = participantOf(frame);
      if (participant === null || typeof frame.body !== "string") {
        return null;
      }
      const sentAt = typeof frame.sent_at === "string" ? frame.sent_at : "";
      return { type: "message", participant, body: frame.body, sentAt };
    }
    case "ended":
      return { type: "ended", reason: typeof frame.reason === "string" ? frame.reason : "" };
    case "error": {
      const code = roomErrorCodes.find((known) => known === frame.code);
      if (code === undefined) {
        return null;
      }
      return {
        type: "error",
        code,
        message: typeof frame.message === "string" ? frame.message : "",
      };
    }
    default:
      return null;
  }
}

const otherParticipantNames = ["A", "B", "C"];

// participantLabelは部屋の中だけで通じる呼び名を返す。参加者は会話ごとの番号でしか
// 区別できないため、自分以外を番号の昇順に並べて名前を振る。番号は会話が変われば
// 別の人を指す。
export function participantLabel(
  participant: number,
  self: number | null,
  roomType: number,
): string {
  if (participant === self) {
    return "あなた";
  }

  const others: number[] = [];
  for (let number = 1; number <= roomType; number += 1) {
    if (number !== self) {
      others.push(number);
    }
  }

  const at = others.indexOf(participant);
  const name = at < 0 ? undefined : otherParticipantNames[at];
  return name === undefined ? `参加者${participant}` : `参加者${name}`;
}
