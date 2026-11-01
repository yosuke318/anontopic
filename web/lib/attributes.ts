// 利用者が任意で答える性別と年代。統計と荒らしの対策にだけ使い、マッチングにも相手への
// 表示にも使わない。値はサーバーの session モジュールが受け付けるものと同じ。

export const genderOptions = [
  { value: "female", label: "女性" },
  { value: "male", label: "男性" },
  { value: "other", label: "その他" },
] as const;

export const ageBandOptions = [
  { value: "under_18", label: "18 歳未満" },
  { value: "18_19", label: "18〜19 歳" },
  { value: "20s", label: "20 代" },
  { value: "30s", label: "30 代" },
  { value: "40s", label: "40 代" },
  { value: "50s", label: "50 代" },
  { value: "60_plus", label: "60 代以上" },
] as const;

export type Gender = (typeof genderOptions)[number]["value"];

export type AgeBand = (typeof ageBandOptions)[number]["value"];

// 答えなかった項目は null にする。
export type Attributes = {
  gender: Gender | null;
  age_band: AgeBand | null;
};

export const attributesNotice = "マッチングには使いません。相手にも表示されません。";
