import React, { useEffect, useRef } from 'react';

export default function Mermaid({ chart }) {
  const ref = useRef(null);

  useEffect(() => {
    if (ref.current && typeof window !== 'undefined') {
      import('mermaid').then(({ default: mermaid }) => {
        mermaid.initialize({
          startOnLoad: false,
          theme: 'neutral',
          securityLevel: 'loose'
        });
        mermaid.contentLoaded();
        mermaid.init(undefined, ref.current);
      });
    }
  }, [chart]);

  return (
    <div className="mermaid" ref={ref}>
      {chart}
    </div>
  );
}