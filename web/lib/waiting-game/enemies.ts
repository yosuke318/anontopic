// 待機ゲームに出てくる四神（亀・白虎・朱雀・青龍）の動きと当たり判定。描画は持たず、
// sprites.tsが状態を読んで描く。
//
// 共通のきまり。
// 1. 攻撃は必ず予告してから出す。
// 2. 敵がいる間は、ふつうの障害物の間隔をのばす。
// 3. 攻撃で画面全部をふさがず、自機2つ分（幅48）以上のすき間を残す。
// 4. 弾は障害物の落下より遅くする。
// 5. 敵の体に触れても終わる。
// 6. 敵は12〜15秒いたら帰る。倒すものではなく、しのぐもの。

export const worldWidth = 320;
export const worldHeight = 420;

export type EnemyKind = "turtle" | "tiger" | "suzaku" | "dragon";

export type Point = { x: number; y: number };
export type Circle = { x: number; y: number; radius: number };
export type Rect = { x: number; y: number; width: number; height: number };

export type BulletKind = "shell" | "spark" | "feather" | "pearl";

export type Bullet = {
  kind: BulletKind;
  x: number;
  y: number;
  vx: number;
  vy: number;
  radius: number;
  // ageは出てからの秒数。尾羽の揺れと宝珠の寿命に使う。
  age: number;
  // bouncesは左右の壁ではね返った回数。甲羅だけが使う。
  bounces: number;
  // swayFromは尾羽が揺れる中心のx。
  swayFrom: number;
};

export type StrikeKind = "claw" | "thunder";

// Strikeは画面を縦に貫く攻撃（白虎の爪あと、青龍の雷）。activeFromまでは予告で当たらず、
// activeFromからuntilまでの間だけ当たる。
export type Strike = {
  kind: StrikeKind;
  x: number;
  width: number;
  warnFrom: number;
  activeFrom: number;
  until: number;
};

// Cueは敵の体に出す予告。fromからtoの間、kindの見た目にする。
export type CueKind = "retract" | "glow";
type Cue = { kind: CueKind; from: number; to: number };

type Event = { at: number; run: (player: Point) => void };

export const enemyNames: Record<EnemyKind, string> = {
  turtle: "亀",
  tiger: "白虎",
  suzaku: "朱雀",
  dragon: "青龍",
};

// 出てくる時間帯（遊び始めてからの秒）。敵は時間帯の中のどこかで現れ、時間帯の内に帰る。
const appearances: { kind: EnemyKind; from: number; to: number }[] = [
  { kind: "turtle", from: 120, to: 180 },
  { kind: "tiger", from: 240, to: 300 },
  { kind: "suzaku", from: 420, to: 480 },
  { kind: "dragon", from: 540, to: 600 },
];

// 敵が画面の上から入ってくるのと、帰っていくのにかける秒数。
const travelDuration = 0.8;
const restY = 48;
const bodyRadius = 18;

// 敵ごとの攻撃の値。warningは予告の秒数、activeは当たる秒数、Speedは座標/秒、
// spawnFactorはこの敵がいる間にふつうの障害物の間隔を何倍にするか。
const turtle = {
  stay: 13,
  attacks: 3,
  firstAt: 1.2,
  interval: 4,
  warning: 0.6,
  shellRadius: 18,
  shellSpeed: 130,
  spawnFactor: 1.5,
};

const tiger = {
  stay: 12,
  attacks: 5,
  firstAt: 1.6,
  interval: 1.4,
  warning: 0.7,
  active: 0.3,
  bandWidth: 60,
  spawnFactor: 1.5,
};

const suzaku = {
  stay: 12,
  attacks: 4,
  firstAt: 2,
  interval: 2,
  warning: 0.8,
  sparkRadius: 7,
  sparkSpeed: 160,
  // 自機に向けた向きを中心に、5方向へまく。
  fanDegrees: [-36, -18, 0, 18, 36],
  featherRadius: 8,
  featherSpeed: 120,
  featherSway: 22,
  // 尾羽は帰りぎわ（stayの終わりからこの秒数前）に予告してから落とす。
  featherWarningAt: 2.6,
  spawnFactor: 2,
};

const dragon = {
  stay: 15,
  attacks: 6,
  firstAt: 1,
  interval: 1.6,
  warning: 1,
  active: 0.3,
  columns: 5,
  // 前半はこの回数だけ2列、残りは3列に落とす。
  twoColumnAttacks: 3,
  pearlWarning: 1,
  pearlRadius: 10,
  pearlSpeed: 90,
  pearlLife: 4,
  // 宝珠が向きを自機へ寄せる速さ（1秒あたりの割合）。小さいほどゆっくり追う。
  pearlTurn: 1.5,
  spawnFactor: 2,
};

// 自機2つ分。攻撃が残すすき間の下限。
export const minimumGap = 48;

const columnWidth = worldWidth / dragon.columns;

function randomBetween(min: number, max: number): number {
  return min + Math.random() * (max - min);
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

function easeOut(t: number): number {
  return 1 - (1 - t) ** 2;
}

function pick<T>(items: T[]): T {
  return items[Math.floor(Math.random() * items.length)];
}

// appearanceWindowは敵の出てくる時間帯を返す。
export function appearanceWindow(kind: EnemyKind): { from: number; to: number } {
  const found = appearances.find((appearance) => appearance.kind === kind);
  return found === undefined ? { from: 0, to: 0 } : { from: found.from, to: found.to };
}

// planAppearancesは1回の遊びで敵が現れる時刻を決める。帰るまでに時間帯を出ないよう、
// 時間帯の頭からstayと出入りの分を引いた範囲でばらつかせる。
export function planAppearances(): { kind: EnemyKind; at: number }[] {
  return appearances.map(({ kind, from, to }) => {
    const span = to - from - stayOf(kind) - travelDuration * 2;
    return { kind, at: from + randomBetween(0, Math.max(span, 0)) };
  });
}

function stayOf(kind: EnemyKind): number {
  return { turtle: turtle.stay, tiger: tiger.stay, suzaku: suzaku.stay, dragon: dragon.stay }[kind];
}

// tigerBandsは白虎の爪あとの中心を決める。1本目は自機のいる列、2本目は1本目との間に
// minimumGap以上のすき間が残るところ。
export function tigerBands(playerX: number, count: 1 | 2): number[] {
  const half = tiger.bandWidth / 2;
  const first = clamp(playerX, half, worldWidth - half);
  if (count === 1) {
    return [first];
  }
  const apart = tiger.bandWidth + minimumGap;
  const sides: number[] = [];
  if (first - apart >= half) {
    sides.push(randomBetween(half, first - apart));
  }
  if (first + apart <= worldWidth - half) {
    sides.push(randomBetween(first + apart, worldWidth - half));
  }
  return [first, pick(sides)];
}

// dragonColumnsは雷を落とす列を決める。自機のいる列には必ず落とし、自機から2列以内に
// 落とさない列を必ず1つ残す。
export function dragonColumns(playerX: number, count: number): number[] {
  const own = clamp(Math.floor(playerX / columnWidth), 0, dragon.columns - 1);
  const near: number[] = [];
  for (let column = own - 2; column <= own + 2; column += 1) {
    if (column !== own && column >= 0 && column < dragon.columns) {
      near.push(column);
    }
  }
  const safe = pick(near);
  const rest: number[] = [];
  for (let column = 0; column < dragon.columns; column += 1) {
    if (column !== own && column !== safe) {
      rest.push(column);
    }
  }
  const struck = [own];
  while (struck.length < count && rest.length > 0) {
    struck.push(rest.splice(Math.floor(Math.random() * rest.length), 1)[0]);
  }
  return struck.sort((a, b) => a - b);
}

// Enemyは画面にいる1体の敵。updateで時間を進め、hazardsで今当たるものを返す。
export class Enemy {
  readonly kind: EnemyKind;
  readonly stay: number;
  age = 0;
  x: number;
  y = -40;
  // facingは体の向き。朱雀は進む向き、ほかは右向き。
  facing: 1 | -1 = 1;
  bullets: Bullet[] = [];
  strikes: Strike[] = [];
  private cues: Cue[] = [];
  private events: Event[] = [];

  constructor(kind: EnemyKind) {
    this.kind = kind;
    this.stay = stayOf(kind);
    this.x = randomBetween(70, worldWidth - 70);

    switch (kind) {
      case "turtle":
        this.planTurtle();
        break;
      case "tiger":
        this.planTiger();
        break;
      case "suzaku":
        this.facing = Math.random() < 0.5 ? 1 : -1;
        this.planSuzaku();
        break;
      case "dragon":
        this.x = worldWidth / 2;
        this.planDragon();
        break;
    }
    this.events.sort((a, b) => a.at - b.at);
  }

  // spawnFactorは、この敵がいる間にふつうの障害物の間隔を何倍にするか。
  get spawnFactor(): number {
    return {
      turtle: turtle.spawnFactor,
      tiger: tiger.spawnFactor,
      suzaku: suzaku.spawnFactor,
      dragon: dragon.spawnFactor,
    }[this.kind];
  }

  // blocksObstaclesは、ふつうの障害物を出さずにおく間か。青龍の雷の予告と雷の間がそれにあたる。
  get blocksObstacles(): boolean {
    return this.kind === "dragon" && this.strikes.length > 0;
  }

  // presentは体が画面にいる間か。帰り始めてから画面の外に出るまでも含む。
  get present(): boolean {
    if (this.kind === "suzaku") {
      return this.age < this.stay;
    }
    return this.age < this.stay + travelDuration;
  }

  // finishedは体が帰り、弾と攻撃もすべて消えたか。
  get finished(): boolean {
    return !this.present && this.bullets.length === 0 && this.strikes.length === 0;
  }

  cue(kind: CueKind): boolean {
    return this.cues.some((cue) => cue.kind === kind && this.age >= cue.from && this.age < cue.to);
  }

  update(dt: number, player: Point): void {
    this.age += dt;
    this.move();

    while (this.events.length > 0 && this.events[0].at <= this.age) {
      this.events.shift()?.run(player);
    }

    for (const bullet of this.bullets) {
      this.moveBullet(bullet, dt, player);
    }
    this.bullets = this.bullets.filter((bullet) => this.keepsBullet(bullet));
    this.strikes = this.strikes.filter((strike) => this.age < strike.until);
  }

  hazards(): { circles: Circle[]; rects: Rect[] } {
    const circles: Circle[] = this.bullets.map(({ x, y, radius }) => ({ x, y, radius }));
    if (this.present && this.y > -bodyRadius) {
      circles.push({ x: this.x, y: this.y, radius: bodyRadius });
    }
    const rects = this.strikes
      .filter((strike) => this.age >= strike.activeFrom)
      .map(({ x, width }) => ({ x, y: 0, width, height: worldHeight }));
    return { circles, rects };
  }

  private move() {
    if (this.kind === "suzaku") {
      // 上をアーチ状に横切る。端から入って、反対の端へ抜ける。
      const progress = clamp(this.age / this.stay, 0, 1);
      const from = this.facing === 1 ? -28 : worldWidth + 28;
      const to = this.facing === 1 ? worldWidth + 28 : -28;
      this.x = from + (to - from) * progress;
      this.y = 66 - 34 * Math.sin(Math.PI * progress);
      return;
    }
    if (this.age < travelDuration) {
      this.y = -40 + (restY + 40) * easeOut(this.age / travelDuration);
    } else if (this.age < this.stay) {
      this.y = restY;
    } else {
      const progress = clamp((this.age - this.stay) / travelDuration, 0, 1);
      this.y = restY - (restY + 60) * progress * progress;
    }
  }

  private moveBullet(bullet: Bullet, dt: number, player: Point) {
    bullet.age += dt;

    if (bullet.kind === "pearl") {
      // ゆっくり向きを自機へ寄せながら、同じ速さで進む。
      const dx = player.x - bullet.x;
      const dy = player.y - bullet.y;
      const length = Math.hypot(dx, dy) || 1;
      const turn = Math.min(dragon.pearlTurn * dt, 1);
      const vx = bullet.vx + ((dx / length) * dragon.pearlSpeed - bullet.vx) * turn;
      const vy = bullet.vy + ((dy / length) * dragon.pearlSpeed - bullet.vy) * turn;
      const speed = Math.hypot(vx, vy) || 1;
      bullet.vx = (vx / speed) * dragon.pearlSpeed;
      bullet.vy = (vy / speed) * dragon.pearlSpeed;
    }

    if (bullet.kind === "feather") {
      bullet.y += bullet.vy * dt;
      bullet.x = bullet.swayFrom + Math.sin(bullet.age * 2.6) * suzaku.featherSway;
      return;
    }

    bullet.x += bullet.vx * dt;
    bullet.y += bullet.vy * dt;

    if (bullet.kind === "shell" && bullet.bounces < 1) {
      const hitsLeft = bullet.x - bullet.radius < 0 && bullet.vx < 0;
      const hitsRight = bullet.x + bullet.radius > worldWidth && bullet.vx > 0;
      if (hitsLeft || hitsRight) {
        bullet.vx = -bullet.vx;
        bullet.bounces += 1;
      }
    }
  }

  private keepsBullet(bullet: Bullet): boolean {
    if (bullet.kind === "pearl") {
      return bullet.age < dragon.pearlLife;
    }
    const margin = bullet.radius + 4;
    return (
      bullet.y < worldHeight + margin &&
      bullet.y > -margin * 4 &&
      bullet.x > -margin &&
      bullet.x < worldWidth + margin
    );
  }

  private shoot(kind: BulletKind, from: Point, angle: number, speed: number, radius: number) {
    this.bullets.push({
      kind,
      x: from.x,
      y: from.y,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed,
      radius,
      age: 0,
      bounces: 0,
      swayFrom: from.x,
    });
  }

  // 亀: 首を引っこめて予告し、甲羅を自機のあたりへ斜めにすべらせる。
  private planTurtle() {
    for (let index = 0; index < turtle.attacks; index += 1) {
      const at = turtle.firstAt + index * turtle.interval;
      this.cues.push({ kind: "retract", from: at, to: at + turtle.warning });
      this.events.push({
        at: at + turtle.warning,
        run: (player) => {
          const from = { x: this.x, y: this.y + 8 };
          // 真下に近い向きでは壁ではね返らないため、横にも必ず進むようにする。
          let angle = Math.atan2(player.y - from.y, player.x - from.x);
          const minimumTilt = (20 * Math.PI) / 180;
          const fromDown = angle - Math.PI / 2;
          if (Math.abs(fromDown) < minimumTilt) {
            angle = Math.PI / 2 + (fromDown < 0 ? -minimumTilt : minimumTilt);
          }
          this.shoot("shell", from, angle, turtle.shellSpeed, turtle.shellRadius);
        },
      });
    }
  }

  // 白虎: 自機のいる列に点線の爪あとを出し、少しあとで3本の爪が上から下まで走る。
  // 最後の1回は2か所同時。
  private planTiger() {
    for (let index = 0; index < tiger.attacks; index += 1) {
      const at = tiger.firstAt + index * tiger.interval;
      const last = index === tiger.attacks - 1;
      this.events.push({
        at,
        run: (player) => {
          for (const center of tigerBands(player.x, last ? 2 : 1)) {
            this.strikes.push({
              kind: "claw",
              x: center - tiger.bandWidth / 2,
              width: tiger.bandWidth,
              warnFrom: this.age,
              activeFrom: this.age + tiger.warning,
              until: this.age + tiger.warning + tiger.active,
            });
          }
        },
      });
    }
  }

  // 朱雀: 上を横切りながら、羽ばたいて光ってから自機に向けて火の粉を5方向にまく。
  // 帰りぎわに尾羽が3枚、揺れながら落ちる。
  private planSuzaku() {
    for (let index = 0; index < suzaku.attacks; index += 1) {
      const at = suzaku.firstAt + index * suzaku.interval;
      this.cues.push({ kind: "glow", from: at, to: at + suzaku.warning });
      this.events.push({
        at: at + suzaku.warning,
        run: (player) => {
          const from = { x: this.x, y: this.y };
          const aim = Math.atan2(player.y - from.y, player.x - from.x);
          for (const degrees of suzaku.fanDegrees) {
            const angle = aim + (degrees * Math.PI) / 180;
            this.shoot("spark", from, angle, suzaku.sparkSpeed, suzaku.sparkRadius);
          }
        },
      });
    }

    const featherAt = this.stay - suzaku.featherWarningAt;
    this.cues.push({ kind: "glow", from: featherAt, to: featherAt + suzaku.warning });
    this.events.push({
      at: featherAt + suzaku.warning,
      run: () => {
        for (const offset of [-44, 0, 44]) {
          const x = clamp(this.x + offset, 24, worldWidth - 24);
          this.shoot(
            "feather",
            { x, y: this.y },
            Math.PI / 2,
            suzaku.featherSpeed,
            suzaku.featherRadius,
          );
        }
      },
    });
  }

  // 青龍: 画面を5列に分け、雲の出た列に雷が落ちる。前半は2列、後半は3列。
  // しめくくりに、宝珠が自機をゆっくり追いかけて消える。
  private planDragon() {
    for (let index = 0; index < dragon.attacks; index += 1) {
      const at = dragon.firstAt + index * dragon.interval;
      const count = index < dragon.twoColumnAttacks ? 2 : 3;
      this.events.push({
        at,
        run: (player) => {
          for (const column of dragonColumns(player.x, count)) {
            this.strikes.push({
              kind: "thunder",
              x: column * columnWidth,
              width: columnWidth,
              warnFrom: this.age,
              activeFrom: this.age + dragon.warning,
              until: this.age + dragon.warning + dragon.active,
            });
          }
        },
      });
    }

    const lastStrikeEnds =
      dragon.firstAt + (dragon.attacks - 1) * dragon.interval + dragon.warning + dragon.active;
    this.cues.push({
      kind: "glow",
      from: lastStrikeEnds,
      to: lastStrikeEnds + dragon.pearlWarning,
    });
    this.events.push({
      at: lastStrikeEnds + dragon.pearlWarning,
      run: (player) => {
        const from = { x: this.x, y: this.y + 18 };
        const angle = Math.atan2(player.y - from.y, player.x - from.x);
        this.shoot("pearl", from, angle, dragon.pearlSpeed, dragon.pearlRadius);
      },
    });
  }
}

// strikeProgressは予告が始まってから攻撃が当たるまでのうち、どこまで進んだかを0〜1で返す。
export function strikeProgress(strike: Strike, age: number): number {
  return clamp((age - strike.warnFrom) / (strike.activeFrom - strike.warnFrom), 0, 1);
}

// pearlFadeは宝珠の消えぎわの濃さを返す。最後の0.5秒で薄くなる。
export function pearlFade(bullet: Bullet): number {
  return clamp((dragon.pearlLife - bullet.age) / 0.5, 0, 1);
}
