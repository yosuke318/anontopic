"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  type FormEvent,
  type KeyboardEvent,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from "react";

import { ReportDialog } from "@/components/report-dialog";
import { joinQueue, leaveQueue, type RoomType } from "@/lib/matching";
import { type BlockReason, messageMaxLength, participantIndex, participantLabel } from "@/lib/room";
import type { Topic } from "@/lib/topics";
import {
  useRoomSocket,
  type NoticeKind,
  type OutgoingFailure,
  type RoomStatus,
  type TimelineItem,
} from "@/lib/use-room-socket";

const legalLinks = [
  { href: "/terms", label: "利用規約" },
  { href: "/privacy", label: "プライバシーポリシー" },
  { href: "/contact", label: "お問い合わせ" },
];

const statusLabels: Record<RoomStatus, string> = {
  connecting: "接続しています",
  open: "接続中",
  reconnecting: "つなぎ直しています",
  ended: "終了しました",
  unavailable: "接続できません",
};

// statusTonesは接続の具合を色でも示す。つながっているかどうかは文言でも分かるため、
// 色は読み取りの助けにとどめる。
const statusTones: Record<RoomStatus, { dot: string; text: string }> = {
  connecting: { dot: "bg-chat-offline", text: "text-chat-ink-muted" },
  open: { dot: "bg-chat-online", text: "text-chat-online-ink" },
  reconnecting: { dot: "bg-chat-alert-line", text: "text-chat-alert" },
  ended: { dot: "bg-chat-offline", text: "text-chat-ink-muted" },
  unavailable: { dot: "bg-chat-alert-line", text: "text-chat-alert" },
};

const failureMessages: Record<OutgoingFailure, string> = {
  blocked:
    "この内容は利用規約で禁止している表現にあたるため、送信できません。相手には届いていません。",
  rate_limited: "続けて送りすぎました。少し待ってから送り直してください。",
  too_long: `本文が長すぎます。${messageMaxLength} 文字までにしてください。`,
  empty_body: "本文が空のため送信できませんでした。",
  invalid_frame: "送信した内容をサーバーが読み取れませんでした。",
  unavailable: "サーバーの都合で送信できませんでした。少し待ってから送り直してください。",
  timeout: "送信を確認できませんでした。相手に届いていない可能性があります。",
};

// blockedMessagesは止めた理由ごとの説明。どの語に当たったかは出さない。
const blockedMessages: Record<BlockReason, string> = {
  contact:
    "URL・電話番号・メールアドレス・SNS の ID・住所などの連絡先は、利用規約で送受信を禁止しているため送信できません。相手には届いていません。",
  meetup:
    "待ち合わせや直接会うことを持ちかける内容は、利用規約で禁止しているため送信できません。相手には届いていません。",
  dating:
    "交際や出会いを目的とした内容は、利用規約で禁止しているため送信できません。相手には届いていません。",
  sexual: "性的な内容は、利用規約で禁止しているため送信できません。相手には届いていません。",
  solicitation:
    "勧誘や宣伝にあたる内容は、利用規約で禁止しているため送信できません。相手には届いていません。",
};

const prohibitedLinkLabel = "禁止していることを見る";

const noticeMessages: Record<NoticeKind, string> = {
  reconnected: "接続が戻りました。切れていた間に流れたメッセージは表示されません。",
  warning:
    "利用規約に反する送信や通報が続いたため、警告します。このまま続くと、一定期間または無期限でご利用いただけなくなります。",
  blocked: "送信した内容は配信されませんでした。",
  rate_limited: "送信が続いたため、いくつかのメッセージが配信されませんでした。",
  too_long: "本文が長すぎるメッセージは配信されませんでした。",
  empty_body: "本文が空のメッセージは配信されませんでした。",
  invalid_frame: "サーバーが読み取れない送信がありました。",
  unavailable: "この部屋をいま扱えませんでした。時間をおいて開き直してください。",
};

const endedTitle = "会話はおしまいです";

const endedMessages: Record<string, string> = {
  user_left: "参加者がいなくなったため、会話は終了しました。",
  reported: "参加者から通報があったため、会話は終了しました。",
  banned: "利用規約に反する送信が続いたため、この端末からのご利用を停止しました。",
};

const endedFallback = "会話は終了しました。";

const unavailableTitle = "会話につなげませんでした";

const unavailableBody = "すでに終わっている会話か、通信が届いていない可能性があります。";

const emptyLead = "まだ発言がありません。最初のひとことを送ってみてください。";

const blockedBody = "ブロックした参加者の発言です。";

const composerPlaceholder = "メッセージを入力";

// 画面の下に置く大きなボタン。押せる高さを54pxとり、影で面を持ち上げる。
const sheetPrimaryClass =
  "bg-chat-self shadow-[0_4px_0_var(--chat-self-edge)] inline-flex min-h-[54px] items-center justify-center rounded-full px-6 text-base font-black disabled:cursor-not-allowed disabled:opacity-50 disabled:shadow-none";

const sheetSecondaryClass =
  "bg-chat-panel border-chat-line inline-flex min-h-[54px] items-center justify-center rounded-full border-[1.5px] px-6 text-base font-bold disabled:cursor-not-allowed disabled:opacity-50";

// ヘッダーに並べる小さなボタン。指で押せる44pxを下回らないようにする。
const headerButtonClass =
  "bg-chat-panel border-chat-line inline-flex min-h-[44px] items-center justify-center rounded-full border-[1.5px] px-4 text-sm font-bold disabled:cursor-not-allowed disabled:opacity-50";

const chipButtonClass =
  "bg-chat-panel text-chat-ink-muted inline-flex min-h-8 items-center justify-center rounded-full px-2.5 text-xs font-bold";

// 吹き出しに添える小さなボタン。説明の中に収めるため、白く塗って縁を持たせない。
const inlineButtonClass =
  "bg-chat-panel text-chat-ink inline-flex min-h-8 items-center justify-center rounded-full px-3 text-xs font-bold no-underline";

// peerTonesは自分以外の参加者に振る色。participantIndexと同じ並びなので、呼び名が
// 参加者Aなら1つめ、参加者Bなら2つめになる。ルームは最大3人で、自分以外は2人まで。
const peerTones = [
  {
    avatar: "bg-chat-peer-1",
    bubble: "bg-chat-peer-1-soft",
    chip: "bg-chat-peer-1-tint",
    dotEdge: "border-chat-peer-1-tint",
  },
  {
    avatar: "bg-chat-peer-2",
    bubble: "bg-chat-peer-2-soft",
    chip: "bg-chat-peer-2-tint",
    dotEdge: "border-chat-peer-2-tint",
  },
] as const;

type PeerTone = (typeof peerTones)[number];

function peerTone(index: number): PeerTone {
  return index <= 0 ? peerTones[0] : peerTones[1];
}

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

function AlertIcon() {
  return (
    <svg
      aria-hidden="true"
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      className="mt-0.5 flex-none"
    >
      <circle cx="8" cy="8" r="6.5" />
      <path d="M8 4.8v3.6M8 11.1v.1" />
    </svg>
  );
}

function SendIcon() {
  return (
    <svg
      aria-hidden="true"
      width="20"
      height="20"
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.4"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M10 15.5V4.5M5.5 9l4.5-4.5L14.5 9" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg
      aria-hidden="true"
      width="24"
      height="24"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M19 14.5A7.5 7.5 0 0 1 9.5 5a7.5 7.5 0 1 0 9.5 9.5z" />
    </svg>
  );
}

function Line({ children }: { children: string }) {
  return (
    <p className="bg-chat-panel text-chat-ink-muted self-center rounded-full px-3.5 py-0.5 text-xs">
      {children}
    </p>
  );
}

// Noteは注意を引く文を、吹き出しに添えるカードとして出す。
function Note({ role, children }: { role?: "alert"; children: ReactNode }) {
  return (
    <div
      role={role}
      className="bg-chat-alert-soft flex max-w-[86%] flex-col gap-2 rounded-[18px] px-3.5 py-2.5"
    >
      {children}
    </div>
  );
}

function NoteText({ children }: { children: string }) {
  return (
    <p className="text-chat-alert flex gap-1.5 text-xs leading-[1.7]">
      <AlertIcon />
      {children}
    </p>
  );
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

  const closed = room.status === "ended" || room.status === "unavailable";

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

  // 通報した人には、ほかの参加者の発言をその場で見えなくする。通報を受けた会話は
  // サーバー側で終了する。理由はdocs/adr/0021-end-and-keep-a-conversation-once-it-is-reported.mdにある。
  function blockEveryoneElse() {
    setBlocked(others);
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
      if (item.notice === "warning") {
        return (
          <div key={item.id} className="flex flex-col items-center">
            <Note role="alert">
              <NoteText>{noticeMessages.warning}</NoteText>
            </Note>
          </div>
        );
      }
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
      const failed = item.failure !== null;
      return (
        <div key={item.id} className="flex flex-col items-end gap-1.5">
          <p
            className={`bg-chat-panel text-chat-ink-muted max-w-[78%] rounded-[20px_6px_20px_20px] border-[1.5px] border-dashed px-3.5 py-2.5 text-[15px] leading-[1.7] whitespace-pre-wrap ${
              failed ? "border-chat-alert-line" : "border-chat-line"
            }`}
          >
            {item.body}
          </p>
          {item.failure === null ? (
            <span className="text-chat-ink-muted text-[11px]">送信中…</span>
          ) : (
            <Note>
              <NoteText>
                {item.failure === "blocked" && item.blockReason !== null
                  ? blockedMessages[item.blockReason]
                  : failureMessages[item.failure]}
              </NoteText>
              <div className="flex flex-wrap justify-end gap-1.5">
                {item.failure === "blocked" && (
                  <Link
                    href="/about#prohibited"
                    target="_blank"
                    rel="noopener noreferrer"
                    className={inlineButtonClass}
                  >
                    {prohibitedLinkLabel}
                  </Link>
                )}
                {item.failure !== "blocked" && (
                  <button
                    type="button"
                    onClick={() => room.resend(item.id, item.body)}
                    className={inlineButtonClass}
                  >
                    送り直す
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => room.discard(item.id)}
                  className={inlineButtonClass}
                >
                  消す
                </button>
              </div>
            </Note>
          )}
        </div>
      );
    }

    if (blocked.includes(item.participant)) {
      return <Line key={item.id}>{blockedBody}</Line>;
    }

    const mine = item.participant === room.self;
    const time = formatTime(item.sentAt);

    if (mine) {
      return (
        <div key={item.id} className="flex flex-col items-end gap-0.5">
          <p className="bg-chat-self max-w-[78%] rounded-[20px_6px_20px_20px] px-3.5 py-2.5 text-[15px] leading-[1.7] whitespace-pre-wrap">
            {item.body}
          </p>
          {time !== "" && (
            <span className="text-chat-ink-muted text-[11px] tabular-nums">{time}</span>
          )}
        </div>
      );
    }

    const tone = peerTone(participantIndex(item.participant, room.self, roomType));

    return (
      <div key={item.id} className="flex items-start gap-2">
        <span aria-hidden className={`h-[30px] w-[30px] flex-none rounded-full ${tone.avatar}`} />
        <div className="flex max-w-[78%] flex-col gap-0.5">
          <span className="text-chat-ink-muted text-[11px] font-bold">
            {participantLabel(item.participant, room.self, roomType)}
          </span>
          <p
            className={`rounded-[6px_20px_20px_20px] px-3.5 py-2.5 text-[15px] leading-[1.7] whitespace-pre-wrap ${tone.bubble}`}
          >
            {item.body}
          </p>
          {time !== "" && (
            <span className="text-chat-ink-muted text-[11px] tabular-nums">{time}</span>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="bg-chat-bg text-chat-ink font-rounded flex h-[100dvh] w-full justify-center leading-[1.8] font-medium">
      <div className="flex w-full max-w-3xl flex-col">
        <header className="bg-chat-panel flex flex-col gap-2.5 rounded-b-[28px] px-4 py-2.5 shadow-[0_8px_24px_-18px_rgba(150,70,100,0.4)]">
          <div className="flex items-center justify-between gap-2.5">
            <div className="flex min-w-0 flex-col">
              <h1 className="truncate text-lg leading-tight font-black">{topicName ?? "会話"}</h1>
              <p
                role="status"
                aria-live="polite"
                className={`flex items-center gap-1.5 text-xs font-bold ${statusTones[room.status].text}`}
              >
                <span
                  aria-hidden
                  className={`h-2 w-2 flex-none rounded-full ${statusTones[room.status].dot}`}
                />
                {statusLabels[room.status]}
              </p>
            </div>
            <div className="flex flex-none gap-1.5">
              <button
                type="button"
                onClick={() => setReporting(true)}
                className={headerButtonClass}
              >
                通報
              </button>
              {!closed && (
                <button
                  type="button"
                  onClick={() => leave("topics")}
                  disabled={leaving}
                  className={`${headerButtonClass} bg-chat-alert-soft border-chat-alert-soft`}
                >
                  退出
                </button>
              )}
            </div>
          </div>

          {others.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {others.map((participant) => {
                const isBlocked = blocked.includes(participant);
                const connected = room.present.includes(participant);
                const tone = peerTone(participantIndex(participant, room.self, roomType));
                return (
                  <div
                    key={participant}
                    className={`flex items-center gap-1.5 rounded-full p-[3px] ${tone.chip}`}
                  >
                    <span
                      aria-hidden
                      className={`relative h-[26px] w-[26px] flex-none rounded-full ${tone.avatar}`}
                    >
                      <span
                        className={`absolute -right-px -bottom-px h-[9px] w-[9px] rounded-full border-2 ${tone.dotEdge} ${
                          connected ? "bg-chat-online" : "bg-chat-offline"
                        }`}
                      />
                    </span>
                    <span className="text-[13px] font-bold">
                      {participantLabel(participant, room.self, roomType)}
                    </span>
                    <span className="sr-only">{connected ? "接続中" : "接続なし"}</span>
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

          {/* 会話を離れずに読めるよう、別のタブで開く。 */}
          <nav
            aria-label="規約と窓口"
            className="text-chat-ink-muted flex flex-wrap gap-x-3.5 text-[11px]"
          >
            {legalLinks.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                target="_blank"
                rel="noopener noreferrer"
                className="hover:text-chat-alert underline-offset-2 hover:underline"
              >
                {link.label}
              </Link>
            ))}
          </nav>
        </header>

        <div className="relative flex-1 overflow-hidden">
          <div
            ref={listRef}
            onScroll={handleScroll}
            role="log"
            aria-live="polite"
            aria-label="会話"
            className="flex h-full flex-col overflow-y-auto px-4 py-4"
          >
            {/* 発言が少ないうちは下端に寄せる。溢れたぶんはmt-autoが0になるので、
                上へスクロールして読める。 */}
            <div className="mt-auto flex flex-col gap-3">
              {room.timeline.length === 0 && room.status !== "unavailable" && (
                <p className="text-chat-ink-muted py-8 text-center text-sm leading-7">
                  {emptyLead}
                </p>
              )}
              {/* まだ部屋に流れていない送信は、部屋の順序に入れずに末尾へ置く。 */}
              {room.timeline.filter((item) => item.kind !== "outgoing").map(renderItem)}
              {room.timeline.filter((item) => item.kind === "outgoing").map(renderItem)}
            </div>
          </div>

          {!following && (
            <button
              type="button"
              onClick={scrollToLatest}
              className="bg-chat-panel border-chat-line absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full border-[1.5px] px-4 py-2 text-xs font-bold shadow-[0_6px_16px_-12px_rgba(150,70,100,0.6)]"
            >
              最新まで移動
            </button>
          )}
        </div>

        {closed ? (
          <div className="bg-chat-panel flex flex-col gap-4 rounded-t-[32px] px-5 pt-6 pb-7 shadow-[0_-10px_30px_-20px_rgba(150,70,100,0.45)]">
            <div className="flex items-center gap-3.5">
              <span
                aria-hidden
                className={`flex h-12 w-12 flex-none items-center justify-center rounded-full ${
                  room.status === "ended" ? "bg-chat-moon" : "bg-chat-alert-soft text-chat-alert"
                }`}
              >
                {room.status === "ended" ? <MoonIcon /> : <AlertIcon />}
              </span>
              <div className="flex flex-col">
                <p className="text-[17px] font-black">
                  {room.status === "ended" ? endedTitle : unavailableTitle}
                </p>
                <p className="text-chat-ink-muted text-[13px] leading-[1.7]">
                  {room.status === "ended"
                    ? (endedMessages[room.endReason ?? ""] ?? endedFallback)
                    : unavailableBody}
                </p>
              </div>
            </div>
            <div className="flex flex-col gap-2.5">
              {room.status === "ended" && room.conversation !== null && (
                <button
                  type="button"
                  onClick={() => leave("queue")}
                  disabled={leaving}
                  className={sheetPrimaryClass}
                >
                  次の相手を探す
                </button>
              )}
              {room.status === "unavailable" && (
                <button type="button" onClick={room.retry} className={sheetPrimaryClass}>
                  もう一度つなぐ
                </button>
              )}
              <button
                type="button"
                onClick={() => leave("topics")}
                disabled={leaving}
                className={sheetSecondaryClass}
              >
                トピックを選ぶ
              </button>
            </div>
          </div>
        ) : (
          <div className="px-3 pt-2.5 pb-5">
            <form ref={formRef} onSubmit={handleSubmit} className="flex items-end gap-2">
              <label htmlFor="composer" className="sr-only">
                メッセージ
              </label>
              {/* 入力欄の高さは、同じ字送りで組んだ写しに決めさせる。行が増えたぶんだけ
                  伸び、上限に達したら中でスクロールする。 */}
              <div className="border-chat-line bg-chat-panel focus-within:border-chat-self grid max-h-40 min-h-12 flex-1 overflow-y-auto rounded-3xl border-[1.5px]">
                <span
                  aria-hidden
                  className={`col-start-1 row-start-1 px-[18px] py-[11px] text-[15px] leading-[1.6] break-words whitespace-pre-wrap ${
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
                  className="placeholder:text-chat-ink-muted col-start-1 row-start-1 resize-none overflow-hidden bg-transparent px-[18px] py-[11px] text-[15px] leading-[1.6] outline-none"
                />
              </div>
              <button
                type="submit"
                aria-label="送信"
                disabled={!canSend}
                className="bg-chat-self shadow-[0_3px_0_var(--chat-self-edge)] flex h-12 w-12 flex-none items-center justify-center rounded-full disabled:cursor-not-allowed disabled:opacity-50 disabled:shadow-none"
              >
                <SendIcon />
              </button>
            </form>

            {length > 0 && (
              <p
                className={`mt-2 text-right text-[11px] ${tooLong ? "text-chat-alert" : "text-chat-ink-muted"}`}
              >
                {`${length} / ${messageMaxLength}`}
              </p>
            )}
          </div>
        )}
      </div>

      <ReportDialog
        conversationId={conversationId}
        open={reporting}
        onClose={() => setReporting(false)}
        onReported={blockEveryoneElse}
      />
    </div>
  );
}
