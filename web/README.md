# web

匿名トピックチャットサービスのフロントエンド。Next.js（App Router / TypeScript / Tailwind CSS）。

起動・環境変数・経路ごとの描画方法は、リポジトリ直下の [README.md](../README.md) にまとめてある。
`make up` で API と一緒に立ち上がるため、通常はこのディレクトリで直接コマンドを叩く必要はない。

単体で動かす場合は、API のオリジンを渡して dev サーバーを起動する。

```bash
npm install
API_BASE_URL=http://localhost:8080 npm run dev
```

| コマンド | 内容 |
| --- | --- |
| `npm run dev` | 開発サーバー |
| `npm run build` | 本番ビルド |
| `npm run lint` | ESLint |
| `npm run typecheck` | `next typegen` と `tsc --noEmit` |
| `npm run format` | Prettier |
