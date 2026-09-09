import type { MetadataRoute } from "next";

import { absoluteUrl } from "@/lib/site";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      // 待機画面と会話はセッションを持つ人にしか意味がなく、内容も毎回変わる。
      disallow: ["/waiting", "/rooms/"],
    },
    sitemap: absoluteUrl("/sitemap.xml"),
  };
}
