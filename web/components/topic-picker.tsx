"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";

import { ApiError } from "@/lib/api";
import { issueSession, joinQueue, roomTypes, type RoomType } from "@/lib/matching";
import { acceptTerms, useTermsAccepted } from "@/lib/terms-consent";
import type { Topic } from "@/lib/topics";

const siteNotice =
  "このサービスは雑談・趣味・相談のためのものです。出会いや交際を目的とした利用、連絡先の交換はできません。はじめる前に";

const roomTypeHint = "3 人が揃わないときは、しばらく待ってから 2 人で始まります。";

const roomTypeLabels: Record<RoomType, { title: string; body: string }> = {
  2: { title: "2 人で話す", body: "相手はひとり。じっくり話したいとき。" },
  3: { title: "3 人で話す", body: "自分を含めて 3 人。会話が続きやすい。" },
};

function messageForError(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.status) {
      case 400:
        return "選んだトピックが見つかりませんでした。画面を再読み込みしてから選び直してください。";
      case 401:
        return "セッションを用意できませんでした。もう一度お試しください。";
      case 403:
        return "利用規約に反する利用があったため、現在この端末または回線からはご利用いただけません。";
      case 429:
        return error.retryAfterSeconds === null
          ? "短い間に何度も試されています。しばらく待ってからお試しください。"
          : `短い間に何度も試されています。${error.retryAfterSeconds} 秒ほど待ってからお試しください。`;
      default:
        return "サーバーが応答しませんでした。しばらく待ってからお試しください。";
    }
  }
  return "サーバーに接続できませんでした。通信状況を確認してください。";
}

export function TopicPicker({ topics }: { topics: Topic[] }) {
  const router = useRouter();
  const [topicId, setTopicId] = useState<number | null>(null);
  const [roomType, setRoomType] = useState<RoomType>(3);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const termsAccepted = useTermsAccepted();
  const [agreed, setAgreed] = useState(false);
  const canStart = topicId !== null && (termsAccepted || agreed);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (topicId === null || !canStart || submitting) {
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      if (!termsAccepted) {
        acceptTerms();
      }
      await issueSession();
      await joinQueue(topicId, roomType);
      router.push("/waiting");
    } catch (err) {
      // 別のキューで待っている、またはすでにルームを持っている場合は、待機画面が
      // その状態を読み取って続きを引き受ける。
      if (err instanceof ApiError && err.status === 409) {
        router.push("/waiting");
        return;
      }
      setError(messageForError(err));
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit}>
      <fieldset>
        <legend className="text-xl font-bold tracking-tight">話したいトピック</legend>
        <p className="text-muted mt-2 leading-7">ひとつ選んでください。</p>
        <div className="mt-6 flex flex-wrap gap-3">
          {topics.map((topic) => (
            <label
              key={topic.id}
              className={`outline-brand cursor-pointer rounded-full border px-5 py-2.5 text-sm font-medium transition-colors focus-within:outline-2 focus-within:outline-offset-2 ${
                topicId === topic.id
                  ? "border-brand bg-brand text-brand-contrast"
                  : "border-line hover:bg-surface"
              }`}
            >
              <input
                type="radio"
                name="topic"
                value={topic.id}
                checked={topicId === topic.id}
                onChange={() => setTopicId(topic.id)}
                className="sr-only"
              />
              {topic.name}
            </label>
          ))}
        </div>
      </fieldset>

      <fieldset className="mt-12">
        <legend className="text-xl font-bold tracking-tight">話す人数</legend>
        <p className="text-muted mt-2 leading-7">{roomTypeHint}</p>
        <div className="mt-6 grid gap-3 sm:grid-cols-2">
          {roomTypes.map((value) => (
            <label
              key={value}
              className={`outline-brand cursor-pointer rounded-2xl border p-5 transition-colors focus-within:outline-2 focus-within:outline-offset-2 ${
                roomType === value ? "border-brand bg-brand-soft" : "border-line hover:bg-surface"
              }`}
            >
              <input
                type="radio"
                name="roomType"
                value={value}
                checked={roomType === value}
                onChange={() => setRoomType(value)}
                className="sr-only"
              />
              <span className="block font-bold">{roomTypeLabels[value].title}</span>
              <span className="text-muted mt-2 block text-sm leading-6">
                {roomTypeLabels[value].body}
              </span>
            </label>
          ))}
        </div>
      </fieldset>

      {error !== null && (
        <p role="alert" className="text-danger mt-8 leading-7">
          {error}
        </p>
      )}

      {!termsAccepted && (
        <div className="border-line mt-12 rounded-2xl border p-5">
          <p className="text-muted text-sm leading-6">
            {siteNotice}
            <Link href="/terms" target="_blank" rel="noopener noreferrer" className="text-foreground underline">
              利用規約
            </Link>
            と
            <Link href="/privacy" target="_blank" rel="noopener noreferrer" className="text-foreground underline">
              プライバシーポリシー
            </Link>
            をお読みください。
          </p>
          <label className="mt-4 flex cursor-pointer items-start gap-3 leading-7">
            <input
              type="checkbox"
              checked={agreed}
              onChange={(event) => setAgreed(event.target.checked)}
              className="accent-brand mt-1.5 h-4 w-4 shrink-0"
            />
            <span className="font-medium">利用規約とプライバシーポリシーに同意する</span>
          </label>
        </div>
      )}

      <div className="mt-10">
        <button
          type="submit"
          disabled={!canStart || submitting}
          className="bg-brand text-brand-contrast hover:bg-brand-hover inline-flex h-12 items-center justify-center rounded-full px-8 text-base font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50"
        >
          {submitting ? "待機を始めています…" : "話しはじめる"}
        </button>
        {!canStart && (
          <p className="text-muted mt-3 text-sm">
            {termsAccepted
              ? "トピックを選ぶと押せるようになります。"
              : "トピックを選び、利用規約に同意すると押せるようになります。"}
          </p>
        )}
      </div>
    </form>
  );
}
