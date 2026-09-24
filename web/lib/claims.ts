import { ApiError, apiBaseUrl, retryAfterSeconds } from "@/lib/api";

// 申し立てで選ぶ権利の種類。理由はdocs/adr/0022-take-rights-infringement-claims-from-anyone.mdにある。
export const claimRights = [
  { value: "privacy", label: "プライバシーの侵害（氏名・住所・勤務先などの書き込み）" },
  { value: "defamation", label: "名誉・信用の毀損" },
  { value: "copyright", label: "著作権の侵害" },
  { value: "other", label: "その他の権利" },
] as const;

export type ClaimRight = (typeof claimRights)[number]["value"];

export const claimNameMaxLength = 100;
export const claimEmailMaxLength = 254;
export const claimDetailsMaxLength = 4000;

export type ClaimInput = {
  name: string;
  email: string;
  right: ClaimRight;
  details: string;
};

// submitClaimは権利侵害の申し立てを送る。利用者でない人も送れるよう、セッションは使わない。
export async function submitClaim(claim: ClaimInput): Promise<void> {
  const res = await fetch(`${apiBaseUrl()}/api/claims`, {
    method: "POST",
    credentials: "omit",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(claim),
  });
  if (!res.ok) {
    throw new ApiError(res.status, retryAfterSeconds(res));
  }
}
