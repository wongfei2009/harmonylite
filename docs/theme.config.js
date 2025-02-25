const basePath = "/harmonylite";

/**
 * @type {import("nextra-theme-docs").DocsThemeConfig}
 */
export default {
  project: { link: "https://github.com/wongfei2009/harmonylite" },
  docsRepositoryBase: "https://github.com/wongfei2009/harmonylite/tree/master/docs",
  titleSuffix: " - HarmonyLite",
  logo: (
    <>
      <img width={50} style={{ marginRight: 5 }} src={`${basePath}/logo.png`} />
      <span className="text-gray-600 font-normal hidden md:inline">
        A distributed SQLite replicator built on top of NATS
      </span>
    </>
  ),
  head: (
    <>
      <meta name="msapplication-TileColor" content="#ffffff" />
      <meta name="theme-color" content="#ffffff" />
      <meta name="viewport" content="width=device-width, initial-scale=1.0" />
      <meta httpEquiv="Content-Language" content="en" />
      <meta
        name="description"
        content="HarmonyLite: A distributed SQLite replicator built on top of NATS"
      />
      <meta
        name="og:description"
        content="HarmonyLite: A distributed SQLite replicator built on top of NATS"
      />
      <meta name="twitter:card" content="summary_large_image" />
      <meta name="twitter:image" content={`${basePath}/logo.png`} />
      <meta name="twitter:site:domain" content="https://github.com/wongfei2009/harmonylite" />
      <meta name="twitter:url" content="https://github.com/wongfei2009/harmonylite" />
      <meta
        name="og:title"
        content="HarmonyLite: A distributed SQLite replicator built on top of NATS"
      />
      <meta name="og:image" content={`${basePath}/logo.png`} />
      <meta name="apple-mobile-web-app-title" content="HarmonyLite" />
      <link rel="apple-touch-icon" sizes="180x180" href={`${basePath}/logo.png`} />
      <link rel="icon" type="image/png" sizes="192x192" href={`${basePath}/logo.png`} />
      <link rel="icon" type="image/png" sizes="32x32" href={`${basePath}/logo.png`} />
      <link rel="icon" type="image/png" sizes="96x96" href={`${basePath}/logo.png`} />
      <link rel="icon" type="image/png" sizes="16x16" href={`${basePath}/logo.png`} />
      <meta name="msapplication-TileImage" content="/ms-icon-144x144.png" />
    </>
  ),
  navigation: true,
  footer: { text: <>MIT {new Date().getFullYear()} © HarmonyLite.</> },
  editLink: { text: "Edit this page on GitHub" },
  unstable_faviconGlyph: "👋",
};
