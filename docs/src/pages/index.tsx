// src/pages/index.tsx
import React from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import styles from './index.module.css';
import HomepageFeatures from '@site/src/components/HomepageFeatures';

export default function Home() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={`${siteConfig.title}`}
      description="A distributed SQLite replicator with leaderless architecture">
      <header className={styles.heroBanner}>
        <div className="container">
          <h1 className="hero__title">{siteConfig.title}</h1>
          <p className="hero__subtitle">{siteConfig.tagline}</p>
          <div className={styles.buttons}>
            <Link
              className="button button--primary button--lg"
              to="https://github.com/wongfei2009/harmonylite/releases/latest">
              Download Latest
            </Link>
            <Link
              className="button button--secondary button--lg"
              to="/docs/introduction">
              Read the Docs
            </Link>
            <Link
              className="button button--secondary button--lg"
              to="/docs/demo">
              See it in Action
            </Link>
          </div>
        </div>
      </header>
    </Layout>
  );
}