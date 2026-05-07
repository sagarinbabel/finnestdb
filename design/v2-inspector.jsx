/* global React */
const { useEffect: useEffectI } = React;

function Inspector({ word, onClose }) {
  useEffectI(() => {
    if (!word) return;
    const onKey = (e) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [word, onClose]);

  return (
    <>
      <div className={`popover-mask ${word ? "open" : ""}`} onClick={onClose} />
      <aside className={`popover ${word ? "open" : ""}`}>
        {word && (
          <div style={{ padding: "20px 24px 32px" }}>
            <div style={{ display: "flex", alignItems: "center", marginBottom: 24 }}>
              <span className="mono" style={{ fontSize: 10, letterSpacing: "0.18em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
                Inspector
              </span>
              <div style={{ flex: 1 }} />
              <button className="btn btn-ghost" onClick={onClose}>Close · <span className="kbd">Esc</span></button>
            </div>

            <div style={{ display: "flex", gap: 6, marginBottom: 12, flexWrap: "wrap" }}>
              <span className="tag">{word.pos}</span>
              <span className="tag">{word.grammar}</span>
              {word.pos === "MWE" && <span className="tag mwe">idiom</span>}
              <span className={`tag ${word.status}`}>{word.status}</span>
            </div>

            <div className="disp" style={{
              fontSize: 36, fontWeight: 600, lineHeight: 1.05, letterSpacing: "-0.02em",
              wordBreak: "break-word", marginBottom: 8,
              color: word.pos === "MWE" ? "var(--accent)" : "var(--ink)"
            }}>
              {word.lemma}
            </div>
            <div style={{ color: "var(--ink-soft)", fontSize: 17, marginBottom: 28, lineHeight: 1.4 }}>
              {word.gloss}
            </div>

            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 8 }}>
              Morphology
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 6, marginBottom: 24 }}>
              {word.morph.map((m, i) => (
                <div key={i} style={{
                  display: "flex", alignItems: "center", gap: 8,
                  padding: "8px 12px",
                  background: "var(--bg-deep)", border: "1px solid var(--line)",
                  borderRadius: 3,
                }}>
                  <span className="mono" style={{ fontSize: 13, color: "var(--ink)", fontWeight: 500 }}>{m.stem}</span>
                  {m.suffix && (
                    <>
                      <span className="mono" style={{ color: "var(--ink-mute)" }}>+</span>
                      <span className="mono" style={{ fontSize: 13, color: "var(--accent)", fontWeight: 500 }}>{m.suffix}</span>
                    </>
                  )}
                  <span style={{ flex: 1 }} />
                  <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-mute)" }}>{m.note}</span>
                </div>
              ))}
            </div>

            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 8 }}>
              Forms in text · {word.forms.length}
            </div>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 24 }}>
              {word.forms.map(f => (
                <span key={f} className="mono" style={{
                  padding: "4px 10px", fontSize: 12.5,
                  background: "var(--bg-deep)", border: "1px solid var(--line)", borderRadius: 3,
                }}>{f}</span>
              ))}
            </div>

            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 8 }}>
              Context
            </div>
            <div style={{
              padding: "12px 14px",
              background: "var(--bg-deep)", border: "1px solid var(--line)", borderRadius: 3,
              fontFamily: "var(--font-disp)", fontSize: 16, lineHeight: 1.5,
            }}>
              {word.exampleSentence || (window.FED_DATA?.SAMPLE_PARSE_FI?.sentences[word.sentence_id]?.text)}
            </div>
          </div>
        )}
      </aside>
    </>
  );
}

window.Inspector = Inspector;
