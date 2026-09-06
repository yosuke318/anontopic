export const siteName = "anontopic";

export const siteTagline = "話したいことから始まる匿名チャット";

export const siteDescription =
  "会員登録なしで、選んだトピックについて匿名でテキストチャットできるサービス。雑談・趣味・相談のための場所です。";

const defaultSiteUrl = "http://localhost:3000";

// 設定値が URL として読めないときは既定のオリジンを使う。ここで例外を投げると、
// このモジュールを読むすべてのページが 500 になる。
function parseSiteUrl(value: string | undefined): URL {
  if (value === undefined) {
    return new URL(defaultSiteUrl);
  }
  try {
    return new URL(value);
  } catch {
    console.warn(
      `NEXT_PUBLIC_SITE_URL is not a URL, using ${defaultSiteUrl} for absolute links: ${value}`,
    );
    return new URL(defaultSiteUrl);
  }
}

// 絶対 URL を要求する OGP・sitemap・canonical のために、公開時のオリジンを持つ。
export const siteUrl = parseSiteUrl(process.env.NEXT_PUBLIC_SITE_URL);

export function absoluteUrl(path: string): string {
  return new URL(path, siteUrl).toString();
}
