/** @type {import('next').NextConfig} */
const withNextra = require("nextra")({
  theme: "nextra-theme-docs",
  themeConfig: "./theme.config.js",
  mdxOptions: {
    rehypePlugins: [
      // Use an async function that dynamically imports rehype-mermaid
      async () => {
        const rehypeMermaid = await import('rehype-mermaid');
        return rehypeMermaid.default;
      }
    ]
  }
});

const nextConfig = {
  // Your other Next.js configuration
  reactStrictMode: true,
  basePath: "/harmonylite",
};

module.exports = withNextra(nextConfig);