/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Produces a minimal standalone server bundle for the Docker runtime stage.
  output: "standalone",
};

export default nextConfig;
