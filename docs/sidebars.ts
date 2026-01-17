import type { SidebarsConfig } from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docs: [
    {
      type: 'category',
      label: 'Introduction',
      items: ['introduction'],
    },
    {
      type: 'category',
      label: 'Getting Started',
      items: ['quick-start', 'demo'],
    },
    {
      type: 'category',
      label: 'Concepts',
      items: ['architecture', 'replication', 'snapshots'],
    },
    {
      type: 'category',
      label: 'Deployment',
      items: ['configuration-reference', 'nats-configuration', 'production-deployment'],
    },
    {
      type: 'category',
      label: 'Operations',
      items: ['troubleshooting', 'health-check', 'faq'],
    },
    {
      type: 'category',
      label: 'Design Proposals',
      items: ['design/schema-versioning'],
    },
  ],
};

export default sidebars;