/* eslint-disable @typescript-eslint/no-var-requires */
/**
 * @type {import('next').NextConfig}
 */
const nextConfig = {
  reactStrictMode: true,
  basePath: "/harmonylite",
};

const withNextra = require("nextra")({
  theme: "nextra-theme-docs",
  themeConfig: "./theme.config.js",
  mdxOptions: {
    rehypePlugins: [
      require('rehype-mermaid')
    ]
  }
});
module.exports = withNextra(nextConfig);
