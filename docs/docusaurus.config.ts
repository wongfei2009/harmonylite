import { themes } from 'prism-react-renderer';
import type { Config } from '@docusaurus/types';

const config: Config = {
  title: 'HarmonyLite',
  tagline: 'A distributed SQLite replicator',
  url: 'https://wongfei2009.github.io',
  baseUrl: '/harmonylite/',
  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',
  favicon: 'img/favicon.ico',

  organizationName: 'wongfei2009',
  projectName: 'harmonylite',
  deploymentBranch: 'gh-pages',

  markdown: {
    mermaid: true,
  },
  
  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/wongfei2009/harmonylite/tree/master/harmonylite-docusaurus/docs/',
        },
        blog: false, // Disable blog if not needed
        theme: {
          customCss: './src/css/custom.css',
        },
      },
    ],
  ],

  themes: [
    '@docusaurus/theme-mermaid'
  ],

  themeConfig: {
    navbar: {
      title: 'HarmonyLite',
      logo: {
        alt: 'HarmonyLite Logo',
        src: 'img/logo.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Documentation',
        },
        {
          href: 'https://github.com/wongfei2009/harmonylite',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      copyright: `Copyright © ${new Date().getFullYear()} HarmonyLite.`,
    },
    prism: {
      theme: themes.github,
      darkTheme: themes.dracula,
    },
    mermaid: {
      theme: { light: 'neutral', dark: 'forest' },
    },
  },
};

export default config;