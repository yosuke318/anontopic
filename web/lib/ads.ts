// AdSense の設定はビルドのときに埋め込む。発行者 ID が無い環境では、サイトの確認用の
// meta タグも ads.txt も出さず、広告ユニットの ID が無い環境では広告枠を描かない（ADR-0038）。

const clientPattern = /^ca-pub-\d+$/;

// 形の崩れた発行者 ID で ads.txt を出すと、正しい ID の広告まで配信されなくなる。
function parseClient(value: string | undefined): string {
  if (value === undefined || value === "") {
    return "";
  }
  if (!clientPattern.test(value)) {
    console.warn(`NEXT_PUBLIC_ADSENSE_CLIENT is not a ca-pub-... ID, ads are disabled: ${value}`);
    return "";
  }
  return value;
}

export const adsenseClient = parseClient(process.env.NEXT_PUBLIC_ADSENSE_CLIENT);

export const adsenseSlot = process.env.NEXT_PUBLIC_ADSENSE_SLOT ?? "";

export const adUnitsEnabled = adsenseClient !== "" && adsenseSlot !== "";

// ads.txt に書く発行者 ID は、広告タグの ID から先頭の "ca-" を除いたもの。
export function adsensePublisherId(): string {
  return adsenseClient.replace(/^ca-/, "");
}
