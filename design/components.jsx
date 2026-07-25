/* global React */
const { useState, useEffect, useRef, useMemo } = React;

// Status dot
function StatusDot({ status }) {
  return <span className={`dot ${status}`} title={status} />;
}

// Tag
function Tag({ kind, children }) {
  return <span className={`tag ${kind || ""}`}>{children}</span>;
}

// Mini sparkline
function Spark({ data, color = "var(--accent)", width = 120, height = 28, fill = true }) {
  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = max - min || 1;
  const stepX = width / (data.length - 1);
  const pts = data.map((v, i) => `${(i * stepX).toFixed(1)},${(height - ((v - min) / range) * (height - 2) - 1).toFixed(1)}`);
  const path = "M" + pts.join(" L");
  const fillPath = path + ` L${width},${height} L0,${height} Z`;
  return (
    <svg width={width} height={height} style={{ display: "block" }}>
      {fill && <path d={fillPath} fill={color} fillOpacity="0.12" />}
      <path d={path} fill="none" stroke={color} strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}

// Heat strip - coverage visualization
function HeatStrip({ coverage = 0.8, segments = 60, accent = "var(--accent-2)" }) {
  const cells = [];
  for (let i = 0; i < segments; i++) {
    const t = i / segments;
    // Use deterministic noise so it looks like real coverage
    const seed = Math.sin(i * 12.9898) * 43758.5453;
    const noise = seed - Math.floor(seed);
    const known = noise < coverage;
    const intensity = known ? 0.4 + (1 - Math.abs(t - 0.5) * 1.4) * 0.6 : 0.08 + noise * 0.18;
    cells.push(
      <div
        key={i}
        className="heat-cell"
        style={{
          background: known ? accent : "var(--bg-raise)",
          opacity: intensity,
        }}
      />
    );
  }
  return <div className="heat-strip">{cells}</div>;
}

// Donut/ring stat
function Ring({ value, total, color = "var(--accent)", size = 56, strokeWidth = 6, label }) {
  const pct = total > 0 ? value / total : 0;
  const radius = (size - strokeWidth) / 2;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference * (1 - pct);
  return (
    <div style={{ position: "relative", width: size, height: size, flex: "none" }}>
      <svg width={size} height={size} style={{ transform: "rotate(-90deg)" }}>
        <circle cx={size / 2} cy={size / 2} r={radius} fill="none" stroke="var(--line)" strokeWidth={strokeWidth} />
        <circle
          cx={size / 2} cy={size / 2} r={radius}
          fill="none" stroke={color} strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          style={{ transition: "stroke-dashoffset 0.5s" }}
        />
      </svg>
      <div style={{
        position: "absolute", inset: 0, display: "grid", placeItems: "center",
        fontFamily: "var(--font-mono)", fontSize: 11, fontWeight: 600,
        fontVariantNumeric: "tabular-nums",
      }}>
        {Math.round(pct * 100)}
      </div>
    </div>
  );
}

// Big stat block
function Stat({ label, value, sub, accent }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <div className="mono" style={{ fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
        {label}
      </div>
      <div className="num disp" style={{ fontSize: 32, fontWeight: 600, lineHeight: 1, color: accent || "var(--ink)", letterSpacing: "-0.02em" }}>
        {value}
      </div>
      {sub && <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)" }}>{sub}</div>}
    </div>
  );
}

// Coverage bar with segments (known / learning / new)
function CoverageBar({ known, learning, neww, height = 8 }) {
  const total = known + learning + neww;
  return (
    <div style={{
      display: "flex", height, borderRadius: 2, overflow: "hidden",
      background: "var(--bg-raise)", border: "1px solid var(--line)",
    }}>
      <div style={{ flex: known / total, background: "var(--accent-2)" }} />
      <div style={{ flex: learning / total, background: "var(--accent-3)" }} />
      <div style={{ flex: neww / total, background: "var(--accent)" }} />
    </div>
  );
}

// Renders a sentence with a highlighted span on a target lemma's surface form
function Sentence({ text, highlight, status = "new" }) {
  if (!highlight) return <span>{text}</span>;
  const idx = text.indexOf(highlight);
  if (idx === -1) return <span>{text}</span>;
  return (
    <>
      {text.slice(0, idx)}
      <span className={`hl ${status}`}>{highlight}</span>
      {text.slice(idx + highlight.length)}
    </>
  );
}

// Toggle pill group (segmented)
function Segmented({ options, value, onChange }) {
  return (
    <div style={{
      display: "inline-flex",
      background: "var(--bg-deep)",
      border: "1px solid var(--line)",
      borderRadius: 4,
      padding: 2,
      gap: 1,
    }}>
      {options.map(opt => (
        <button
          key={opt.value}
          onClick={() => onChange(opt.value)}
          className="mono"
          style={{
            all: "unset",
            cursor: "pointer",
            padding: "4px 10px",
            fontSize: 11,
            letterSpacing: "0.04em",
            textTransform: "uppercase",
            borderRadius: 3,
            color: value === opt.value ? "var(--bg-deep)" : "var(--ink-soft)",
            background: value === opt.value ? "var(--ink)" : "transparent",
            fontWeight: 500,
            transition: "all 0.08s",
          }}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

Object.assign(window, { StatusDot, Tag, Spark, HeatStrip, Ring, Stat, CoverageBar, Sentence, Segmented });
