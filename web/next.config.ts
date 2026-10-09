import type { NextConfig } from "next";

// Docs pages use time-based revalidation (`export const revalidate`): they
// change only when a maintainer uploads new game data.
const nextConfig: NextConfig = {
  cacheComponents: false,
};

export default nextConfig;
