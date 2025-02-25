import "../styles/globals.css";
import { useEffect } from "react";

export default function Nextra({ Component, pageProps }) {
  const getLayout = Component.getLayout || ((page) => page);
  
  return getLayout(<Component {...pageProps} />);
}