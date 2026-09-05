import type { Metadata } from "next";

import { WaitingPanel } from "@/components/waiting-panel";
import { fetchTopics, type Topic } from "@/lib/topics";

// 待機の状態はセッションごとに違うため、静的に持てるものが無い。
export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "待機中",
  robots: { index: false, follow: false },
};

export default async function WaitingPage() {
  // 表示に使うのはトピック名だけなので、読めなかった場合も待機画面は出す。
  let topics: Topic[] = [];
  try {
    topics = await fetchTopics();
  } catch {
    topics = [];
  }

  return (
    <div className="mx-auto w-full max-w-2xl px-5 py-16">
      <WaitingPanel topics={topics} />
    </div>
  );
}
