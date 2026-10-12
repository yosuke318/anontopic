import { adsenseClient, adsensePublisherId } from "@/lib/ads";

// 発行者 ID はビルドのときに決まるため、ビルドの時点で生成する。
export const dynamic = "force-static";

// f08c47fec0942fa0 は Google の認証機関 ID で、どの発行者でも同じ値を書く。
export function GET() {
  if (adsenseClient === "") {
    return new Response("Not Found\n", {
      status: 404,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }
  return new Response(`google.com, ${adsensePublisherId()}, DIRECT, f08c47fec0942fa0\n`, {
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
}
