"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type ReactNode, useEffect, useRef, useState } from "react";

import {
  WaitingGame,
  bestDistance,
  emptyWaitingGameStats,
  type WaitingGameStats,
} from "@/components/waiting-game";
import { ApiError } from "@/lib/api";
import { leaveQueue, readMatchingState, type MatchingState } from "@/lib/matching";
import type { Topic } from "@/lib/topics";
import { formatDistance, saveWaitingDistance } from "@/lib/waiting-distance";

const pollIntervalMs = 2000;

// 続けてこの回数だけ状態を読めなかったら、待ち続けずに読み込み直してもらう。
const maxConsecutiveFailures = 5;

// 待機が見つからなくなったとき、打ち切りの時刻からこの時間の内なら時間切れとみなす。
// 画面の時計とサーバーの時計のずれと、状態を読む間隔のぶんを見込む。
const expiryMarginMs = 30_000;

// 待機の打ち切りまでの時間が読めないときに案内する分数。サーバーの既定の待機時間と同じ。
const defaultWaitMinutes = 10;

const copy = {
  checking: {
    title: "待機中",
    lead: "待機の状態を確認しています。",
  },
  waiting: {
    title: "相手を待っています",
    lead: "同じトピックを選んだ人が集まると会話が始まります。待っている間はいつでもやめられます。",
  },
  matched: {
    title: "相手が見つかりました",
    lead: "会話の準備ができました。開くと相手にも参加が伝わります。",
  },
  gone: {
    title: "待機は終わっています",
    lead: "待機できる時間を過ぎたか、別の画面で待機をやめています。もう一度トピックを選んでください。",
  },
  expired: {
    title: "待機時間が終わりました",
    lead: "時間内に相手が見つかりませんでした。時間をおいて、もう一度トピックを選んでください。",
  },
  error: {
    title: "状態を確認できませんでした",
    lead: "サーバーが応答しませんでした。通信状況を確認してから読み込み直してください。",
  },
} as const;

const fallbackHint = "3 人が揃わないときは、しばらく待ってから 2 人で始まります。";

const gameNote =
  "相手が見つかるとゲームは終わり、待っている間にいちばん進んだ距離が会話の相手にも伝わります。";

const unstableHint = "サーバーの応答が返っていません。接続を試し続けています。";

type Status =
  | { kind: "checking" }
  | { kind: "waiting"; state: MatchingState }
  // distanceは待っている間にいちばん進んだ距離。ゲームを遊ばなかったときはnull。
  | { kind: "matched"; state: MatchingState; distance: number | null }
  | { kind: "gone" }
  | { kind: "expired" }
  | { kind: "error" };

function topicLabel(topicName: string | undefined): string {
  return topicName === undefined ? "選んだトピック" : `「${topicName}」`;
}

function formatDuration(milliseconds: number): string {
  const total = Math.max(0, Math.floor(milliseconds / 1000));
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return minutes === 0 ? `${seconds} 秒` : `${minutes} 分 ${seconds} 秒`;
}

// timeOfは時刻の文字列をミリ秒にする。読めない値にはnullを返す。
function timeOf(value: string | undefined): number | null {
  if (value === undefined) {
    return null;
  }
  const time = new Date(value).getTime();
  return Number.isNaN(time) ? null : time;
}

// waitMinutesOfは待機できる時間を分で返す。サーバーが返す待機の始まりと打ち切りの差から
// 求めるので、待機時間の設定を変えても案内がずれない。
function waitMinutesOf(state: MatchingState): number {
  const since = timeOf(state.waiting_since);
  const until = timeOf(state.waiting_until);
  if (since === null || until === null || until <= since) {
    return defaultWaitMinutes;
  }
  return Math.round((until - since) / 60_000);
}

const perfectTitle = "誰にもマッチしませんでしたが、perfect!!!";

// PerfectDialogは、待機の間ずっと一度も障害物に当たらなかった人に出す。
function PerfectDialog({ distance, onClose }: { distance: number; onClose: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    closeRef.current?.focus();
  }, []);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="perfect-title"
        className="bg-background shadow-brand w-full max-w-sm rounded-3xl p-8 text-center"
      >
        <p aria-hidden className="text-4xl">
          🎉
        </p>
        <h2 id="perfect-title" className="mt-4 text-xl font-bold leading-8">
          {perfectTitle}
        </h2>
        <p className="text-muted mt-4 leading-7">
          {`待っている間、一度も障害物に当たらずに ${formatDistance(distance)} 進みました。`}
        </p>
        <button
          ref={closeRef}
          type="button"
          onClick={onClose}
          className="bg-brand text-brand-contrast hover:bg-brand-hover shadow-brand mt-8 inline-flex h-12 items-center justify-center rounded-full px-8 font-bold transition-colors"
        >
          閉じる
        </button>
      </div>
    </div>
  );
}

function Screen({ kind, children }: { kind: Status["kind"]; children?: ReactNode }) {
  return (
    <>
      <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">{copy[kind].title}</h1>
      <p className="text-muted mt-6 leading-8">{copy[kind].lead}</p>
      {children !== undefined && (
        <div className="border-line mt-12 rounded-2xl border p-8">{children}</div>
      )}
    </>
  );
}

const primaryButtonClass =
  "bg-brand text-brand-contrast hover:bg-brand-hover shadow-brand inline-flex h-12 items-center justify-center rounded-full px-8 font-bold transition-colors";

const secondaryButtonClass =
  "border-line hover:bg-surface inline-flex h-12 items-center justify-center rounded-full border px-8 font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50";

export function WaitingPanel({ topics }: { topics: Topic[] }) {
  const router = useRouter();
  const [status, setStatus] = useState<Status>({ kind: "checking" });
  const [unstable, setUnstable] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const [perfect, setPerfect] = useState<number | null>(null);
  const failures = useRef(0);
  const game = useRef<WaitingGameStats>(emptyWaitingGameStats());
  // waitingUntilは最後に読めた待機の打ち切りの時刻。待機が見つからなくなったとき、
  // 時間切れかどうかをこれで見分ける。
  const waitingUntil = useRef<number | null>(null);

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;

    async function poll() {
      try {
        const state = await readMatchingState();
        if (!active) {
          return;
        }
        failures.current = 0;
        setUnstable(false);

        if (state === null) {
          const until = waitingUntil.current;
          if (until !== null && Date.now() >= until - expiryMarginMs) {
            const stats = game.current;
            if (stats.started && !stats.crashed) {
              setPerfect(bestDistance(stats));
            }
            setStatus({ kind: "expired" });
          } else {
            setStatus({ kind: "gone" });
          }
          return;
        }
        if (state.state === "matched") {
          // 会話の画面が部屋に入るときに、待っている間に進んだ距離を相手に伝える。
          const distance = game.current.started ? bestDistance(game.current) : null;
          if (state.conversation !== undefined && distance !== null) {
            saveWaitingDistance(state.conversation.id, distance);
          }
          setStatus({ kind: "matched", state, distance });
          return;
        }
        waitingUntil.current = timeOf(state.waiting_until) ?? waitingUntil.current;
        setStatus({ kind: "waiting", state });
      } catch (err) {
        if (!active) {
          return;
        }
        // セッションが失効していれば、待ち続けても状態は返ってこない。
        if (err instanceof ApiError && err.status === 401) {
          setStatus({ kind: "gone" });
          return;
        }
        failures.current += 1;
        if (failures.current >= maxConsecutiveFailures) {
          setStatus({ kind: "error" });
          return;
        }
        setUnstable(true);
      }
      timer = setTimeout(poll, pollIntervalMs);
    }

    void poll();

    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, []);

  useEffect(() => {
    if (status.kind !== "waiting") {
      return;
    }
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [status.kind]);

  async function handleLeave() {
    setLeaving(true);
    try {
      await leaveQueue();
    } catch {
      // 待機の取り消しに失敗しても、キューは待機の期限で外れる。選び直しには進める。
    }
    router.push("/topics");
  }

  const dialog =
    perfect === null ? null : <PerfectDialog distance={perfect} onClose={() => setPerfect(null)} />;

  if (status.kind === "checking") {
    return <Screen kind="checking" />;
  }

  if (status.kind === "gone" || status.kind === "expired") {
    return (
      <>
        <Screen kind={status.kind}>
          <Link href="/topics" className={primaryButtonClass}>
            トピックを選ぶ
          </Link>
        </Screen>
        {dialog}
      </>
    );
  }

  if (status.kind === "error") {
    return (
      <Screen kind="error">
        <a href="/waiting" className={secondaryButtonClass}>
          読み込み直す
        </a>
      </Screen>
    );
  }

  const topicName = topics.find((topic) => topic.id === status.state.topic_id)?.name;

  if (status.kind === "matched") {
    const conversation = status.state.conversation;
    return (
      <Screen kind="matched">
        <p className="leading-7">
          {`${topicLabel(topicName)}について ${status.state.room_type} 人で話します。`}
        </p>
        {status.distance !== null && (
          <p className="text-muted mt-4 leading-7">
            {`待っている間に ${formatDistance(status.distance)} 進みました。会話の相手にも伝わります。`}
          </p>
        )}
        {conversation !== undefined && (
          <Link href={`/rooms/${conversation.id}`} className={`${primaryButtonClass} mt-8`}>
            会話を開く
          </Link>
        )}
      </Screen>
    );
  }

  const until = timeOf(status.state.waiting_until);
  const remaining = until === null ? null : until - now;
  const waitMinutes = waitMinutesOf(status.state);

  return (
    <Screen kind="waiting">
      <div role="status" aria-live="polite">
        <p className="leading-7">
          {`${topicLabel(topicName)}を選んだ人が ${status.state.room_type} 人集まるまで待ちます。`}
        </p>
        <p className="text-muted mt-2 text-sm leading-6">
          {`待機は最長 ${waitMinutes} 分です。${waitMinutes} 分たっても相手が見つからなければ、待機は終わります。`}
        </p>
        {remaining !== null && (
          <p className="mt-6">
            <span className="text-muted text-sm">のこり</span>
            <span className="ml-2 text-3xl font-bold tabular-nums">
              {formatDuration(remaining)}
            </span>
          </p>
        )}
        {status.state.room_type === 3 && (
          <p className="text-muted mt-6 leading-7">{fallbackHint}</p>
        )}
        {unstable && <p className="text-muted mt-4 text-sm leading-6">{unstableHint}</p>}
      </div>
      {(remaining === null || remaining > 0) && (
        <div className="mt-8">
          <WaitingGame stats={game} />
          <p className="text-muted mt-3 text-xs leading-5">{gameNote}</p>
        </div>
      )}
      <button
        type="button"
        onClick={handleLeave}
        disabled={leaving}
        className={`${secondaryButtonClass} mt-8`}
      >
        {leaving ? "待機をやめています…" : "待機をやめる"}
      </button>
    </Screen>
  );
}
