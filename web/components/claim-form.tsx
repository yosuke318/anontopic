"use client";

import { type FormEvent, useState } from "react";

import { ApiError } from "@/lib/api";
import {
  claimDetailsMaxLength,
  claimEmailMaxLength,
  claimNameMaxLength,
  claimRights,
  type ClaimRight,
  submitClaim,
} from "@/lib/claims";

const done =
  "申し立てを受け付けました。内容を確認し、ご入力いただいたメールアドレスに対応の結果をお知らせします。";

const detailsPlaceholder =
  "例: 〇月〇日ごろの会話で、私の本名と勤務先が書き込まれていたと知人から聞きました。削除を求めます。";

function messageForError(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.status) {
      case 400:
        return "入力内容を読み取れませんでした。メールアドレスの形式と、各項目の文字数を確認してください。";
      case 429: {
        const minutes =
          error.retryAfterSeconds === null ? null : Math.ceil(error.retryAfterSeconds / 60);
        return minutes === null
          ? "短い時間に続けて送信されました。しばらく待ってからお試しください。"
          : `短い時間に続けて送信されました。${minutes} 分ほど待ってからお試しください。`;
      }
      default:
        return "サーバーが応答しませんでした。しばらく待ってからお試しください。";
    }
  }
  return "サーバーに接続できませんでした。通信状況を確認してください。";
}

const fieldClass =
  "border-line bg-background focus:outline-brand w-full rounded-xl border px-4 py-3 text-sm focus:outline-2 focus:outline-offset-2";

const primaryButtonClass =
  "bg-brand text-brand-contrast hover:bg-brand-hover inline-flex h-12 items-center justify-center rounded-full px-8 text-base font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50";

export function ClaimForm() {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [right, setRight] = useState<ClaimRight>("privacy");
  const [details, setDetails] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await submitClaim({ name, email, right, details });
      setSubmitted(true);
    } catch (err) {
      setError(messageForError(err));
    }
    setSubmitting(false);
  }

  if (submitted) {
    return (
      <p role="status" className="border-line bg-surface mt-10 rounded-2xl border p-6 leading-7">
        {done}
      </p>
    );
  }

  const detailsLength = [...details.trim()].length;

  return (
    <form onSubmit={handleSubmit} className="mt-10 flex flex-col gap-8">
      <div>
        <label htmlFor="claim-name" className="text-sm font-bold">
          お名前（名称）
        </label>
        <input
          id="claim-name"
          type="text"
          required
          maxLength={claimNameMaxLength}
          autoComplete="name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          className={`${fieldClass} mt-2`}
        />
      </div>

      <div>
        <label htmlFor="claim-email" className="text-sm font-bold">
          連絡先メールアドレス
        </label>
        <input
          id="claim-email"
          type="email"
          required
          maxLength={claimEmailMaxLength}
          autoComplete="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          className={`${fieldClass} mt-2`}
        />
      </div>

      <fieldset>
        <legend className="text-sm font-bold">侵害されたと考える権利</legend>
        <div className="mt-3 flex flex-col gap-2">
          {claimRights.map((option) => (
            <label
              key={option.value}
              className={`outline-brand cursor-pointer rounded-xl border px-4 py-3 text-sm transition-colors focus-within:outline-2 focus-within:outline-offset-2 ${
                right === option.value
                  ? "border-brand bg-brand-soft"
                  : "border-line hover:bg-surface"
              }`}
            >
              <input
                type="radio"
                name="right"
                value={option.value}
                checked={right === option.value}
                onChange={() => setRight(option.value)}
                className="sr-only"
              />
              {option.label}
            </label>
          ))}
        </div>
      </fieldset>

      <div>
        <label htmlFor="claim-details" className="text-sm font-bold">
          申し立ての内容
        </label>
        <textarea
          id="claim-details"
          required
          rows={8}
          maxLength={claimDetailsMaxLength}
          placeholder={detailsPlaceholder}
          value={details}
          onChange={(event) => setDetails(event.target.value)}
          className={`${fieldClass} mt-2 leading-7`}
        />
        <p className="text-muted mt-2 text-right text-xs">
          {`${detailsLength} / ${claimDetailsMaxLength}`}
        </p>
      </div>

      {error !== null && (
        <p role="alert" className="text-danger text-sm leading-6">
          {error}
        </p>
      )}

      <div>
        <button type="submit" disabled={submitting} className={primaryButtonClass}>
          {submitting ? "送信しています…" : "申し立てを送信する"}
        </button>
      </div>
    </form>
  );
}
