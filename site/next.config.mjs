import path from "node:path";
import { fileURLToPath } from "node:url";

const dirname = path.dirname(fileURLToPath(import.meta.url));

const nextConfig = {
  output: "export",
  images: {
    unoptimized: true
  },
  turbopack: {
    root: dirname
  }
};

export default nextConfig;
