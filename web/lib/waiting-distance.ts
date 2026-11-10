// 待機中のゲームで進んだ距離を、会話の画面に渡すまで持っておく。待機画面と会話の画面は
// 別のページなので、タブを閉じれば消える sessionStorage に会話 ID ごとに置く。
// 読み書きできないブラウザでは、距離を送らずに会話へ進む。

const keyPrefix = "waiting-distance:";

export function saveWaitingDistance(conversationId: string, distance: number): void {
  try {
    sessionStorage.setItem(keyPrefix + conversationId, String(Math.floor(distance)));
  } catch {
    // 保存できなくても会話には入れる。
  }
}

export function loadWaitingDistance(conversationId: string): number | null {
  try {
    const value = sessionStorage.getItem(keyPrefix + conversationId);
    if (value === null) {
      return null;
    }
    const distance = Number(value);
    return Number.isInteger(distance) && distance >= 0 ? distance : null;
  } catch {
    return null;
  }
}

// formatDistanceは距離を「1,234 m」の形にする。
export function formatDistance(distance: number): string {
  return `${Math.floor(distance).toLocaleString("ja-JP")} m`;
}
