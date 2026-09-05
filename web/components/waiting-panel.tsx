"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { ApiError, leaveQueue, readMatchingState, type MatchingState } from "@/lib/matching";
import type { Topic } from "@/lib/topics";

const pollIntervalMs = 2000;

// 続けてこの回数だけ状態を読めなかったら、待ち続けずに読み込み直してもらう。
const maxConsecutiveFailures = 5;

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
  error: {
    title: "状態を確認できませんでした",
    lead: "サーバーが応答しませんでした。通信状況を確認してから読み込み直してください。",
  },
} as const;

const fallbackHint = "3 人が揃わないときは、しばらく待ってから 2 人で始まります。";

const unstableHint = "サーバーの応答が返っていません。接続を試し続けています。";

type Status =
  | { kind: "checking" }
  | { kind: "waiting"; state: MatchingState }
  | { kind: "matched"; state: MatchingState }
  | { kind: "gone" }
  | { kind: "error" };

function topicLabel(topicName: string | undefined): string {
  return topicName === undefined ? "選んだトピック" : `「${topicName}」`;
}

function formatElapsed(milliseconds: number): string {
  const total = Math.max(0, Math.floor(milliseconds / 1000));
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return minutes === 0 ? `${seconds} 秒` : `${minutes} 分 ${seconds} 秒`;
}

function Screen({ kind, children }: { kind: Status["kind"]; children?: React.ReactNode }) {
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
  "bg-brand text-brand-contrast hover:bg-brand-hover inline-flex h-12 items-center justify-center rounded-full px-8 font-semibold transition-colors";

const secondaryButtonClass =
  "border-line hover:bg-surface inline-flex h-12 items-center justify-center rounded-full border px-8 font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50";

export function WaitingPanel({ topics }: { topics: Topic[] }) {
  const router = useRouter();
  const [status, setStatus] = useState<Status>({ kind: "checking" });
  const [unstable, setUnstable] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const failures = useRef(0);

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
          setStatus({ kind: "gone" });
          return;
        }
        if (state.state === "matched") {
          setStatus({ kind: "matched", state });
          return;
        }
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

  if (status.kind === "checking") {
    return <Screen kind="checking" />;
  }

  if (status.kind === "gone") {
    return (
      <Screen kind="gone">
        <Link href="/topics" className={primaryButtonClass}>
          トピックを選ぶ
        </Link>
      </Screen>
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
        {conversation !== undefined && (
          <Link href={`/rooms/${conversation.id}`} className={`${primaryButtonClass} mt-8`}>
            会話を開く
          </Link>
        )}
      </Screen>
    );
  }

  const waitingSince = status.state.waiting_since;
  const elapsed = waitingSince === undefined ? null : now - new Date(waitingSince).getTime();

  return (
    <Screen kind="waiting">
      <div role="status" aria-live="polite">
        <p className="leading-7">
          {`${topicLabel(topicName)}を選んだ人が ${status.state.room_type} 人集まるまで待ちます。`}
        </p>
        {elapsed !== null && (
          <p className="mt-6 text-3xl font-bold tabular-nums">{formatElapsed(elapsed)}</p>
        )}
        {status.state.room_type === 3 && (
          <p className="text-muted mt-6 leading-7">{fallbackHint}</p>
        )}
        {unstable && <p className="text-muted mt-4 text-sm leading-6">{unstableHint}</p>}
      </div>
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
