import { ApiError, apiBaseUrl, retryAfterSeconds } from "@/lib/api";

// 通報の分類。参加者は互いに匿名で、部屋の中の参加者番号は部屋の外では意味を
// 持たないため、通報するのは相手ではなく会話そのものになる。理由は
// docs/adr/0015-report-a-conversation-rather-than-a-participant.mdにある。
export const reportReasons = [
  { value: "sexual", label: "性的な内容・出会い目的の書き込み" },
  { value: "contact", label: "連絡先の交換・外部への誘導" },
  { value: "harassment", label: "暴言・いやがらせ" },
  { value: "spam", label: "宣伝・スパム" },
  { value: "other", label: "その他" },
] as const;

export type ReportReason = (typeof reportReasons)[number]["value"];

// submitReportは会話を通報する。同じ会話を繰り返し通報しても記録は増えない。
export async function submitReport(conversationId: string, reason: ReportReason): Promise<void> {
  const res = await fetch(`${apiBaseUrl()}/api/reports`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ conversation_id: conversationId, reason }),
  });
  if (!res.ok) {
    throw new ApiError(res.status, retryAfterSeconds(res));
  }
}
