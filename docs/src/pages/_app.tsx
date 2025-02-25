import "../styles/globals.css";
import { useEffect } from "react";

export default function Nextra({ Component, pageProps }) {
  const getLayout = Component.getLayout || ((page) => page);
  
  useEffect(() => {
    // Initialize mermaid when the component mounts
    if (typeof window !== "undefined") {
      import("mermaid").then(({ default: mermaid }) => {
        mermaid.initialize({
          startOnLoad: true,
          theme: "neutral",
          securityLevel: "loose"
        });
        mermaid.contentLoaded();
      });
    }
  }, []);
  
  return getLayout(<Component {...pageProps} />);
}