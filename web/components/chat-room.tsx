"use client";

import { useRouter } from "next/navigation";
import { type FormEvent, type KeyboardEvent, useEffect, useRef, useState } from "react";

import { ReportDialog } from "@/components/report-dialog";
import { joinQueue, leaveQueue, type RoomType } from "@/lib/matching";
import { messageMaxLength, participantLabel } from "@/lib/room";
import type { Topic } from "@/lib/topics";
import {
  useRoomSocket,
  type NoticeKind,
  type OutgoingFailure,
  type RoomStatus,
  type TimelineItem,
} from "@/lib/use-room-socket";

const statusLabels: Record<RoomStatus, string> = {
  connecting: "接続しています",
  open: "接続中",
  reconnecting: "つなぎ直しています",
  ended: "終了しました",
  unavailable: "接続できません",
};

const failureMessages: Record<OutgoingFailure, string> = {
  blocked:
    "この内容は送信できません。連絡先の交換や禁止している表現が含まれていないか確認してください。",
  rate_limited: "続けて送りすぎました。少し待ってから送り直してください。",
  too_long: `本文が長すぎます。${messageMaxLength} 文字までにしてください。`,
  empty_body: "本文が空のため送信できませんでした。",
  invalid_frame: "送信した内容をサーバーが読み取れませんでした。",
  unavailable: "サーバーの都合で送信できませんでした。少し待ってから送り直してください。",
  timeout: "送信を確認できませんでした。相手に届いていない可能性があります。",
};

const noticeMessages: Record<NoticeKind, string> = {
  reconnected: "接続が戻りました。切れていた間に流れたメッセージは表示されません。",
  blocked: "送信した内容は配信されませんでした。",
  rate_limited: "送信が続いたため、いくつかのメッセージが配信されませんでした。",
  too_long: "本文が長すぎるメッセージは配信されませんでした。",
  empty_body: "本文が空のメッセージは配信されませんでした。",
  invalid_frame: "サーバーが読み取れない送信がありました。",
  unavailable: "この部屋をいま扱えませんでした。時間をおいて開き直してください。",
};

const endedMessages: Record<string, string> = {
  user_left: "参加者がいなくなったため、会話は終了しました。",
};

const endedFallback = "会話は終了しました。";

const unavailableLead =
  "会話につなげませんでした。すでに終わっている会話か、通信が届いていない可能性があります。";

const emptyLead = "まだ発言がありません。最初のひとことを送ってみてください。";

const blockedBody = "ブロックした参加者の発言です。";

const composerPlaceholder = "メッセージを入力";

const primaryButtonClass =
  "bg-brand text-brand-contrast hover:bg-brand-hover inline-flex h-11 items-center justify-center rounded-full px-6 text-sm font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50";

const secondaryButtonClass =
  "border-line hover:bg-surface inline-flex h-11 items-center justify-center rounded-full border px-5 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50";

const chipButtonClass =
  "border-line hover:bg-surface rounded-full border px-2.5 py-1 text-xs font-medium transition-colors";

// 発言の吹き出しをスクロールの下端とみなす幅。ここより下に居る間は新着で追従する。
const followThresholdPx = 80;

function formatTime(sentAt: string): string {
  const at = new Date(sentAt);
  if (Number.isNaN(at.getTime())) {
    return "";
  }
  return `${String(at.getHours()).padStart(2, "0")}:${String(at.getMinutes()).padStart(2, "0")}`;
}

// roomTypeOfは次に入るキューの人数を決める。3人ルームで待っていても2人で成立する
// ことがあるため、いま居るルームの人数をそのまま引き継ぐ。
function roomTypeOf(roomType: number): RoomType {
  return roomType === 3 ? 3 : 2;
}

function Line({ children }: { children: string }) {
  return <p className="text-muted px-4 py-2 text-center text-xs leading-6">{children}</p>;
}

export function ChatRoom({ conversationId, topics }: { conversationId: string; topics: Topic[] }) {
  const router = useRouter();
  const room = useRoomSocket(conversationId);

  const [draft, setDraft] = useState("");
  const [blocked, setBlocked] = useState<number[]>([]);
  const [reporting, setReporting] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [following, setFollowing] = useState(true);

  const listRef = useRef<HTMLDivElement>(null);
  const formRef = useRef<HTMLFormElement>(null);
  const followRef = useRef(true);

  const topicId = room.conversation?.topicId;
  const topicName = topics.find((topic) => topic.id === topicId)?.name;

  const roomType = room.conversation?.roomType ?? 2;

  // 参加者の番号は接続して初めて分かる。つながる前は誰も並べない。
  const others: number[] = [];
  if (room.self !== null) {
    for (let number = 1; number <= roomType; number += 1) {
      if (number !== room.self) {
        others.push(number);
      }
    }
  }

  useEffect(() => {
    const list = listRef.current;
    if (list === null || !followRef.current) {
      return;
    }
    list.scrollTop = list.scrollHeight;
  }, [room.timeline]);

  function handleScroll() {
    const list = listRef.current;
    if (list === null) {
      return;
    }
    const near = list.scrollHeight - list.scrollTop - list.clientHeight < followThresholdPx;
    followRef.current = near;
    setFollowing(near);
  }

  function scrollToLatest() {
    const list = listRef.current;
    if (list === null) {
      return;
    }
    followRef.current = true;
    setFollowing(true);
    list.scrollTop = list.scrollHeight;
  }

  // ブロックはこの画面の中だけで持ち、サーバーには送らない。相手にも伝わらない。
  // 理由はdocs/adr/0016-block-a-participant-in-the-browser-only.mdにある。
  function toggleBlock(participant: number) {
    setBlocked((current) =>
      current.includes(participant)
        ? current.filter((number) => number !== participant)
        : [...current, participant],
    );
  }

  // leaveは接続を閉じ、待機の割り当ても返す。会話そのものは、参加者が居なくなって
  // から猶予を過ぎたときにサーバー側で終わる。
  async function leave(next: "topics" | "queue") {
    setLeaving(true);
    room.disconnect();
    try {
      await leaveQueue();
    } catch {
      // 割り当てを返せなくても、待機の期限で外れる。トピックの選び直しには進める。
    }

    const conversation = room.conversation;
    if (next === "queue" && conversation !== null) {
      try {
        await joinQueue(conversation.topicId, roomTypeOf(conversation.roomType));
        router.push("/waiting");
        return;
      } catch {
        // 待機に入れなかった場合は、トピックから選び直してもらう。
      }
    }
    router.push("/topics");
  }

  const trimmed = draft.trim();
  const length = [...trimmed].length;
  const tooLong = length > messageMaxLength;
  const canSend = room.status === "open" && trimmed !== "" && !tooLong;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canSend) {
      return;
    }
    if (room.send(trimmed)) {
      setDraft("");
      scrollToLatest();
    }
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    // 変換中のEnterは確定なので、送信に使わない。
    if (event.key !== "Enter" || event.shiftKey || event.nativeEvent.isComposing) {
      return;
    }
    event.preventDefault();
    formRef.current?.requestSubmit();
  }

  function renderItem(item: TimelineItem) {
    if (item.kind === "notice") {
      return <Line key={item.id}>{noticeMessages[item.notice]}</Line>;
    }

    if (item.kind === "presence") {
      const label = participantLabel(item.participant, room.self, roomType);
      return (
        <Line key={item.id}>
          {item.event === "joined" ? `${label}が入室しました` : `${label}が退出しました`}
        </Line>
      );
    }

    if (item.kind === "outgoing") {
      return (
        <div key={item.id} className="flex flex-col items-end gap-1">
          <div
            className={`max-w-[85%] rounded-2xl border px-4 py-2.5 text-sm leading-7 whitespace-pre-wrap ${
              item.failure === null ? "border-line text-muted border-dashed" : "border-danger"
            }`}
          >
            {item.body}
          </div>
          {item.failure === null ? (
            <span className="text-muted text-xs">送信中…</span>
          ) : (
            <div className="flex max-w-[85%] flex-col items-end gap-1">
              <span className="text-danger text-xs leading-5">{failureMessages[item.failure]}</span>
              <div className="flex gap-2">
                {item.failure !== "blocked" && (
                  <button
                    type="button"
                    onClick={() => room.resend(item.id, item.body)}
                    className={chipButtonClass}
                  >
                    送り直す
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => room.discard(item.id)}
                  className={chipButtonClass}
                >
                  消す
                </button>
              </div>
            </div>
          )}
        </div>
      );
    }

    if (blocked.includes(item.participant)) {
      return <Line key={item.id}>{blockedBody}</Line>;
    }

    const mine = item.participant === room.self;
    const time = formatTime(item.sentAt);

    return (
      <div key={item.id} className={`flex flex-col gap-1 ${mine ? "items-end" : "items-start"}`}>
        {!mine && (
          <span className="text-muted px-1 text-xs font-medium">
            {participantLabel(item.participant, room.self, roomType)}
          </span>
        )}
        <div
          className={`max-w-[85%] rounded-2xl px-4 py-2.5 text-sm leading-7 whitespace-pre-wrap ${
            mine ? "bg-brand text-brand-contrast" : "bg-surface-strong"
          }`}
        >
          {item.body}
        </div>
        {time !== "" && <span className="text-muted px-1 text-xs tabular-nums">{time}</span>}
      </div>
    );
  }

  return (
    <div className="mx-auto flex h-[100dvh] w-full max-w-3xl flex-col">
      <header className="border-line border-b px-5 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <h1 className="truncate text-base font-bold tracking-tight">{topicName ?? "会話"}</h1>
            <p className="text-muted mt-0.5 text-xs" role="status" aria-live="polite">
              {statusLabels[room.status]}
            </p>
          </div>
          <div className="flex shrink-0 gap-2">
            <button
              type="button"
              onClick={() => setReporting(true)}
              className={secondaryButtonClass}
            >
              通報
            </button>
            <button
              type="button"
              onClick={() => leave("topics")}
              disabled={leaving}
              className={secondaryButtonClass}
            >
              退出
            </button>
          </div>
        </div>

        {others.length > 0 && (
          <div className="mt-3 flex flex-wrap items-center gap-2">
            {others.map((participant) => {
              const isBlocked = blocked.includes(participant);
              const connected = room.present.includes(participant);
              return (
                <div
                  key={participant}
                  className="border-line flex items-center gap-2 rounded-full border px-3 py-1 text-xs"
                >
                  <span
                    aria-hidden
                    className={`h-1.5 w-1.5 rounded-full ${connected ? "bg-brand" : "bg-muted"}`}
                  />
                  <span className="font-medium">
                    {participantLabel(participant, room.self, roomType)}
                  </span>
                  <span className="text-muted">{connected ? "接続中" : "接続なし"}</span>
                  <button
                    type="button"
                    onClick={() => toggleBlock(participant)}
                    className={chipButtonClass}
                  >
                    {isBlocked ? "ブロック解除" : "ブロック"}
                  </button>
                </div>
              );
            })}
          </div>
        )}
      </header>

      <div className="relative flex-1 overflow-hidden">
        <div
          ref={listRef}
          onScroll={handleScroll}
          role="log"
          aria-live="polite"
          aria-label="会話"
          className="flex h-full flex-col gap-3 overflow-y-auto px-5 py-4"
        >
          {room.timeline.length === 0 && room.status !== "unavailable" && (
            <p className="text-muted py-8 text-center text-sm leading-7">{emptyLead}</p>
          )}
          {/* まだ部屋に流れていない送信は、部屋の順序に入れずに末尾へ置く。 */}
          {room.timeline.filter((item) => item.kind !== "outgoing").map(renderItem)}
          {room.timeline.filter((item) => item.kind === "outgoing").map(renderItem)}
        </div>

        {!following && (
          <button
            type="button"
            onClick={scrollToLatest}
            className="border-line bg-background absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full border px-4 py-2 text-xs font-medium shadow-sm"
          >
            最新まで移動
          </button>
        )}
      </div>

      <div className="border-line border-t px-5 py-3">
        {room.status === "ended" && (
          <div>
            <p className="text-sm leading-7">
              {endedMessages[room.endReason ?? ""] ?? endedFallback}
            </p>
            <div className="mt-3 flex flex-wrap gap-2">
              {room.conversation !== null && (
                <button
                  type="button"
                  onClick={() => leave("queue")}
                  disabled={leaving}
                  className={primaryButtonClass}
                >
                  次の相手を探す
                </button>
              )}
              <button
                type="button"
                onClick={() => leave("topics")}
                disabled={leaving}
                className={secondaryButtonClass}
              >
                トピックを選ぶ
              </button>
            </div>
          </div>
        )}

        {room.status === "unavailable" && (
          <div>
            <p className="text-sm leading-7">{unavailableLead}</p>
            <div className="mt-3 flex flex-wrap gap-2">
              <button type="button" onClick={room.retry} className={primaryButtonClass}>
                もう一度つなぐ
              </button>
              <button
                type="button"
                onClick={() => leave("topics")}
                disabled={leaving}
                className={secondaryButtonClass}
              >
                トピックを選ぶ
              </button>
            </div>
          </div>
        )}

        {room.status !== "ended" && room.status !== "unavailable" && (
          <form ref={formRef} onSubmit={handleSubmit} className="flex items-end gap-2">
            <label htmlFor="composer" className="sr-only">
              メッセージ
            </label>
            {/* 入力欄の高さは、同じ字送りで組んだ写しに決めさせる。行が増えたぶんだけ
                伸び、上限に達したら中でスクロールする。 */}
            <div className="border-line focus-within:border-brand grid max-h-40 flex-1 overflow-y-auto rounded-2xl border">
              <span
                aria-hidden
                className={`col-start-1 row-start-1 px-4 py-2.5 text-sm leading-7 break-words whitespace-pre-wrap ${
                  draft === "" ? "text-transparent" : "invisible"
                }`}
              >
                {draft === "" ? composerPlaceholder : `${draft} `}
              </span>
              <textarea
                id="composer"
                rows={1}
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={handleKeyDown}
                placeholder={composerPlaceholder}
                className="col-start-1 row-start-1 resize-none overflow-hidden bg-transparent px-4 py-2.5 text-sm leading-7 outline-none"
              />
            </div>
            <button type="submit" disabled={!canSend} className={primaryButtonClass}>
              送信
            </button>
          </form>
        )}

        {room.status !== "ended" && room.status !== "unavailable" && length > 0 && (
          <p className={`mt-2 text-right text-xs ${tooLong ? "text-danger" : "text-muted"}`}>
            {`${length} / ${messageMaxLength}`}
          </p>
        )}
      </div>

      <ReportDialog
        conversationId={conversationId}
        open={reporting}
        onClose={() => setReporting(false)}
      />
    </div>
  );
}
