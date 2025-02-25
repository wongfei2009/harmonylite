/** @type {import('next').NextConfig} */
const withNextra = require("nextra")({
  theme: "nextra-theme-docs",
  themeConfig: "./theme.config.js"
});

const nextConfig = {
  // Your other Next.js configuration
  reactStrictMode: true,
  basePath: "/harmonylite",
  
  // Disable minification to resolve Terser errors
  swcMinify: false,
  
  // Add custom webpack configuration
  webpack: (config, { dev, isServer }) => {
    // Disable Terser minification
    if (!dev && !isServer) {
      config.optimization.minimize = false;
    }
    
    return config;
  },
};

module.exports = withNextra(nextConfig);