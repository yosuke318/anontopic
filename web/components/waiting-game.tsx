"use client";

import { type PointerEvent, type RefObject, useEffect, useRef, useState } from "react";

import { formatDistance } from "@/lib/waiting-distance";
import {
  type Circle,
  Enemy,
  type EnemyKind,
  type Rect,
  appearanceWindow,
  enemyNames,
  planAppearances,
  worldHeight,
  worldWidth,
} from "@/lib/waiting-game/enemies";
import { drawEnemy, drawStrikes, labelColors } from "@/lib/waiting-game/sprites";

// 敵が現れたことを知らせる文字を出しておく秒数。
const enemyLabelDuration = 2;

// previewKindは開発中にだけ読む、?game-preview=<敵の名前>の値。遊び始めるとすぐその敵が現れ、
// 自機は当たっても止まらない。本番のビルドでは読まない。
function previewKind(): EnemyKind | null {
  if (process.env.NODE_ENV !== "development") {
    return null;
  }
  const value = new URLSearchParams(window.location.search).get("game-preview");
  return value === "turtle" || value === "tiger" || value === "suzaku" || value === "dragon"
    ? value
    : null;
}

const playerRadius = 11;
const playerBottomGap = 64;

// 落ちてくる速さ（座標/秒）。時間がたつほど速くなり、待機の終わりごろに上限に届く。
const baseSpeed = 120;
const speedGainPerSecond = 0.4;
const maxSpeed = 340;

// 障害物が出てくる間隔（秒）。時間がたつほど短くなり、待機の終わりごろに下限に届く。
const baseSpawnInterval = 0.85;
const minSpawnInterval = 0.24;
const spawnIntervalLossPerSecond = 0.0012;

// 座標で10進むと1mとして数える。
const unitsPerMetre = 10;

// 指でなぞった量を自機の動きに直すときの倍率。1より大きくして、短くなぞっても大きく動ける
// ようにする。
const dragGain = 1.4;
const keySpeed = 240;

const heldKeys: Record<string, [number, number]> = {
  ArrowLeft: [-1, 0],
  ArrowRight: [1, 0],
  ArrowUp: [0, -1],
  ArrowDown: [0, 1],
  a: [-1, 0],
  d: [1, 0],
  w: [0, -1],
  s: [0, 1],
};

// WaitingGameStatsは待機画面がゲームの結果を読むための値。ゲームはこれを書き換えるだけで、
// 待機画面が再描画されることはない。
export type WaitingGameStats = {
  // bestはぶつかって終わった回のうち、いちばん進んだ距離。
  best: number;
  // currentは今遊んでいる回の距離。遊んでいなければ0。
  current: number;
  // startedは一度でも遊び始めたか。
  started: boolean;
  // crashedは一度でも障害物に当たったか。
  crashed: boolean;
};

export function emptyWaitingGameStats(): WaitingGameStats {
  return { best: 0, current: 0, started: false, crashed: false };
}

// bestDistanceはこの待機の間にいちばん進んだ距離を返す。
export function bestDistance(stats: WaitingGameStats): number {
  return Math.max(stats.best, stats.current);
}

type Phase = "idle" | "running" | "crashed" | "paused";

type PointerHandler = (event: PointerEvent<HTMLCanvasElement>) => void;

type Controls = {
  start: () => void;
  grab: PointerHandler;
  drag: PointerHandler;
  release: PointerHandler;
};

type Obstacle = { x: number; y: number; width: number; height: number; speed: number; tone: 0 | 1 };

type Palette = {
  panel: string;
  line: string;
  ink: string;
  muted: string;
  brand: string;
  brandEdge: string;
  obstacles: [string, string];
  obstacleEdges: [string, string];
  font: string;
};

function readPalette(): Palette {
  const root = getComputedStyle(document.documentElement);
  const token = (name: string, fallback: string) => root.getPropertyValue(name).trim() || fallback;
  return {
    panel: token("--chat-panel", "#ffffff"),
    line: token("--chat-line", "#ead9df"),
    ink: token("--foreground", "#3b2f36"),
    muted: token("--muted", "#74636c"),
    brand: token("--brand", "#ff8fab"),
    brandEdge: token("--brand-edge", "#e8718f"),
    obstacles: [token("--chat-peer-1-soft", "#ddf3ea"), token("--chat-peer-2-soft", "#ece6fa")],
    obstacleEdges: [token("--chat-peer-1", "#9ed9c2"), token("--chat-peer-2", "#c7b9f2")],
    font: getComputedStyle(document.body).fontFamily || "sans-serif",
  };
}

function currentSpeed(elapsed: number): number {
  return Math.min(baseSpeed + elapsed * speedGainPerSecond, maxSpeed);
}

function randomBetween(min: number, max: number): number {
  return min + Math.random() * (max - min);
}

// 当たり判定に使う自機の半径。角の丸みや絵のふちのぶん、少しだけ甘くする。
const playerReach = playerRadius * 0.85;

// hitsは自機の円が四角（障害物や縦に貫く攻撃）に触れているかを返す。
function hits(x: number, y: number, rect: Rect): boolean {
  const nearestX = Math.min(Math.max(x, rect.x), rect.x + rect.width);
  const nearestY = Math.min(Math.max(y, rect.y), rect.y + rect.height);
  const dx = x - nearestX;
  const dy = y - nearestY;
  return dx * dx + dy * dy < playerReach * playerReach;
}

// hitsCircleは自機の円が敵の体や弾の円に触れているかを返す。
function hitsCircle(x: number, y: number, circle: Circle): boolean {
  const reach = playerReach + circle.radius;
  return (x - circle.x) ** 2 + (y - circle.y) ** 2 < reach * reach;
}

const overlayCopy = {
  idle: {
    title: "よけてあそぶ",
    body: "画面をなぞると、なぞった向きに動きます。キーボードなら矢印キーか WASD。2 分を過ぎると、四神がひとりずつ現れます。",
    action: "あそぶ",
  },
  crashed: { title: "ぶつかった！", body: "", action: "もう一度" },
  paused: { title: "一時停止中", body: "", action: "つづける" },
} as const;

// WaitingGameは待機画面に置く、上から落ちてくる障害物を縦横に動いてよけるゲーム。
// 結果はstatsに書き、待機画面は相手が見つかったときと待機が終わったときにそれを読む。
export function WaitingGame({ stats }: { stats: RefObject<WaitingGameStats> }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [phase, setPhase] = useState<Phase>("idle");
  const [lastDistance, setLastDistance] = useState(0);
  const [best, setBest] = useState(0);

  // キャンバスの操作とオーバーレイのボタンが、同じ開始・移動の処理を呼ぶための入り口。
  const controls = useRef<Controls>({
    start: () => {},
    grab: () => {},
    drag: () => {},
    release: () => {},
  });

  useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext("2d");
    if (!canvas || !context) {
      return;
    }

    const palette = readPalette();
    let current: Phase = "idle";
    let player = { x: worldWidth / 2, y: worldHeight - playerBottomGap };
    let obstacles: Obstacle[] = [];
    let elapsed = 0;
    let distance = 0;
    let spawnIn = 0.6;
    let laneOffset = 0;
    let lastFrame: number | null = null;
    let frame = 0;
    let scale = 1;
    let enemy: Enemy | null = null;
    let appearances: { kind: EnemyKind; at: number }[] = [];
    const preview = previewKind();
    const keys = new Set<string>();
    let pointer: { id: number; x: number; y: number } | null = null;

    function enter(next: Phase) {
      current = next;
      setPhase(next);
    }

    function clampPlayer() {
      player = {
        x: Math.min(Math.max(player.x, playerRadius), worldWidth - playerRadius),
        y: Math.min(Math.max(player.y, playerRadius), worldHeight - playerRadius),
      };
    }

    function start() {
      if (current === "paused") {
        lastFrame = null;
        enter("running");
        run();
        return;
      }
      if (current === "running") {
        return;
      }
      player = { x: worldWidth / 2, y: worldHeight - playerBottomGap };
      obstacles = [];
      enemy = null;
      appearances = planAppearances();
      elapsed = 0;
      if (preview !== null) {
        elapsed = appearanceWindow(preview).from;
        appearances = [{ kind: preview, at: elapsed + 1 }];
      }
      distance = 0;
      spawnIn = 0.6;
      lastFrame = null;
      stats.current.started = true;
      stats.current.current = 0;
      enter("running");
      run();
    }

    function crash() {
      if (preview !== null) {
        return;
      }
      stats.current.crashed = true;
      stats.current.best = Math.max(stats.current.best, distance);
      stats.current.current = 0;
      pointer = null;
      keys.clear();
      setLastDistance(distance);
      setBest(stats.current.best);
      enter("crashed");
    }

    function spawn() {
      const width = randomBetween(26, 84);
      const speed = currentSpeed(elapsed);
      obstacles.push({
        x: randomBetween(-width / 3, worldWidth - (width * 2) / 3),
        y: -30,
        width,
        height: randomBetween(14, 26),
        speed: speed * randomBetween(0.8, 1.25),
        tone: Math.random() < 0.5 ? 0 : 1,
      });
      const interval = Math.max(
        baseSpawnInterval - elapsed * spawnIntervalLossPerSecond,
        minSpawnInterval,
      );
      spawnIn = interval * randomBetween(0.6, 1.4);
    }

    function step(dt: number) {
      elapsed += dt;
      const speed = currentSpeed(elapsed);
      distance += (speed * dt) / unitsPerMetre;
      stats.current.current = distance;
      laneOffset = (laneOffset + speed * dt) % 48;

      let dx = 0;
      let dy = 0;
      for (const key of keys) {
        const direction = heldKeys[key];
        if (direction) {
          dx += direction[0];
          dy += direction[1];
        }
      }
      if (dx !== 0 || dy !== 0) {
        const length = Math.hypot(dx, dy);
        player = {
          x: player.x + (dx / length) * keySpeed * dt,
          y: player.y + (dy / length) * keySpeed * dt,
        };
        clampPlayer();
      }

      // 敵は1体ずつ。前の敵が帰って、攻撃も消えてから次が現れる。
      if (enemy === null && appearances.length > 0 && appearances[0].at <= elapsed) {
        const next = appearances.shift();
        if (next !== undefined) {
          enemy = new Enemy(next.kind);
        }
      }
      enemy?.update(dt, player);
      if (enemy?.finished) {
        enemy = null;
      }

      // 敵がいる間は、ふつうの障害物の間隔をのばす。青龍の雷の間は出さない。
      if (!enemy?.blocksObstacles) {
        spawnIn -= dt / (enemy?.spawnFactor ?? 1);
        if (spawnIn <= 0) {
          spawn();
        }
      }

      for (const obstacle of obstacles) {
        obstacle.y += obstacle.speed * dt;
      }
      obstacles = obstacles.filter((obstacle) => obstacle.y < worldHeight + 10);

      const hazards = enemy?.hazards();
      if (
        obstacles.some((obstacle) => hits(player.x, player.y, obstacle)) ||
        hazards?.rects.some((rect) => hits(player.x, player.y, rect)) ||
        hazards?.circles.some((circle) => hitsCircle(player.x, player.y, circle))
      ) {
        crash();
      }
    }

    function roundedRect(x: number, y: number, width: number, height: number, radius: number) {
      if (!context) {
        return;
      }
      context.beginPath();
      context.roundRect(x, y, width, height, radius);
    }

    function draw() {
      if (!canvas || !context) {
        return;
      }
      context.setTransform(scale, 0, 0, scale, 0, 0);
      context.fillStyle = palette.panel;
      context.fillRect(0, 0, worldWidth, worldHeight);

      // 流れていく線で、進んでいることを見せる。
      context.strokeStyle = palette.line;
      context.lineWidth = 2;
      context.setLineDash([14, 34]);
      context.lineDashOffset = -laneOffset;
      for (const x of [worldWidth / 4, worldWidth / 2, (worldWidth * 3) / 4]) {
        context.beginPath();
        context.moveTo(x, 0);
        context.lineTo(x, worldHeight);
        context.stroke();
      }
      context.setLineDash([]);

      if (enemy !== null) {
        drawStrikes(context, enemy);
      }

      for (const obstacle of obstacles) {
        context.fillStyle = palette.obstacles[obstacle.tone];
        context.strokeStyle = palette.obstacleEdges[obstacle.tone];
        context.lineWidth = 2;
        roundedRect(obstacle.x, obstacle.y, obstacle.width, obstacle.height, 7);
        context.fill();
        context.stroke();
      }

      if (enemy !== null) {
        drawEnemy(context, enemy);
      }

      context.fillStyle = palette.brand;
      context.strokeStyle = palette.brandEdge;
      context.lineWidth = 3;
      context.beginPath();
      context.arc(player.x, player.y, playerRadius, 0, Math.PI * 2);
      context.fill();
      context.stroke();
      context.fillStyle = "rgba(255, 255, 255, 0.7)";
      context.beginPath();
      context.arc(player.x - 3.5, player.y - 3.5, 3, 0, Math.PI * 2);
      context.fill();

      context.fillStyle = palette.ink;
      context.font = `700 16px ${palette.font}`;
      context.textBaseline = "top";
      context.textAlign = "left";
      context.fillText(formatDistance(distance), 14, 12);
      if (stats.current.best > 0) {
        context.fillStyle = palette.muted;
        context.font = `500 12px ${palette.font}`;
        context.textAlign = "right";
        context.fillText(`ベスト ${formatDistance(stats.current.best)}`, worldWidth - 14, 14);
      }

      if (enemy !== null && enemy.age < enemyLabelDuration) {
        context.save();
        context.globalAlpha = Math.min(1, (enemyLabelDuration - enemy.age) * 2);
        context.fillStyle = labelColors[enemy.kind];
        context.font = `700 15px ${palette.font}`;
        context.textAlign = "center";
        context.fillText(`${enemyNames[enemy.kind]}があらわれた！`, worldWidth / 2, 96);
        context.restore();
      }
    }

    // 遊んでいない間は絵が変わらないため、フレームを回すのは遊んでいる間だけにする。
    // 止まっている間の絵は、止まった回の最後のフレームか、大きさが変わったときに描き直したもの。
    function loop(now: number) {
      if (current !== "running") {
        frame = 0;
        return;
      }
      const dt = lastFrame === null ? 0 : Math.min((now - lastFrame) / 1000, 0.05);
      lastFrame = now;
      step(dt);
      draw();
      frame = current === "running" ? requestAnimationFrame(loop) : 0;
    }

    function run() {
      if (frame === 0) {
        frame = requestAnimationFrame(loop);
      }
    }

    function resize() {
      if (!canvas) {
        return;
      }
      const ratio = window.devicePixelRatio || 1;
      const width = canvas.clientWidth;
      canvas.width = Math.round(width * ratio);
      canvas.height = Math.round((width * worldHeight * ratio) / worldWidth);
      scale = canvas.width / worldWidth;
      draw();
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (current !== "running" || !(event.key in heldKeys)) {
        return;
      }
      event.preventDefault();
      keys.add(event.key);
    }

    function handleKeyUp(event: KeyboardEvent) {
      keys.delete(event.key);
    }

    function handleVisibilityChange() {
      if (document.visibilityState === "hidden" && current === "running") {
        pointer = null;
        keys.clear();
        enter("paused");
      }
    }

    controls.current = {
      start,
      grab(event) {
        if (current !== "running") {
          start();
        }
        pointer = { id: event.pointerId, x: event.clientX, y: event.clientY };
        event.currentTarget.setPointerCapture(event.pointerId);
      },
      drag(event) {
        if (current !== "running" || pointer === null || pointer.id !== event.pointerId) {
          return;
        }
        const ratio = worldWidth / event.currentTarget.clientWidth;
        player = {
          x: player.x + (event.clientX - pointer.x) * ratio * dragGain,
          y: player.y + (event.clientY - pointer.y) * ratio * dragGain,
        };
        clampPlayer();
        pointer = { ...pointer, x: event.clientX, y: event.clientY };
      },
      release(event) {
        if (pointer?.id === event.pointerId) {
          pointer = null;
        }
      },
    };

    const observer = new ResizeObserver(resize);
    observer.observe(canvas);
    resize();
    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("keyup", handleKeyUp);
    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("keyup", handleKeyUp);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [stats]);

  const overlay = phase === "running" ? null : overlayCopy[phase];
  const crashedBody =
    phase === "crashed"
      ? `${formatDistance(lastDistance)} 進みました。ベストは ${formatDistance(best)}。`
      : "";

  return (
    // スマホでも残り時間とゲームが一度に見えるよう、高さを画面の6割ほどに抑える。
    <div
      className="border-line relative mx-auto overflow-hidden rounded-2xl border"
      style={{ maxWidth: `calc(60svh * ${worldWidth / worldHeight})` }}
    >
      <canvas
        ref={canvasRef}
        aria-label="障害物をよけるゲーム"
        onPointerDown={(event) => controls.current.grab(event)}
        onPointerMove={(event) => controls.current.drag(event)}
        onPointerUp={(event) => controls.current.release(event)}
        onPointerCancel={(event) => controls.current.release(event)}
        className="block aspect-[320/420] w-full cursor-pointer touch-none select-none"
      />
      {overlay !== null && (
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center bg-white/75 p-6 text-center">
          <p className="text-lg font-bold">{overlay.title}</p>
          {(overlay.body !== "" || crashedBody !== "") && (
            <p className="text-muted mt-2 text-sm leading-6">{overlay.body || crashedBody}</p>
          )}
          <button
            type="button"
            onClick={() => controls.current.start()}
            className="bg-brand text-brand-contrast hover:bg-brand-hover shadow-brand pointer-events-auto mt-5 inline-flex h-11 items-center justify-center rounded-full px-7 font-bold transition-colors"
          >
            {overlay.action}
          </button>
        </div>
      )}
    </div>
  );
}
