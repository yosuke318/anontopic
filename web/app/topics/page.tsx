import { Suspense } from "react";
import type { Metadata } from "next";

import { TopicPicker } from "@/components/topic-picker";
import { siteName } from "@/lib/site";
import { fetchTopics, type Topic } from "@/lib/topics";

const lead =
  "トピックと人数を選ぶと、同じトピックを選んだ人を待つ画面に移ります。会員登録はいりません。";

// トピックは管理 API から変えられるため、ビルド時ではなくリクエストごとに読む。
export const dynamic = "force-dynamic";

const description =
  "話したいトピックと、2 人ルームか 3 人ルームかを選んで、匿名のテキストチャットを始めます。会員登録はいりません。";

export const metadata: Metadata = {
  title: "トピックを選ぶ",
  description,
  alternates: { canonical: "/topics" },
  openGraph: {
    title: `トピックを選ぶ | ${siteName}`,
    description,
    url: "/topics",
  },
};

function Notice({ title, body }: { title: string; body: string }) {
  return (
    <div className="border-line rounded-2xl border p-6">
      <h2 className="font-bold">{title}</h2>
      <p className="text-muted mt-3 leading-7">{body}</p>
      <a
        href="/topics"
        className="border-line hover:bg-surface mt-6 inline-flex h-11 items-center justify-center rounded-full border px-6 text-sm font-medium transition-colors"
      >
        読み込み直す
      </a>
    </div>
  );
}

function PickerSkeleton() {
  return (
    <div aria-hidden className="animate-pulse">
      <div className="bg-surface-strong h-7 w-48 rounded" />
      <div className="mt-8 flex flex-wrap gap-3">
        {[80, 112, 96, 128, 88, 104].map((width, index) => (
          <div
            key={index}
            className="bg-surface-strong h-11 rounded-full"
            style={{ width: `${width}px` }}
          />
        ))}
      </div>
      <div className="bg-surface-strong mt-12 h-7 w-32 rounded" />
      <div className="mt-8 grid gap-3 sm:grid-cols-2">
        <div className="bg-surface-strong h-28 rounded-2xl" />
        <div className="bg-surface-strong h-28 rounded-2xl" />
      </div>
    </div>
  );
}

async function Picker() {
  let topics: Topic[];
  try {
    topics = await fetchTopics();
  } catch {
    return (
      <Notice
        title="トピックを読み込めませんでした"
        body="サーバーが応答しませんでした。しばらく待ってから読み込み直してください。"
      />
    );
  }

  if (topics.length === 0) {
    return (
      <Notice
        title="いま選べるトピックがありません"
        body="トピックの準備ができるまでお待ちください。"
      />
    );
  }

  return <TopicPicker topics={topics} />;
}

export default function TopicsPage() {
  return (
    <div className="mx-auto w-full max-w-3xl px-5 py-16">
      <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">何について話しますか</h1>
      <p className="text-muted mt-6 leading-8">{lead}</p>
      <div className="mt-12">
        <Suspense fallback={<PickerSkeleton />}>
          <Picker />
        </Suspense>
      </div>
    </div>
  );
}
