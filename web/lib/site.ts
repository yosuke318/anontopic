export const siteName = "anontopic";

export const siteTagline = "話したいことから始まる匿名チャット";

export const siteDescription =
  "会員登録なしで、選んだトピックについて匿名でテキストチャットできるサービス。雑談・趣味・相談のための場所です。";

const defaultSiteUrl = "http://localhost:3000";

// 絶対 URL を要求する OGP・sitemap・canonical のために、公開時のオリジンを持つ。
export const siteUrl = new URL(process.env.NEXT_PUBLIC_SITE_URL ?? defaultSiteUrl);

export function absoluteUrl(path: string): string {
  return new URL(path, siteUrl).toString();
}
