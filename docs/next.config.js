const withNextra = require("nextra")({
  theme: "nextra-theme-docs",
  themeConfig: "./theme.config.js",
  mdxOptions: {
    rehypePlugins: [
      // Remove the dynamic import which is causing problems
      require('rehype-mermaid')
    ]
  }
});

const nextConfig = {
  reactStrictMode: true,
  basePath: "/harmonylite",
};

module.exports = withNextra(nextConfig);