/* global React, FED_M */
const { useState: useStateMs, useEffect: useEffectMs, useRef: useRefMs } = React;

// ════════════════════════════════════════════════════════════════════════════
// SHARED HOOKS / PRIMITIVES
// ════════════════════════════════════════════════════════════════════════════

// Phased word-by-word parsing animation — drives the wordlist reveal.
function useStaggerReveal(count, { delay = 80, startDelay = 200, deps = [] } = {}) {
  const [revealed, setRevealed] = useStateMs(0);
  useEffectMs(() => {
    setRevealed(0);
    let i = 0;
    const start = setTimeout(function tick() {
      setRevealed(++i);
      if (i < count) setTimeout(tick, delay);
    }, startDelay);
    return () => clearTimeout(start);
  }, deps); // eslint-disable-line
  return revealed;
}

// Shimmer dot for the "parsing…" state
function PulseDot({ size = 6, color = "currentColor" }) {
  return (
    <span style={{
      display: "inline-block", width: size, height: size, borderRadius: "50%",
      background: color, animation: "mPulse 1.2s ease-in-out infinite",
      verticalAlign: "middle",
    }} />
  );
}

// ════════════════════════════════════════════════════════════════════════════
// SWIPE REVIEW CARD — used by all three platforms
// ════════════════════════════════════════════════════════════════════════════

function SwipeReviewCard({ tone = "web", width = 340, height = 540 }) {
  const c = FED_M.card;
  const [revealed, setRevealed] = useStateMs(false);
  const [drag, setDrag] = useStateMs({ x: 0, y: 0, active: false });
  const [released, setReleased] = useStateMs(null); // {dir, label} during exit anim
  const startRef = useRefMs(null);
  const cardRef = useRefMs(null);

  // burst particles for right-swipe celebration
  const [burst, setBurst] = useStateMs(false);

  const dir =
    Math.abs(drag.x) > Math.abs(drag.y)
      ? drag.x > 0 ? "right" : drag.x < 0 ? "left" : null
      : drag.y > 0 ? "down" : drag.y < 0 ? "up" : null;
  const dist = Math.hypot(drag.x, drag.y);
  const intensity = Math.min(dist / 140, 1);

  const labelMap = {
    right: { text: "GOOD",  color: FED_M.brand.known    },
    left:  { text: "AGAIN", color: "#EF4444"            },
    up:    { text: "EASY",  color: FED_M.brand.accent   },
    down:  { text: "HARD",  color: FED_M.brand.learning },
  };
  const activeLabel = drag.active && intensity > 0.25 && dir ? labelMap[dir] : null;

  function onDown(e) {
    if (!revealed) { setRevealed(true); return; }
    const t = e.touches ? e.touches[0] : e;
    startRef.current = { x: t.clientX, y: t.clientY };
    setDrag({ x: 0, y: 0, active: true });
  }
  function onMove(e) {
    if (!startRef.current) return;
    const t = e.touches ? e.touches[0] : e;
    setDrag({ x: t.clientX - startRef.current.x, y: t.clientY - startRef.current.y, active: true });
  }
  function onUp() {
    if (!startRef.current) return;
    startRef.current = null;
    if (dist > 90 && dir) {
      setReleased({ dir, ...labelMap[dir] });
      if (dir === "right") { setBurst(true); setTimeout(() => setBurst(false), 800); }
      setTimeout(() => {
        setRevealed(false);
        setReleased(null);
        setDrag({ x: 0, y: 0, active: false });
      }, 380);
    } else {
      setDrag({ x: 0, y: 0, active: false });
    }
  }

  // Apply transform either from drag or from release-fly-out
  const tx = released
    ? (released.dir === "right" ? width * 1.4 : released.dir === "left" ? -width * 1.4 : drag.x)
    : drag.x;
  const ty = released
    ? (released.dir === "down" ? height * 1.4 : released.dir === "up" ? -height * 1.4 : drag.y)
    : drag.y;
  const rot = (drag.x / 12) || 0;

  return (
    <div style={{ position: "relative", width, height, userSelect: "none" }}>
      {/* Background "next card" peek */}
      <div style={{
        position: "absolute", inset: 0,
        borderRadius: tone === "ios" ? 20 : 18,
        background: tone === "ios" ? "#fff" : tone === "android" ? "#FEF7FF" : "#FAFAF7",
        border: tone === "android" ? "1px solid #E7E0EC" : "1px solid rgba(0,0,0,0.06)",
        transform: "scale(0.94) translateY(8px)",
        opacity: 0.6,
      }} />

      {/* Direction tint glow */}
      {dir && drag.active && (
        <div style={{
          position: "absolute", inset: -20,
          borderRadius: 32, pointerEvents: "none",
          background: `radial-gradient(closest-side, ${labelMap[dir].color}33, transparent 70%)`,
          opacity: intensity,
          transition: drag.active ? "none" : "opacity 0.2s",
        }} />
      )}

      {/* Card */}
      <div
        ref={cardRef}
        onMouseDown={onDown} onMouseMove={drag.active ? onMove : undefined} onMouseUp={onUp} onMouseLeave={drag.active ? onUp : undefined}
        onTouchStart={onDown} onTouchMove={onMove} onTouchEnd={onUp}
        style={{
          position: "absolute", inset: 0,
          borderRadius: tone === "ios" ? 20 : 18,
          background: tone === "ios" ? "#fff" : tone === "android" ? "#FFFBFE" : "#FAFAF7",
          border: tone === "ios" ? "1px solid rgba(0,0,0,0.06)" : tone === "android" ? "1px solid #E7E0EC" : "1px solid rgba(0,0,0,0.08)",
          boxShadow: tone === "ios"
            ? "0 12px 36px rgba(0,0,0,0.10), 0 1px 2px rgba(0,0,0,0.04)"
            : tone === "android"
            ? "0 6px 18px rgba(103,80,164,0.16)"
            : "0 8px 24px rgba(0,0,0,0.10)",
          transform: `translate(${tx}px, ${ty}px) rotate(${rot}deg)`,
          transition: drag.active ? "none" : "transform 0.32s cubic-bezier(0.2,0.8,0.2,1), opacity 0.32s",
          opacity: released ? 0 : 1,
          padding: 20,
          display: "flex", flexDirection: "column",
          cursor: drag.active ? "grabbing" : "grab",
          touchAction: "none",
          fontFamily: tone === "ios" ? "-apple-system, 'SF Pro Text', system-ui, sans-serif"
                    : tone === "android" ? "'Roboto', system-ui, sans-serif"
                    : "system-ui, -apple-system, sans-serif",
        }}
      >
        {/* Top meta */}
        <div style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 11, color: "#6B7280", marginBottom: 14 }}>
          <span style={{
            padding: "2px 7px", borderRadius: tone === "android" ? 8 : 4,
            background: `${FED_M.brand.fi}15`, color: FED_M.brand.fi, fontWeight: 600, fontSize: 10,
            letterSpacing: "0.04em",
          }}>FI</span>
          <span style={{
            padding: "2px 7px", borderRadius: tone === "android" ? 8 : 4,
            background: "#F3F4F6", color: "#4B5563", fontWeight: 500, fontSize: 10,
          }}>{c.pos}</span>
          <div style={{ flex: 1 }} />
          <span style={{ fontSize: 10.5 }}>{c.deck}</span>
        </div>

        {/* Sentence */}
        <div style={{
          flex: revealed ? "none" : 1,
          display: "flex", alignItems: "center", justifyContent: "center",
          textAlign: "center", padding: revealed ? "8px 0" : "16px 0",
        }}>
          <div style={{
            fontSize: revealed ? 19 : 24, lineHeight: 1.35, fontWeight: 500,
            letterSpacing: "-0.01em", color: "#111827",
            transition: "font-size 0.3s",
          }}>
            {(() => {
              const i = c.sentence.indexOf(c.highlight);
              if (i === -1) return c.sentence;
              return <>
                {c.sentence.slice(0, i)}
                <span style={{
                  background: revealed ? "transparent" : `${FED_M.brand.accent}25`,
                  color: FED_M.brand.accent, fontWeight: 700,
                  padding: "0 2px", borderRadius: 3,
                }}>{c.highlight}</span>
                {c.sentence.slice(i + c.highlight.length)}
              </>;
            })()}
          </div>
        </div>

        {revealed && (
          <>
            <div style={{ borderTop: "1px solid #E5E7EB", margin: "8px 0 14px" }} />
            <div style={{ textAlign: "center", marginBottom: 10 }}>
              <div style={{ fontSize: 26, fontWeight: 700, color: FED_M.brand.accent, letterSpacing: "-0.02em" }}>
                {c.gloss}
              </div>
              <div style={{ fontSize: 11.5, color: "#6B7280", marginTop: 4, fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace" }}>
                <b style={{ color: "#111827" }}>{c.lemma}</b> · {c.grammar}
              </div>
              <div style={{ marginTop: 10, display: "flex", gap: 4, justifyContent: "center" }}>
                {c.morph.map((m, j) => (
                  <span key={j} style={{
                    padding: "3px 8px", borderRadius: tone === "android" ? 8 : 4,
                    background: "#F9FAFB", border: "1px solid #E5E7EB",
                    fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace", fontSize: 11.5,
                  }}>
                    {m.stem}<span style={{ color: FED_M.brand.accent, marginLeft: 2 }}>+{m.suffix}</span>
                  </span>
                ))}
              </div>
            </div>

            {/* Examples (compact for mobile) */}
            <div style={{ marginTop: 8, fontSize: 12 }}>
              <div style={{
                fontSize: 9.5, color: "#9CA3AF", letterSpacing: "0.16em",
                textTransform: "uppercase", textAlign: "center", marginBottom: 6, fontWeight: 600,
              }}>
                Other examples
              </div>
              {c.examples.slice(0, 1).map((ex, k) => (
                <div key={k} style={{
                  padding: "8px 10px", marginBottom: 4,
                  background: tone === "android" ? "#F7F2FA" : "#F9FAFB",
                  border: tone === "android" ? "none" : "1px solid #E5E7EB",
                  borderRadius: tone === "android" ? 12 : 6,
                }}>
                  <div style={{ color: "#111827", lineHeight: 1.4 }}>{ex.fi}</div>
                  <div style={{ color: "#6B7280", fontSize: 11, marginTop: 2 }}>{ex.en}</div>
                </div>
              ))}
            </div>

            {/* Swipe hint */}
            <div style={{ flex: 1 }} />
            <div style={{ fontSize: 10, color: "#9CA3AF", textAlign: "center", marginTop: 10, letterSpacing: "0.08em" }}>
              ← AGAIN · ↓ HARD · → GOOD · ↑ EASY
            </div>
          </>
        )}

        {!revealed && (
          <>
            <div style={{ flex: 1 }} />
            <div style={{
              padding: "10px 14px",
              background: tone === "ios" ? "rgba(0,122,255,0.08)" : tone === "android" ? "#EADDFF" : "#111827",
              color: tone === "ios" ? "#007AFF" : tone === "android" ? "#21005D" : "#fff",
              borderRadius: tone === "android" ? 100 : 10,
              textAlign: "center", fontWeight: 600, fontSize: 13, letterSpacing: "0.02em",
            }}>
              Tap to reveal
            </div>
          </>
        )}

        {/* Active swipe label overlay */}
        {activeLabel && (
          <div style={{
            position: "absolute",
            top: dir === "down" ? "auto" : 24,
            bottom: dir === "down" ? 24 : "auto",
            left: dir === "right" ? 24 : dir === "left" ? "auto" : "50%",
            right: dir === "left" ? 24 : "auto",
            transform: dir === "up" || dir === "down" ? "translateX(-50%)" : "none",
            padding: "6px 14px",
            border: `2px solid ${activeLabel.color}`,
            color: activeLabel.color,
            borderRadius: 8,
            fontWeight: 800, fontSize: 16, letterSpacing: "0.12em",
            background: "rgba(255,255,255,0.92)",
            opacity: intensity,
            pointerEvents: "none",
          }}>
            {activeLabel.text}
          </div>
        )}
      </div>

      {/* Right-swipe celebration burst */}
      {burst && (
        <div style={{ position: "absolute", inset: 0, pointerEvents: "none", overflow: "visible" }}>
          {Array.from({ length: 14 }).map((_, i) => {
            const angle = (i / 14) * Math.PI * 2;
            const r = 80 + Math.random() * 60;
            const dx = Math.cos(angle) * r;
            const dy = Math.sin(angle) * r;
            const colors = [FED_M.brand.known, FED_M.brand.accent, FED_M.brand.fi, "#FACC15"];
            const cc = colors[i % colors.length];
            return (
              <span key={i} style={{
                position: "absolute", left: "50%", top: "50%",
                width: 8, height: 8, borderRadius: "50%",
                background: cc,
                animation: `mBurst 0.7s cubic-bezier(0.2,0.8,0.4,1) forwards`,
                ["--dx"]: `${dx}px`, ["--dy"]: `${dy}px`,
              }} />
            );
          })}
          <span style={{
            position: "absolute", left: "50%", top: "50%",
            width: 80, height: 80, borderRadius: "50%",
            border: `3px solid ${FED_M.brand.known}`,
            transform: "translate(-50%, -50%)",
            animation: "mRing 0.7s cubic-bezier(0.2,0.8,0.4,1) forwards",
          }} />
        </div>
      )}
    </div>
  );
}

window.SwipeReviewCard = SwipeReviewCard;
window.useStaggerReveal = useStaggerReveal;
window.PulseDot = PulseDot;
