'use client';

import React, { useEffect, useRef } from 'react';
import dynamic from 'next/dynamic';

// Use dynamic import with no SSR for the component
const MermaidComponent = ({ chart }) => {
  const ref = useRef(null);

  useEffect(() => {
    if (ref.current && typeof window !== 'undefined') {
      // Dynamic import of mermaid to ensure it only loads in browser
      import('mermaid').then(({ default: mermaid }) => {
        try {
          // Reset any previous renderings
          if (ref.current) {
            ref.current.innerHTML = chart;
          }
          
          mermaid.initialize({
            startOnLoad: false,
            theme: 'neutral',
            securityLevel: 'loose'
          });
          
          mermaid.run({
            nodes: [ref.current]
          }).catch(error => {
            console.error('Mermaid render error:', error);
            if (ref.current) {
              ref.current.innerHTML = `<pre>Error rendering Mermaid diagram: ${error.message}</pre>`;
            }
          });
        } catch (err) {
          console.error('Mermaid initialization error:', err);
          if (ref.current) {
            ref.current.innerHTML = `<pre>Error initializing Mermaid: ${err.message}</pre>`;
          }
        }
      });
    }
  }, [chart]);

  return (
    <div className="mermaid" ref={ref}>
      {chart}
    </div>
  );
};

// Create a dynamic version with no SSR
const Mermaid = dynamic(() => Promise.resolve(MermaidComponent), {
  ssr: false
});

export default Mermaid;