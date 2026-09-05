export type Topic = {
  id: number;
  name: string;
};

// サーバーコンポーネントからは同一ネットワーク内の API を直接呼ぶ。ブラウザに配る
// NEXT_PUBLIC_API_BASE_URL とはホスト名が異なりうるため、別の変数で受ける。
function serverApiBaseUrl(): string {
  return (
    process.env.API_BASE_URL ?? process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080"
  );
}

function parseTopics(body: unknown): Topic[] {
  if (typeof body !== "object" || body === null || !("topics" in body)) {
    throw new Error("topic list has no topics field");
  }

  const { topics } = body as { topics: unknown };
  if (!Array.isArray(topics)) {
    throw new Error("topics is not an array");
  }

  return topics.map((topic) => {
    if (
      typeof topic !== "object" ||
      topic === null ||
      typeof (topic as Topic).id !== "number" ||
      typeof (topic as Topic).name !== "string"
    ) {
      throw new Error("topic is missing id or name");
    }
    return { id: (topic as Topic).id, name: (topic as Topic).name };
  });
}

// 選択できるトピックを API から読む。API 側がプロセス内に一定時間持っているため、
// Next.js 側では保持せずリクエストごとに取りに行く。
export async function fetchTopics(): Promise<Topic[]> {
  const res = await fetch(`${serverApiBaseUrl()}/api/topics`, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`list topics: ${res.status}`);
  }
  return parseTopics(await res.json());
}
