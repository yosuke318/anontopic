"use client";

import { useCallback, useEffect, useReducer, useRef, useState } from "react";

import {
  clientFrame,
  parseRoomEvent,
  roomSocketUrl,
  type RoomConversation,
  type RoomErrorCode,
  type RoomEvent,
} from "@/lib/room";

// 接続が切れてから次に試すまでの待ち時間。試すたびに倍にして、この値で頭打ちにする。
const reconnectBaseDelayMs = 500;
const reconnectMaxDelayMs = 10000;

// 続けてこの回数つながらなかったら、試すのをやめて操作を利用者に返す。
const maxReconnectAttempts = 6;

// 一度もつながっていないうちは、拒否されている可能性が高いので早く諦める。
// ハンドシェイクが断られた理由はブラウザからは読めない。
const maxFirstConnectAttempts = 2;

// 送信したメッセージがこの時間で確認されなければ、届かなかったものとして扱う。
const sendTimeoutMs = 10000;

export type RoomStatus = "connecting" | "open" | "reconnecting" | "ended" | "unavailable";

// OutgoingFailure は送信が確認されなかった理由。timeout はサーバーから何も返って
// こなかったもので、それ以外はサーバーが返した error の code。
export type OutgoingFailure = RoomErrorCode | "timeout";

// NoticeKind は部屋の出来事のうち、発言でも入退室でもないもの。
export type NoticeKind = "reconnected" | RoomErrorCode;

type Identified = { id: string };

type MessageItem = { kind: "message"; participant: number; body: string; sentAt: string };
type PresenceItem = { kind: "presence"; participant: number; event: "joined" | "left" };
type NoticeItem = { kind: "notice"; notice: NoticeKind };
type OutgoingItem = {
  kind: "outgoing";
  body: string;
  startedAt: number;
  failure: OutgoingFailure | null;
};

type NewItem = MessageItem | PresenceItem | NoticeItem | OutgoingItem;

export type TimelineItem =
  | (MessageItem & Identified)
  | (PresenceItem & Identified)
  | (NoticeItem & Identified)
  | (OutgoingItem & Identified);

type RoomState = {
  status: RoomStatus;
  conversation: RoomConversation | null;
  self: number | null;
  present: number[];
  endReason: string | null;
  timeline: TimelineItem[];
  nextId: number;
};

const initialState: RoomState = {
  status: "connecting",
  conversation: null,
  self: null,
  present: [],
  endReason: null,
  timeline: [],
  nextId: 1,
};

type Action =
  | { type: "status"; status: RoomStatus }
  | { type: "event"; event: RoomEvent }
  | { type: "queued"; body: string; at: number }
  | { type: "resent"; id: string; at: number }
  | { type: "discarded"; id: string }
  | { type: "tick"; now: number };

function append(state: RoomState, item: NewItem): RoomState {
  return {
    ...state,
    nextId: state.nextId + 1,
    timeline: [...state.timeline, { ...item, id: String(state.nextId) }],
  };
}

// confirmSent は返ってきた自分の発言を、送信中のものと結びつけて消す。
//
// 部屋に流れるメッセージは送ったフレームを指す ID を持たないため、同じ本文で
// まだ確認されていないもののうち最も古いものを、その発言として扱う。
function confirmSent(state: RoomState, participant: number, body: string): RoomState {
  if (participant !== state.self) {
    return state;
  }

  const at = state.timeline.findIndex(
    (item) => item.kind === "outgoing" && item.failure === null && item.body === body,
  );
  if (at < 0) {
    return state;
  }

  return { ...state, timeline: state.timeline.filter((_, index) => index !== at) };
}

// failSending は受け付けられなかったフレームに理由を付ける。
//
// error もどのフレームへの返答かを持たない。サーバーは 1 つの接続のフレームを
// 順に読むため、まだ確認されていない最も古い送信が断られたものにあたる。送信中の
// ものが無い error は、部屋そのものが扱えなかったことを指す。
function failSending(state: RoomState, code: RoomErrorCode): RoomState {
  const at = state.timeline.findIndex((item) => item.kind === "outgoing" && item.failure === null);
  if (at < 0) {
    return append(state, { kind: "notice", notice: code });
  }

  return {
    ...state,
    timeline: state.timeline.map((item, index) =>
      index === at && item.kind === "outgoing" ? { ...item, failure: code } : item,
    ),
  };
}

function applyEvent(state: RoomState, event: RoomEvent): RoomState {
  switch (event.type) {
    case "joined":
      return {
        ...state,
        conversation: event.conversation,
        self: event.participant,
        present: event.present,
      };
    case "participant_joined":
    case "participant_left": {
      const next = { ...state, present: event.present };
      // 自分の入退室は接続状態として出しているため、部屋の流れには入れない。
      if (event.participant === state.self) {
        return next;
      }
      return append(next, {
        kind: "presence",
        participant: event.participant,
        event: event.type === "participant_joined" ? "joined" : "left",
      });
    }
    case "message":
      return append(confirmSent(state, event.participant, event.body), {
        kind: "message",
        participant: event.participant,
        body: event.body,
        sentAt: event.sentAt,
      });
    case "ended":
      return { ...state, status: "ended", endReason: event.reason, present: [] };
    case "error":
      return failSending(state, event.code);
  }
}

function reduce(state: RoomState, action: Action): RoomState {
  switch (action.type) {
    case "status": {
      if (state.status === action.status) {
        return state;
      }
      const next = { ...state, status: action.status };
      // つなぎ直している間に部屋へ流れたものは受け取れないため、そのことを残す。
      if (action.status === "open" && state.status === "reconnecting") {
        return append(next, { kind: "notice", notice: "reconnected" });
      }
      return next;
    }
    case "event":
      return applyEvent(state, action.event);
    case "queued":
      return append(state, {
        kind: "outgoing",
        body: action.body,
        startedAt: action.at,
        failure: null,
      });
    case "resent":
      return {
        ...state,
        timeline: state.timeline.map((item) =>
          item.id === action.id && item.kind === "outgoing"
            ? { ...item, startedAt: action.at, failure: null }
            : item,
        ),
      };
    case "discarded":
      return { ...state, timeline: state.timeline.filter((item) => item.id !== action.id) };
    case "tick": {
      let changed = false;
      const timeline = state.timeline.map((item) => {
        if (
          item.kind !== "outgoing" ||
          item.failure !== null ||
          action.now - item.startedAt <= sendTimeoutMs
        ) {
          return item;
        }
        changed = true;
        return { ...item, failure: "timeout" as const };
      });
      return changed ? { ...state, timeline } : state;
    }
  }
}

export type RoomSocket = RoomState & {
  // send はメッセージを送り、フレームを出せたかどうかを返す。
  send: (body: string) => boolean;
  resend: (id: string, body: string) => void;
  discard: (id: string) => void;
  // disconnect は接続を閉じ、つなぎ直しもやめる。
  disconnect: () => void;
  // retry は諦めた接続をもう一度試す。
  retry: () => void;
};

function reconnectDelayMs(attempt: number): number {
  return Math.min(reconnectBaseDelayMs * 2 ** (attempt - 1), reconnectMaxDelayMs);
}

// useRoomSocket は会話につないだ WebSocket を持ち、部屋の流れを組み立てる。
export function useRoomSocket(conversationId: string): RoomSocket {
  const [state, dispatch] = useReducer(reduce, initialState);
  const [generation, setGeneration] = useState(0);

  const socketRef = useRef<WebSocket | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  // 利用者が退出したか、会話が終わったかを閉じたときに読む。どちらもつなぎ直さない。
  const leftRef = useRef(false);
  const endedRef = useRef(false);

  useEffect(() => {
    let cancelled = false;
    let attempt = 0;
    let opened = false;

    function connect() {
      dispatch({ type: "status", status: attempt === 0 ? "connecting" : "reconnecting" });

      const socket = new WebSocket(roomSocketUrl(conversationId));
      socketRef.current = socket;

      socket.onopen = () => {
        attempt = 0;
        opened = true;
        dispatch({ type: "status", status: "open" });
      };

      socket.onmessage = (message) => {
        if (typeof message.data !== "string") {
          return;
        }
        const event = parseRoomEvent(message.data);
        if (event === null) {
          return;
        }
        if (event.type === "ended") {
          endedRef.current = true;
        }
        dispatch({ type: "event", event });
      };

      socket.onclose = () => {
        // 閉じたのが今つないでいる接続とは限らない。すでに次の接続に張り替えた後の
        // close で、その接続を手放さないようにする。
        if (socketRef.current === socket) {
          socketRef.current = null;
        }
        if (cancelled || leftRef.current || endedRef.current) {
          return;
        }

        attempt += 1;
        if (attempt > (opened ? maxReconnectAttempts : maxFirstConnectAttempts)) {
          dispatch({ type: "status", status: "unavailable" });
          return;
        }

        dispatch({ type: "status", status: "reconnecting" });
        timerRef.current = setTimeout(connect, reconnectDelayMs(attempt));
      };
    }

    connect();

    return () => {
      cancelled = true;
      clearTimeout(timerRef.current);
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [conversationId, generation]);

  const waiting = state.timeline.some((item) => item.kind === "outgoing" && item.failure === null);

  useEffect(() => {
    if (!waiting) {
      return;
    }
    const id = setInterval(() => dispatch({ type: "tick", now: Date.now() }), 1000);
    return () => clearInterval(id);
  }, [waiting]);

  const write = useCallback((body: string): boolean => {
    const socket = socketRef.current;
    if (socket === null || socket.readyState !== WebSocket.OPEN) {
      return false;
    }
    socket.send(clientFrame(body));
    return true;
  }, []);

  const send = useCallback(
    (body: string): boolean => {
      if (!write(body)) {
        return false;
      }
      dispatch({ type: "queued", body, at: Date.now() });
      return true;
    },
    [write],
  );

  const resend = useCallback(
    (id: string, body: string) => {
      if (write(body)) {
        dispatch({ type: "resent", id, at: Date.now() });
      }
    },
    [write],
  );

  const discard = useCallback((id: string) => dispatch({ type: "discarded", id }), []);

  const disconnect = useCallback(() => {
    leftRef.current = true;
    clearTimeout(timerRef.current);
    socketRef.current?.close();
    socketRef.current = null;
  }, []);

  const retry = useCallback(() => {
    leftRef.current = false;
    setGeneration((value) => value + 1);
  }, []);

  return { ...state, send, resend, discard, disconnect, retry };
}
