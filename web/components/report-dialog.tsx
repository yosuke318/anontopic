"use client";

import { type FormEvent, useEffect, useRef, useState } from "react";

import { ApiError } from "@/lib/api";
import { reportReasons, submitReport, type ReportReason } from "@/lib/reports";

const lead =
  "この会話を運営に報告します。相手を選ぶ必要はありません。報告された会話は運営だけが確認します。";

const done =
  "通報を受け付けました。内容は運営が確認します。結果を個別にお知らせすることはありません。";

function messageForError(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.status) {
      case 401:
        return "セッションが切れています。トピックを選び直してください。";
      case 403:
        return "この会話は通報できません。";
      default:
        return "サーバーが応答しませんでした。しばらく待ってからお試しください。";
    }
  }
  return "サーバーに接続できませんでした。通信状況を確認してください。";
}

const primaryButtonClass =
  "bg-brand text-brand-contrast hover:bg-brand-hover inline-flex h-11 items-center justify-center rounded-full px-6 text-sm font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50";

const secondaryButtonClass =
  "border-line hover:bg-surface inline-flex h-11 items-center justify-center rounded-full border px-6 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50";

export function ReportDialog({
  conversationId,
  open,
  onClose,
}: {
  conversationId: string;
  open: boolean;
  onClose: () => void;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const [reason, setReason] = useState<ReportReason>("other");
  const [submitting, setSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (dialog === null) {
      return;
    }
    if (open && !dialog.open) {
      setReason("other");
      setSubmitting(false);
      setSubmitted(false);
      setError(null);
      dialog.showModal();
    }
    if (!open && dialog.open) {
      dialog.close();
    }
  }, [open]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await submitReport(conversationId, reason);
      setSubmitted(true);
    } catch (err) {
      setError(messageForError(err));
    }
    setSubmitting(false);
  }

  return (
    <dialog
      ref={dialogRef}
      onClose={onClose}
      aria-labelledby="report-dialog-title"
      className="border-line bg-background text-foreground m-auto w-[min(28rem,calc(100vw-2rem))] rounded-2xl border p-6 backdrop:bg-black/50"
    >
      <h2 id="report-dialog-title" className="text-lg font-bold tracking-tight">
        この会話を通報する
      </h2>

      {submitted ? (
        <>
          <p className="text-muted mt-4 text-sm leading-7">{done}</p>
          <div className="mt-6 flex justify-end">
            <button type="button" onClick={onClose} className={primaryButtonClass}>
              閉じる
            </button>
          </div>
        </>
      ) : (
        <form onSubmit={handleSubmit}>
          <p className="text-muted mt-4 text-sm leading-7">{lead}</p>

          <fieldset className="mt-6">
            <legend className="text-sm font-bold">どれにあてはまりますか</legend>
            <div className="mt-3 flex flex-col gap-2">
              {reportReasons.map((option) => (
                <label
                  key={option.value}
                  className={`outline-brand cursor-pointer rounded-xl border px-4 py-3 text-sm transition-colors focus-within:outline-2 focus-within:outline-offset-2 ${
                    reason === option.value
                      ? "border-brand bg-brand-soft"
                      : "border-line hover:bg-surface"
                  }`}
                >
                  <input
                    type="radio"
                    name="reason"
                    value={option.value}
                    checked={reason === option.value}
                    onChange={() => setReason(option.value)}
                    className="sr-only"
                  />
                  {option.label}
                </label>
              ))}
            </div>
          </fieldset>

          {error !== null && (
            <p role="alert" className="text-danger mt-4 text-sm leading-6">
              {error}
            </p>
          )}

          <div className="mt-6 flex justify-end gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className={secondaryButtonClass}
            >
              やめる
            </button>
            <button type="submit" disabled={submitting} className={primaryButtonClass}>
              {submitting ? "送信しています…" : "通報する"}
            </button>
          </div>
        </form>
      )}
    </dialog>
  );
}
