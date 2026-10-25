"use client";

import { type FormEvent, useEffect, useRef, useState } from "react";

import { ApiError } from "@/lib/api";
import { reportReasons, submitReport, type ReportReason } from "@/lib/reports";

const lead =
  "この会話を運営に報告します。相手を選ぶ必要はありません。通報するとこの会話は終了し、ほかの参加者の発言は見えなくなります。報告された会話は運営だけが確認します。";

const done =
  "通報を受け付けました。この会話は終了しました。内容は運営が確認します。結果を個別にお知らせすることはありません。";

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

// 画面の下に出すシートのボタン。押せる高さを52pxとる。
const submitButtonClass =
  "bg-chat-ink text-chat-panel inline-flex min-h-[52px] items-center justify-center rounded-full px-4 text-[15px] font-black disabled:cursor-not-allowed disabled:opacity-50";

const cancelButtonClass =
  "bg-chat-panel border-chat-line inline-flex min-h-[52px] items-center justify-center rounded-full border-[1.5px] px-4 text-[15px] font-bold disabled:cursor-not-allowed disabled:opacity-50";

export function ReportDialog({
  conversationId,
  open,
  onClose,
  onReported,
}: {
  conversationId: string;
  open: boolean;
  onClose: () => void;
  onReported: () => void;
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
      onReported();
    } catch (err) {
      setError(messageForError(err));
    }
    setSubmitting(false);
  }

  return (
    // dialogの色は継承されないため、会話画面と同じ配色をここでも指定する。
    // mt-autoで画面の下に寄せ、上の角だけを丸めてシートにする。並べ方は中の箱に
    // 任せる。dialog自身をflexにすると、閉じているときのdisplay:noneを打ち消す。
    <dialog
      ref={dialogRef}
      onClose={onClose}
      aria-labelledby="report-dialog-title"
      className="bg-chat-panel text-chat-ink font-rounded backdrop:bg-chat-ink/50 mx-auto mt-auto w-full max-w-lg max-h-[90dvh] overflow-y-auto rounded-t-[32px] px-5 pt-3 pb-6 leading-[1.8] font-medium"
    >
      <div className="flex flex-col gap-3.5">
        <span
          aria-hidden
          className="bg-chat-line h-[5px] w-11 flex-none self-center rounded-full"
        />

        <h2 id="report-dialog-title" className="text-xl leading-snug font-black">
          この会話を通報する
        </h2>

        {submitted ? (
          <>
            <p className="text-chat-ink-muted text-[13px] leading-[1.8]">{done}</p>
            <button type="button" onClick={onClose} className={submitButtonClass}>
              閉じる
            </button>
          </>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-3.5">
            <p className="text-chat-ink-muted text-[13px] leading-[1.8]">{lead}</p>

            <fieldset className="m-0 border-0 p-0">
              <legend className="mb-2 p-0 text-sm font-black">どれにあてはまりますか</legend>
              <div className="flex flex-col gap-2">
                {reportReasons.map((option) => (
                  <label
                    key={option.value}
                    className={`focus-within:outline-chat-self flex min-h-[46px] cursor-pointer items-center gap-3 rounded-2xl border-[1.5px] px-4 text-sm focus-within:outline-2 focus-within:outline-offset-2 ${
                      reason === option.value
                        ? "border-chat-self bg-chat-alert-soft font-bold"
                        : "border-chat-option-line bg-chat-panel"
                    }`}
                  >
                    <input
                      type="radio"
                      name="reason"
                      value={option.value}
                      checked={reason === option.value}
                      onChange={() => setReason(option.value)}
                      className="accent-chat-self-edge m-0 h-[18px] w-[18px] flex-none"
                    />
                    {option.label}
                  </label>
                ))}
              </div>
            </fieldset>

            {error !== null && (
              <p role="alert" className="text-chat-alert text-[13px] leading-[1.7]">
                {error}
              </p>
            )}

            <div className="mt-1 grid grid-cols-2 gap-2.5">
              <button
                type="button"
                onClick={onClose}
                disabled={submitting}
                className={cancelButtonClass}
              >
                やめる
              </button>
              <button type="submit" disabled={submitting} className={submitButtonClass}>
                {submitting ? "送信しています…" : "通報する"}
              </button>
            </div>
          </form>
        )}
      </div>
    </dialog>
  );
}
