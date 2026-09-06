import type { Metadata, Viewport } from "next";

import { ChatRoom } from "@/components/chat-room";
import { fetchTopics, type Topic } from "@/lib/topics";

// 会話はセッションごとに違うため、静的に持てるものが無い。
export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "会話",
  robots: { index: false, follow: false },
};

// ソフトキーボードが開いたときにビューポートごと縮め、入力欄が隠れないようにする。
export const viewport: Viewport = {
  interactiveWidget: "resizes-content",
};

export default async function RoomPage({ params }: PageProps<"/rooms/[roomID]">) {
  const { roomID } = await params;

  // どのトピックの会話かは接続して初めて分かるため、名前を引く表だけ渡す。
  // 読めなかった場合も会話そのものには入れる。
  let topics: Topic[] = [];
  try {
    topics = await fetchTopics();
  } catch {
    topics = [];
  }

  return <ChatRoom conversationId={roomID} topics={topics} />;
}
