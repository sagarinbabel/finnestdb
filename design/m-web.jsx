/* global React, FED_M, SwipeReviewCard, useStaggerReveal, PulseDot */
const { useState: useStateMw, useEffect: useEffectMw } = React;

// ════════════════════════════════════════════════════════════════════════════
// MOBILE WEB - clean PWA-style, no native chrome
// Frame is just a phone-shaped container; the "browser" is the actual web app.
// ════════════════════════════════════════════════════════════════════════════

const WEB_FONT = "system-ui, -apple-system, 'Segoe UI', sans-serif";

function MWFrame({ children, addressBar = "finnest.app", chrome = true, dark = false }) {
  return (
    <div style={{
      width: 380, height: 760, borderRadius: 36,
      background: dark ? "#0B0E0F" : "#FAFAF7",
      border: "10px solid #1B1B1B",
      boxShadow: "0 30px 60px rgba(0,0,0,0.18), inset 0 0 0 1.5px #2a2a2a",
      overflow: "hidden",
      position: "relative",
      fontFamily: WEB_FONT,
      color: dark ? "#F9FAFB" : "#111827",
      display: "flex", flexDirection: "column",
    }}>
      {/* Status bar */}
      <div style={{
        height: 32, padding: "0 22px", display: "flex", alignItems: "center", justifyContent: "space-between",
        fontSize: 13, fontWeight: 600,
      }}>
        <span>9:41</span>
        <span style={{ display: "flex", gap: 4, alignItems: "center", fontSize: 11 }}>
          <span>●●●</span><span>5G</span>
          <span style={{
            display: "inline-block", width: 22, height: 11,
            border: `1.2px solid ${dark ? "#fff" : "#000"}`, borderRadius: 2.5, position: "relative", marginLeft: 4,
          }}>
            <span style={{ position: "absolute", inset: 1.4, width: "82%", background: dark ? "#fff" : "#000", borderRadius: 1 }} />
          </span>
        </span>
      </div>

      {/* Browser chrome */}
      {chrome && (
        <div style={{
          padding: "4px 10px 8px", display: "flex", gap: 8, alignItems: "center",
          background: dark ? "#0B0E0F" : "#FAFAF7",
          borderBottom: dark ? "1px solid #1F2937" : "1px solid #E5E7EB",
        }}>
          <span style={{ fontSize: 16, color: dark ? "#9CA3AF" : "#6B7280" }}>‹</span>
          <span style={{ fontSize: 16, color: dark ? "#9CA3AF" : "#6B7280" }}>›</span>
          <div style={{
            flex: 1, height: 32, borderRadius: 10,
            background: dark ? "#1F2937" : "#F3F4F6",
            display: "flex", alignItems: "center", padding: "0 10px",
            fontSize: 12, color: dark ? "#9CA3AF" : "#6B7280",
            gap: 6,
          }}>
            <span style={{ fontSize: 10 }}>🔒</span>
            <span style={{ fontWeight: 500, color: dark ? "#E5E7EB" : "#111827" }}>{addressBar}</span>
          </div>
          <span style={{ fontSize: 16, color: dark ? "#9CA3AF" : "#6B7280" }}>↑</span>
          <span style={{ fontSize: 16, color: dark ? "#9CA3AF" : "#6B7280" }}>⎙</span>
        </div>
      )}

      {/* Content */}
      <div style={{ flex: 1, overflow: "hidden", position: "relative" }}>
        {children}
      </div>

      {/* Home indicator */}
      <div style={{ height: 28, display: "flex", alignItems: "center", justifyContent: "center", background: dark ? "#0B0E0F" : "#FAFAF7" }}>
        <div style={{ width: 110, height: 4, background: dark ? "#fff" : "#000", borderRadius: 2, opacity: 0.85 }} />
      </div>
    </div>
  );
}

// ── Web · Landing ───────────────────────────────────────────────────────────
function MWLanding() {
  const [text, setText] = useStateMw("");
  return (
    <MWFrame addressBar="finnest.app">
      <div style={{ padding: "20px 18px 0", flex: 1, display: "flex", flexDirection: "column" }}>
        {/* App brand row */}
        <div style={{ display: "flex", alignItems: "center", marginBottom: 18 }}>
          <span style={{ fontSize: 22, fontWeight: 800, letterSpacing: "-0.03em" }}>finnest<span style={{ color: FED_M.brand.accent }}>.</span></span>
          <div style={{ flex: 1 }} />
          <span style={{
            width: 32, height: 32, borderRadius: "50%",
            background: "#FBBF24", color: "#fff",
            display: "grid", placeItems: "center", fontSize: 13, fontWeight: 700,
          }}>S</span>
        </div>

        <div style={{
          display: "inline-flex", alignSelf: "flex-start", alignItems: "center", gap: 6,
          padding: "4px 10px", marginBottom: 12,
          fontFamily: "ui-monospace, monospace", fontSize: 10,
          color: "#9CA3AF", letterSpacing: "0.08em",
        }}>
          <PulseDot color={FED_M.brand.known} /> free · no history saved
        </div>

        <div style={{ fontSize: 26, fontWeight: 600, lineHeight: 1.15, letterSpacing: "-0.02em", marginBottom: 8 }}>
          Paste your Finnish<br/>or Estonian text.
        </div>
        <div style={{ fontSize: 13, color: "#6B7280", lineHeight: 1.45, marginBottom: 16 }}>
          Get the words, meanings, grammar - all parsed against a real dictionary.
        </div>

        {/* Paste area */}
        <div style={{
          flex: 1,
          background: "#fff",
          border: `1.5px solid ${text ? FED_M.brand.accent : "#E5E7EB"}`,
          borderRadius: 12,
          display: "flex", flexDirection: "column",
          transition: "border-color 0.2s",
        }}>
          <textarea
            value={text}
            onChange={e => setText(e.target.value)}
            placeholder="Toissapäivänä menin pankkiin…"
            style={{
              all: "unset",
              padding: "14px 14px 8px",
              fontSize: 16, lineHeight: 1.5, color: "#111827",
              minHeight: 120, flex: 1,
              fontFamily: "Georgia, 'Times New Roman', serif",
              fontStyle: text ? "normal" : "italic",
            }}
          />
          {/* sticky footer */}
          <div style={{
            display: "flex", alignItems: "center", padding: "10px 12px",
            borderTop: "1px solid #E5E7EB", background: "#FAFAF7",
            borderRadius: "0 0 12px 12px", gap: 8,
          }}>
            <span style={{
              display: "inline-flex", alignItems: "center", gap: 5,
              padding: "3px 9px", borderRadius: 4,
              background: text ? `${FED_M.brand.fi}15` : "#F3F4F6",
              color: text ? FED_M.brand.fi : "#9CA3AF",
              fontSize: 11, fontWeight: 600, letterSpacing: "0.04em",
            }}>
              <span style={{ fontFamily: "Georgia, serif", fontStyle: "italic", fontWeight: 700 }}>Aa</span>
              {text ? "FINNISH" : "DETECTING…"}
            </span>
            <div style={{ flex: 1 }} />
            <span style={{ fontSize: 10.5, color: "#9CA3AF", fontFamily: "ui-monospace, monospace" }}>
              {text.length}/300k
            </span>
          </div>
        </div>

        {/* Big primary action */}
        <button style={{
          all: "unset", textAlign: "center",
          padding: "16px", marginTop: 12, marginBottom: 14,
          background: FED_M.brand.accent, color: "#fff",
          borderRadius: 12, fontWeight: 700, fontSize: 16,
          boxShadow: `0 6px 18px ${FED_M.brand.accent}44`,
          cursor: "pointer",
          opacity: text ? 1 : 0.5,
        }}>
          Parse text →
        </button>
      </div>
    </MWFrame>
  );
}

// ── Web · Wordlist (with parse-in animation) ────────────────────────────────
function MWWordlist() {
  const words = FED_M.words;
  const reveal = useStaggerReveal(words.length, { delay: 70, startDelay: 350 });
  const parsing = reveal < words.length;

  return (
    <MWFrame addressBar="finnest.app/parse">
      <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
        {/* Header */}
        <div style={{ padding: "12px 16px", borderBottom: "1px solid #E5E7EB", background: "#FAFAF7" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <span style={{ fontSize: 18, color: "#6B7280" }}>‹</span>
            <span style={{
              padding: "2px 8px", borderRadius: 4,
              background: `${FED_M.brand.fi}15`, color: FED_M.brand.fi,
              fontSize: 10, fontWeight: 700, letterSpacing: "0.04em",
            }}>FI · FINNISH</span>
            <div style={{ flex: 1 }} />
            <span style={{ fontSize: 11, color: "#6B7280", fontFamily: "ui-monospace, monospace" }}>
              {parsing ? <><PulseDot color={FED_M.brand.accent} /> parsing… {reveal}/{words.length}</> : `${words.length} unique lemmas`}
            </span>
          </div>
        </div>

        {/* List */}
        <div style={{ flex: 1, overflow: "auto", padding: "4px 0" }}>
          {words.map((w, i) => {
            const visible = i < reveal;
            return (
              <div key={w.lemma} style={{
                padding: "12px 16px",
                borderBottom: i < words.length - 1 ? "1px solid #F3F4F6" : "none",
                opacity: visible ? 1 : 0,
                transform: visible ? "translateX(0)" : "translateX(-8px)",
                transition: "opacity 0.3s, transform 0.3s",
              }}>
                <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
                  <span style={{
                    width: 6, height: 6, borderRadius: "50%",
                    background: w.status === "known" ? FED_M.brand.known
                              : w.status === "learning" ? FED_M.brand.learning
                              : FED_M.brand.new,
                    flex: "none", alignSelf: "center",
                  }} />
                  <span style={{
                    fontFamily: "ui-monospace, 'SF Mono', monospace",
                    fontSize: 15, fontWeight: 600, color: "#111827",
                  }}>{w.lemma}</span>
                  <span style={{
                    padding: "1px 6px", borderRadius: 3,
                    background: "#F3F4F6", color: "#6B7280",
                    fontSize: 9.5, fontWeight: 600, letterSpacing: "0.04em",
                  }}>{w.pos}</span>
                  <div style={{ flex: 1 }} />
                  <span style={{ fontSize: 10.5, color: "#9CA3AF", fontFamily: "ui-monospace, monospace" }}>×{w.count}</span>
                </div>
                <div style={{ paddingLeft: 14, marginTop: 4, display: "flex", gap: 12, alignItems: "baseline" }}>
                  <span style={{ fontSize: 13, color: "#374151", flex: 1, lineHeight: 1.3 }}>{w.gloss}</span>
                </div>
                <div style={{ paddingLeft: 14, marginTop: 3, fontSize: 10.5, color: "#9CA3AF", fontFamily: "ui-monospace, monospace" }}>
                  {w.forms.join(", ")} · {w.grammar}
                </div>
              </div>
            );
          })}
        </div>

        {/* Bottom action bar */}
        <div style={{
          padding: "10px 14px",
          borderTop: "1px solid #E5E7EB", background: "#FAFAF7",
          display: "flex", gap: 8,
        }}>
          <button style={{
            all: "unset", textAlign: "center", flex: 1,
            padding: "11px", border: "1px solid #D1D5DB", borderRadius: 8,
            fontWeight: 600, fontSize: 13, color: "#374151",
          }}>Copy list</button>
          <button style={{
            all: "unset", textAlign: "center", flex: 1.4,
            padding: "11px", background: "#111827", color: "#fff",
            borderRadius: 8, fontWeight: 600, fontSize: 13,
          }}>Save as deck →</button>
        </div>
      </div>
    </MWFrame>
  );
}

// ── Web · Review (swipe) ────────────────────────────────────────────────────
function MWReview() {
  return (
    <MWFrame addressBar="finnest.app/review" chrome={false}>
      <div style={{ height: "100%", display: "flex", flexDirection: "column", background: "#FAFAF7" }}>
        {/* Top bar */}
        <div style={{ padding: "12px 16px", display: "flex", alignItems: "center", gap: 10 }}>
          <span style={{ fontSize: 18, color: "#6B7280" }}>‹</span>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 14, fontWeight: 700, lineHeight: 1.1 }}>Review</div>
            <div style={{ fontSize: 11, color: "#6B7280", marginTop: 2 }}>
              Konsta Punkkinen · 4 / 24
            </div>
          </div>
          <span style={{ fontSize: 16, color: "#6B7280" }}>⋯</span>
        </div>

        {/* Progress strip */}
        <div style={{ display: "flex", gap: 2, padding: "0 16px 14px" }}>
          {Array.from({ length: 24 }).map((_, i) => (
            <div key={i} style={{
              flex: 1, height: 3,
              background: i < 4 ? FED_M.brand.known : i === 4 ? FED_M.brand.accent : "#E5E7EB",
              borderRadius: 1,
            }} />
          ))}
        </div>

        {/* Card stack */}
        <div style={{ flex: 1, display: "grid", placeItems: "center", padding: "8px 16px 24px" }}>
          <SwipeReviewCard tone="web" width={324} height={500} />
        </div>
      </div>
    </MWFrame>
  );
}

window.MWLanding = MWLanding;
window.MWWordlist = MWWordlist;
window.MWReview = MWReview;
