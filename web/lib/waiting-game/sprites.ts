// 待機ゲームの四神と、その攻撃の描き方。色はキャラクターのデザインにある配色を使う。
import {
  type Bullet,
  type Enemy,
  type Strike,
  pearlFade,
  strikeProgress,
  worldHeight,
} from "@/lib/waiting-game/enemies";

type Context = CanvasRenderingContext2D;

const ink = "#3b2f36";
const cheek = "rgba(255, 150, 170, 0.55)";

const colors = {
  turtle: { body: "#8fd3b6", light: "#b5e5d0", edge: "#4e9a7c" },
  tiger: { body: "#ffffff", stripe: "#b5a6d6", edge: "#9d8cc8", pink: "#ffc1cf" },
  suzaku: {
    body: "#f89d86",
    belly: "#ffe3d6",
    crest: "#ffd36b",
    crestEdge: "#e0a93a",
    edge: "#e06d5a",
  },
  dragon: { body: "#9ccaf5", light: "#cfe6fb", edge: "#5d93cc", horn: "#f5cd4a", belly: "#fff3c4" },
  thunder: { bolt: "#f5cd4a", boltEdge: "#e0b030", glow: "rgba(255, 236, 160, 0.55)" },
};

// labelColorsは「〜があらわれた！」の文字の色。
export const labelColors = {
  turtle: colors.turtle.edge,
  tiger: colors.tiger.edge,
  suzaku: colors.suzaku.edge,
  dragon: colors.dragon.edge,
};

function circle(context: Context, x: number, y: number, radius: number) {
  context.beginPath();
  context.arc(x, y, radius, 0, Math.PI * 2);
}

function ellipse(context: Context, x: number, y: number, rx: number, ry: number, rotation = 0) {
  context.beginPath();
  context.ellipse(x, y, rx, ry, rotation, 0, Math.PI * 2);
}

function fillStroke(context: Context, fill: string, stroke: string, width = 2) {
  context.fillStyle = fill;
  context.fill();
  context.strokeStyle = stroke;
  context.lineWidth = width;
  context.stroke();
}

function eyes(context: Context, x: number, y: number, spread: number) {
  context.fillStyle = ink;
  circle(context, x - spread, y, 1.8);
  context.fill();
  circle(context, x + spread, y, 1.8);
  context.fill();
}

function cheeks(context: Context, x: number, y: number, spread: number) {
  context.fillStyle = cheek;
  ellipse(context, x - spread, y, 2.6, 1.6);
  context.fill();
  ellipse(context, x + spread, y, 2.6, 1.6);
  context.fill();
}

function hexagon(context: Context, x: number, y: number, radius: number, rotation: number) {
  context.beginPath();
  for (let index = 0; index < 6; index += 1) {
    const angle = rotation + (index * Math.PI) / 3;
    const px = x + Math.cos(angle) * radius;
    const py = y + Math.sin(angle) * radius;
    if (index === 0) {
      context.moveTo(px, py);
    } else {
      context.lineTo(px, py);
    }
  }
  context.closePath();
}

// glowは予告のときに体の後ろに出す光。時間で明るさが揺れる。
function glow(context: Context, x: number, y: number, radius: number, color: string, age: number) {
  const pulse = 0.55 + 0.45 * Math.sin(age * 18);
  const gradient = context.createRadialGradient(x, y, radius * 0.3, x, y, radius);
  gradient.addColorStop(0, color);
  gradient.addColorStop(1, "rgba(255, 255, 255, 0)");
  context.save();
  context.globalAlpha = pulse;
  context.fillStyle = gradient;
  circle(context, x, y, radius);
  context.fill();
  context.restore();
}

function drawTurtle(context: Context, enemy: Enemy) {
  const { x, y } = enemy;
  const retracted = enemy.cue("retract");
  // 引っこめている間は、甲羅が小さく震える。
  const shake = retracted ? Math.sin(enemy.age * 50) * 1.2 : 0;
  const { body, light, edge } = colors.turtle;

  context.save();
  context.translate(x + shake, y);

  // 足
  for (const footX of [-11, 9]) {
    ellipse(context, footX, 8, 5, 3.5);
    fillStroke(context, light, edge, 1.5);
  }
  // 頭
  if (!retracted) {
    circle(context, 20, -4, 7.5);
    fillStroke(context, light, edge);
    eyes(context, 21, -5, 2.6);
    cheeks(context, 21, -1.5, 4.2);
  } else {
    circle(context, 15, 0, 4);
    fillStroke(context, light, edge, 1.5);
  }
  // 甲羅
  context.beginPath();
  context.ellipse(0, 3, 19, 15, 0, Math.PI, 0);
  context.closePath();
  fillStroke(context, body, edge);
  hexagon(context, -1, -4, 5.5, Math.PI / 6);
  context.fillStyle = light;
  context.fill();
  context.beginPath();
  context.roundRect(-21, 2, 42, 6, 3);
  fillStroke(context, light, edge, 1.5);

  context.restore();
}

function drawShell(context: Context, bullet: Bullet) {
  const { body, light, edge } = colors.turtle;
  circle(context, bullet.x, bullet.y, bullet.radius);
  fillStroke(context, body, edge);
  hexagon(context, bullet.x, bullet.y, bullet.radius * 0.5, bullet.age * 7);
  context.fillStyle = light;
  context.fill();
  // 回っていることがわかる弧
  context.strokeStyle = edge;
  context.lineWidth = 1.5;
  for (const start of [bullet.age * 7, bullet.age * 7 + Math.PI]) {
    context.beginPath();
    context.arc(bullet.x, bullet.y, bullet.radius + 4, start, start + 0.7);
    context.stroke();
  }
}

function drawTiger(context: Context, enemy: Enemy) {
  const { x, y } = enemy;
  const { body, stripe, edge, pink } = colors.tiger;

  context.save();
  context.translate(x, y);

  // しっぽと体
  context.strokeStyle = edge;
  context.lineWidth = 3;
  context.beginPath();
  context.moveTo(9, 16);
  context.quadraticCurveTo(22, 18, 19, 6);
  context.stroke();
  ellipse(context, 0, 14, 11, 8);
  fillStroke(context, body, edge);
  // 耳
  for (const side of [-1, 1]) {
    circle(context, side * 10, -11, 5);
    fillStroke(context, body, edge);
    circle(context, side * 10, -11, 2.4);
    context.fillStyle = pink;
    context.fill();
  }
  // 顔
  circle(context, 0, 0, 14);
  fillStroke(context, body, edge);
  context.strokeStyle = stripe;
  context.lineWidth = 2;
  for (const stripeX of [-4, 0, 4]) {
    context.beginPath();
    context.moveTo(stripeX, -12);
    context.lineTo(stripeX, -7);
    context.stroke();
  }
  for (const side of [-1, 1]) {
    context.beginPath();
    context.moveTo(side * 14, 1);
    context.lineTo(side * 9, 2);
    context.stroke();
  }
  eyes(context, 0, -1, 5);
  cheeks(context, 0, 4, 8);
  context.strokeStyle = ink;
  context.lineWidth = 1.2;
  context.beginPath();
  context.arc(-1.5, 4, 1.5, 0, Math.PI);
  context.arc(1.5, 4, 1.5, 0, Math.PI);
  context.stroke();

  context.restore();
}

function drawClaw(context: Context, strike: Strike, age: number) {
  const { stripe } = colors.tiger;
  const lanes = [0.25, 0.5, 0.75].map((share) => strike.x + strike.width * share);

  context.save();
  context.lineCap = "round";
  if (age < strike.activeFrom) {
    // 予告: 点線の爪あと。当たる時刻が近づくほど濃くなる。
    const progress = strikeProgress(strike, age);
    context.globalAlpha = 0.35 + 0.55 * progress;
    context.fillStyle = "rgba(181, 166, 214, 0.12)";
    context.fillRect(strike.x, 0, strike.width, worldHeight);
    context.strokeStyle = stripe;
    context.lineWidth = 5;
    context.setLineDash([6, 8]);
    for (const lane of lanes) {
      context.beginPath();
      context.moveTo(lane, 6);
      context.lineTo(lane, worldHeight - 6);
      context.stroke();
    }
  } else {
    // 爪が上から下まで走る。
    const reach = Math.min((age - strike.activeFrom) / 0.1, 1) * worldHeight;
    context.fillStyle = "rgba(181, 166, 214, 0.22)";
    context.fillRect(strike.x, 0, strike.width, worldHeight);
    context.strokeStyle = stripe;
    context.lineWidth = 8;
    for (const lane of lanes) {
      context.beginPath();
      context.moveTo(lane, 0);
      context.lineTo(lane, reach);
      context.stroke();
    }
  }
  context.restore();
}

function drawSuzaku(context: Context, enemy: Enemy) {
  const { x, y, facing } = enemy;
  const { body, belly, crest, crestEdge, edge } = colors.suzaku;
  const glowing = enemy.cue("glow");
  if (glowing) {
    glow(context, x, y, 30, "rgba(255, 211, 107, 0.9)", enemy.age);
  }
  // 羽ばたき。予告の間は速く羽ばたく。
  const flap = Math.sin(enemy.age * (glowing ? 30 : 9)) * 0.35;

  context.save();
  context.translate(x, y);
  context.scale(facing, 1);

  // 尾羽
  for (const [angle, color] of [
    [2.6, crest],
    [2.95, body],
    [3.3, crest],
  ] as const) {
    ellipse(context, Math.cos(angle) * 16, Math.sin(angle) * 16 - 2, 9, 3.5, angle);
    fillStroke(context, color, edge, 1.5);
  }
  // 体
  circle(context, 0, 0, 14);
  fillStroke(context, body, edge);
  ellipse(context, 1, 5, 8, 6.5);
  context.fillStyle = belly;
  context.fill();
  // とさか
  for (const offset of [-4, 0, 4]) {
    ellipse(context, offset, -15, 2.5, 5, offset * 0.08);
    fillStroke(context, crest, crestEdge, 1.2);
  }
  // 羽
  ellipse(context, -5, 2, 6, 9, -0.4 + flap);
  fillStroke(context, body, edge, 1.5);
  // くちばしと顔
  context.beginPath();
  context.moveTo(13, -1);
  context.lineTo(19, 1);
  context.lineTo(13, 3);
  context.closePath();
  fillStroke(context, crest, crestEdge, 1.2);
  eyes(context, 6, -4, 3);
  cheeks(context, 6, 0, 5.5);

  context.restore();
}

function drawSpark(context: Context, bullet: Bullet) {
  const { crest, edge } = colors.suzaku;
  circle(context, bullet.x, bullet.y, bullet.radius);
  fillStroke(context, crest, edge);
}

function drawFeather(context: Context, bullet: Bullet) {
  const { body, edge } = colors.suzaku;
  const tilt = Math.cos(bullet.age * 2.6) * 0.6;
  ellipse(context, bullet.x, bullet.y, 4.5, bullet.radius + 3, tilt);
  fillStroke(context, body, edge, 1.5);
  context.strokeStyle = edge;
  context.lineWidth = 1;
  context.beginPath();
  context.moveTo(bullet.x - Math.sin(tilt) * 9, bullet.y + Math.cos(tilt) * 9);
  context.lineTo(bullet.x + Math.sin(tilt) * 9, bullet.y - Math.cos(tilt) * 9);
  context.stroke();
}

function cloud(context: Context, x: number, y: number, scale: number) {
  context.save();
  context.translate(x, y);
  context.scale(scale, scale);
  for (const [cx, cy, radius] of [
    [-9, 2, 7],
    [0, -2, 9],
    [10, 2, 7],
  ] as const) {
    circle(context, cx, cy, radius);
    fillStroke(context, "#ffffff", colors.dragon.light, 2);
  }
  context.fillStyle = "#ffffff";
  context.fillRect(-14, 1, 28, 7);
  context.restore();
}

function drawDragon(context: Context, enemy: Enemy) {
  const { x, y } = enemy;
  const { body, light, edge, horn, belly } = colors.dragon;
  const glowing = enemy.cue("glow");

  context.save();
  context.translate(x, y);

  // しっぽと体
  context.strokeStyle = edge;
  context.lineWidth = 7;
  context.lineCap = "round";
  context.beginPath();
  context.moveTo(8, 14);
  context.quadraticCurveTo(24, 14, 21, 0);
  context.stroke();
  context.strokeStyle = body;
  context.lineWidth = 4;
  context.stroke();
  ellipse(context, 0, 13, 10, 9);
  fillStroke(context, body, edge);
  ellipse(context, 0, 15, 5.5, 5);
  context.fillStyle = belly;
  context.fill();
  // 角
  context.strokeStyle = horn;
  context.lineWidth = 2.5;
  for (const side of [-1, 1]) {
    context.beginPath();
    context.moveTo(side * 6, -11);
    context.lineTo(side * 9, -19);
    context.moveTo(side * 8, -16);
    context.lineTo(side * 12, -17);
    context.stroke();
  }
  // 頭
  circle(context, 0, -2, 12);
  fillStroke(context, body, edge);
  ellipse(context, 0, 3, 7, 4.5);
  fillStroke(context, light, edge, 1.5);
  context.fillStyle = edge;
  circle(context, -2.2, 3, 0.9);
  context.fill();
  circle(context, 2.2, 3, 0.9);
  context.fill();
  eyes(context, 0, -5, 4.5);
  cheeks(context, 0, -1, 9);

  context.restore();

  cloud(context, x, y + 23, 1.1);

  if (glowing) {
    // 宝珠を出す前に、抱えた宝珠が光る。
    glow(context, x, y + 18, 18, "rgba(207, 230, 251, 1)", enemy.age);
    circle(context, x, y + 18, 6);
    fillStroke(context, "#ffffff", body, 1.5);
  }
}

function drawThunder(context: Context, strike: Strike, age: number) {
  const { bolt, boltEdge, glow: glowColor } = colors.thunder;

  context.save();
  if (age < strike.activeFrom) {
    // 予告: 列の上に雲が出て、列がだんだん光る。
    const progress = strikeProgress(strike, age);
    context.globalAlpha = 0.3 + 0.6 * progress;
    context.fillStyle = glowColor;
    context.fillRect(strike.x + 2, 0, strike.width - 4, worldHeight);
    context.strokeStyle = bolt;
    context.lineWidth = 1.5;
    context.setLineDash([6, 6]);
    context.strokeRect(strike.x + 2, 0, strike.width - 4, worldHeight);
    context.restore();
    cloud(context, strike.x + strike.width / 2, 14, 1);
    return;
  }

  context.fillStyle = glowColor;
  context.fillRect(strike.x, 0, strike.width, worldHeight);
  const center = strike.x + strike.width / 2;
  const points: [number, number][] = [];
  for (let step = 0; step <= 8; step += 1) {
    const offset = step === 0 || step === 8 ? 0 : step % 2 === 0 ? -12 : 12;
    points.push([center + offset, (worldHeight / 8) * step]);
  }
  context.lineJoin = "round";
  for (const [color, width] of [
    [boltEdge, 10],
    [bolt, 6],
  ] as const) {
    context.strokeStyle = color;
    context.lineWidth = width;
    context.beginPath();
    points.forEach(([px, py], index) =>
      index === 0 ? context.moveTo(px, py) : context.lineTo(px, py),
    );
    context.stroke();
  }
  context.restore();
  cloud(context, center, 14, 1);
}

function drawPearl(context: Context, bullet: Bullet) {
  const { light, body } = colors.dragon;
  context.save();
  context.globalAlpha = pearlFade(bullet);
  const gradient = context.createRadialGradient(
    bullet.x - 3,
    bullet.y - 3,
    1,
    bullet.x,
    bullet.y,
    bullet.radius,
  );
  gradient.addColorStop(0, "#ffffff");
  gradient.addColorStop(1, light);
  circle(context, bullet.x, bullet.y, bullet.radius);
  context.fillStyle = gradient;
  context.fill();
  context.strokeStyle = body;
  context.lineWidth = 2;
  context.stroke();
  context.restore();
}

// drawStrikesは画面を縦に貫く攻撃を描く。障害物より下に描き、予告が障害物を隠さないようにする。
export function drawStrikes(context: Context, enemy: Enemy) {
  for (const strike of enemy.strikes) {
    if (strike.kind === "claw") {
      drawClaw(context, strike, enemy.age);
    } else {
      drawThunder(context, strike, enemy.age);
    }
  }
}

// drawEnemyは敵の体と、敵が出した弾を描く。
export function drawEnemy(context: Context, enemy: Enemy) {
  if (enemy.present) {
    switch (enemy.kind) {
      case "turtle":
        drawTurtle(context, enemy);
        break;
      case "tiger":
        drawTiger(context, enemy);
        break;
      case "suzaku":
        drawSuzaku(context, enemy);
        break;
      case "dragon":
        drawDragon(context, enemy);
        break;
    }
  }

  for (const bullet of enemy.bullets) {
    switch (bullet.kind) {
      case "shell":
        drawShell(context, bullet);
        break;
      case "spark":
        drawSpark(context, bullet);
        break;
      case "feather":
        drawFeather(context, bullet);
        break;
      case "pearl":
        drawPearl(context, bullet);
        break;
    }
  }
}
