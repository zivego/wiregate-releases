import { useState } from "react";
import { uiTheme } from "./uiTheme";

const BRAND_PNG = "/branding/wiregate-logo.png";
const BRAND_FALLBACK_SVG = "/branding/wiregate-logo.svg";

interface BrandMarkProps {
  size: number | string;
}

function BrandMark({ size }: BrandMarkProps) {
  const [src, setSrc] = useState(BRAND_PNG);
  const [errored, setErrored] = useState(false);

  return (
    <img
      src={src}
      onError={() => {
        if (src !== BRAND_FALLBACK_SVG) {
          setSrc(BRAND_FALLBACK_SVG);
          return;
        }
        setErrored(true);
      }}
      alt=""
      aria-hidden="true"
      style={{
        display: errored ? "none" : "block",
        width: size,
        height: size,
        objectFit: "contain",
        flexShrink: 0,
        borderRadius: 6,
      }}
    />
  );
}

export function HeaderBrand() {
  return (
    <span style={s.headerWrap}>
      <BrandMark size="clamp(22px, 2.3vw, 28px)" />
      <span style={s.headerText}>wiregate</span>
    </span>
  );
}

export function LoginBrand() {
  return (
    <div style={s.loginWrap}>
      <BrandMark size="clamp(92px, 18vw, 132px)" />
      <h1 style={s.loginTitle}>wiregate</h1>
    </div>
  );
}

const s: Record<string, React.CSSProperties> = {
  headerWrap: {
    display: "inline-flex",
    alignItems: "center",
    gap: "0.55rem",
    minWidth: 0,
  },
  headerText: {
    fontSize: "1.2rem",
    fontWeight: 700,
    letterSpacing: 1,
    color: uiTheme.headerText,
    lineHeight: 1,
    whiteSpace: "nowrap",
  },
  loginWrap: {
    display: "grid",
    justifyItems: "center",
    gap: "0.65rem",
    marginBottom: "0.2rem",
  },
  loginTitle: {
    margin: 0,
    fontSize: "clamp(1.5rem, 3.6vw, 1.85rem)",
    fontWeight: 700,
    color: uiTheme.text,
    lineHeight: 1.1,
  },
};
