import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // 本番は docker/web.release.Dockerfile で、動かすのに要るファイルだけをイメージに入れる。
  output: "standalone",
};

export default nextConfig;
