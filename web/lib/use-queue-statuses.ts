"use client";

import { useEffect, useState } from "react";

import { fetchQueueStatuses, type RoomType } from "@/lib/matching";

// 待っている人の有無を読み直す間隔。APIは答えを数秒使い回すため、これより短くしても
// 新しい答えは返らない。
const pollIntervalMs = 10000;

function queueKey(topicId: number, roomType: RoomType): string {
  return `${topicId}:${roomType}`;
}

// useQueueStatusesは、キューごとに待っている人がいるかを読み続け、問い合わせる関数を
// 返す。まだ読めていないとき、読めなかったときはnullを返す。タブが裏にある間は読まない。
export function useQueueStatuses(): (topicId: number, roomType: RoomType) => boolean | null {
  const [waiting, setWaiting] = useState<Map<string, boolean> | null>(null);

  useEffect(() => {
    let cancelled = false;
    let loading = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    async function load() {
      // タブが表に戻ったときと次の読み直しが重なっても、問い合わせは 1 本に保つ。
      if (loading) {
        return;
      }
      loading = true;
      clearTimeout(timer);
      try {
        const statuses = await fetchQueueStatuses();
        if (cancelled) {
          return;
        }
        setWaiting(
          new Map(
            statuses.map((status) => [queueKey(status.topic_id, status.room_type), status.waiting]),
          ),
        );
      } catch {
        // 古い表示を残すと、いない相手を待っていると誤解させるため消す。
        if (!cancelled) {
          setWaiting(null);
        }
      }
      loading = false;
      if (!cancelled && document.visibilityState === "visible") {
        timer = setTimeout(load, pollIntervalMs);
      }
    }

    function handleVisibilityChange() {
      if (document.visibilityState === "visible") {
        void load();
      } else {
        clearTimeout(timer);
      }
    }

    void load();
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      cancelled = true;
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, []);

  return (topicId, roomType) => waiting?.get(queueKey(topicId, roomType)) ?? null;
}
