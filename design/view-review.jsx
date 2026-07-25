/* global React, FED_DATA, Tag, Sentence */
const { useState: useStateR } = React;

function ReviewView() {
  const cards = [
    {
      idx: 1, total: 24, isNew: true, mode: "sentence",
      sentence: "Toissapäivänä menin pankkiin.",
      highlight: "Toissapäivänä",
      lemma: "toissapäivä",
      meaning: "the day before yesterday",
      grammar: "Essive singular (-nä)",
      pos: "NOUN",
      morph: [{ stem: "toissapäivä", suffix: "nä" }],
      examples: [{ text: "Toissapäivänä menin pankkiin.", source: "Konsta Punkkinen - vol I" }],
    }
  ];
  const [revealed, setRevealed] = useStateR(false);
  const c = cards[0];

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Review</h1>
          <div className="sub">FSRS · session {c.idx} of {c.total} · est {Math.round((c.total - c.idx) * 0.7)} min</div>
        </div>
        <div className="actions">
          <button className="btn">Pause</button>
          <button className="btn">Settings</button>
          <button className="btn">Exit · <span className="kbd">Esc</span></button>
        </div>
      </div>

      <div className="page-body" style={{ display: "grid", gridTemplateColumns: "1fr 280px", gap: 24, maxWidth: 1100, margin: "0 auto" }}>

        <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>

          {/* Progress strip */}
          <div style={{ display: "flex", gap: 2, height: 4 }}>
            {Array.from({ length: c.total }).map((_, i) => (
              <div key={i} style={{
                flex: 1,
                background: i < c.idx ? "var(--accent-2)" : i === c.idx ? "var(--accent)" : "var(--bg-raise)",
                borderRadius: 1,
              }} />
            ))}
          </div>

          {/* Card */}
          <div className="panel" style={{ padding: "48px 48px 32px", minHeight: 460, display: "flex", flexDirection: "column" }}>
            <div style={{ display: "flex", gap: 6, marginBottom: 32 }}>
              <Tag kind="new">NEW</Tag>
              <Tag kind="fi">FI</Tag>
              <Tag>{c.pos}</Tag>
              <div style={{ flex: 1 }} />
              <span className="mono" style={{ fontSize: 11, color: "var(--ink-mute)" }}>
                Konsta Punkkinen · sentence 1
              </span>
            </div>

            {/* Front */}
            <div style={{ flex: 1, display: "flex", flexDirection: "column", justifyContent: "center", alignItems: "center", textAlign: "center" }}>
              <div className="disp" style={{ fontSize: 38, lineHeight: 1.3, letterSpacing: "-0.02em", maxWidth: 700, marginBottom: 24 }}>
                <Sentence text={c.sentence} highlight={c.highlight} status="new" />
              </div>

              {revealed && (
                <div style={{ borderTop: "1px solid var(--line)", paddingTop: 28, marginTop: 8, width: "100%", maxWidth: 540 }}>
                  <div className="disp" style={{ fontSize: 44, fontWeight: 600, color: "var(--accent)", letterSpacing: "-0.02em", lineHeight: 1.05 }}>
                    {c.meaning}
                  </div>
                  <div className="mono" style={{ fontSize: 13, color: "var(--ink-soft)", marginTop: 8 }}>
                    lemma <b style={{ color: "var(--ink)" }}>{c.lemma}</b> · {c.grammar}
                  </div>
                  <div style={{ display: "flex", gap: 6, justifyContent: "center", marginTop: 16 }}>
                    {c.morph.map((m, i) => (
                      <span key={i} className="mono" style={{ padding: "4px 10px", background: "var(--bg)", border: "1px solid var(--line)", borderRadius: 3, fontSize: 13 }}>
                        <span>{m.stem}</span>
                        {m.suffix && <span style={{ color: "var(--accent)", marginLeft: 4 }}>+{m.suffix}</span>}
                      </span>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* Action area */}
          {!revealed ? (
            <button
              className="btn btn-primary"
              style={{ padding: "14px", justifyContent: "center", fontSize: 14 }}
              onClick={() => setRevealed(true)}
            >
              Show answer · <span className="kbd">Space</span>
            </button>
          ) : (
            <>
              <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 8 }}>
                {[
                  { k: "Again", t: "<1m", c: "oklch(0.62 0.22 28)", n: "1" },
                  { k: "Hard", t: "8m", c: "var(--accent-3)", n: "2" },
                  { k: "Good", t: "1d", c: "var(--accent-2)", n: "3" },
                  { k: "Easy", t: "4d", c: "var(--accent-4)", n: "4" },
                ].map(b => (
                  <button key={b.k} className="btn" style={{
                    flexDirection: "column",
                    padding: "14px 12px",
                    borderColor: b.c,
                    color: b.c,
                    fontWeight: 600,
                    fontSize: 14,
                    gap: 4,
                    alignItems: "stretch",
                  }} onClick={() => setRevealed(false)}>
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <span>{b.k}</span>
                      <span className="kbd" style={{ borderColor: "currentColor" }}>{b.n}</span>
                    </div>
                    <div className="mono" style={{ fontSize: 11, opacity: 0.7, fontWeight: 400, textAlign: "left" }}>next · {b.t}</div>
                  </button>
                ))}
              </div>

              <div style={{ display: "flex", justifyContent: "center", gap: 6 }}>
                <button className="btn btn-ghost" style={{ fontSize: 11.5 }}>↻ Show context</button>
                <button className="btn btn-ghost" style={{ fontSize: 11.5 }}>★ Mark known</button>
                <button className="btn btn-ghost" style={{ fontSize: 11.5 }}>⊘ Ignore</button>
                <button className="btn btn-ghost" style={{ fontSize: 11.5 }}>✎ Edit card</button>
              </div>
            </>
          )}
        </div>

        {/* Right - session info */}
        <div style={{ display: "flex", flexDirection: "column", gap: 16, position: "sticky", top: 16, alignSelf: "start" }}>
          <div className="panel" style={{ padding: 18 }}>
            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 14 }}>
              Today
            </div>
            {[
              { l: "Reviewed",  v: c.idx - 1, c: "var(--accent-2)" },
              { l: "Remaining", v: c.total - c.idx + 1, c: "var(--accent)" },
              { l: "Streak",    v: FED_DATA.TODAY_QUEUE.streak_days + "d", c: "var(--ink)" },
              { l: "Retention", v: Math.round(FED_DATA.TODAY_QUEUE.retention_30d * 100) + "%", c: "var(--accent-2)" },
            ].map(r => (
              <div key={r.l} style={{ display: "flex", justifyContent: "space-between", padding: "8px 0", borderBottom: "1px dashed var(--line-soft)", fontSize: 13 }}>
                <span style={{ color: "var(--ink-mute)" }}>{r.l}</span>
                <span className="mono num" style={{ color: r.c, fontWeight: 600 }}>{r.v}</span>
              </div>
            ))}
          </div>

          <div className="panel" style={{ padding: 18 }}>
            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 10 }}>
              Keyboard
            </div>
            {[
              ["Space", "show answer"],
              ["1 / 2 / 3 / 4", "again / hard / good / easy"],
              ["U", "undo last"],
              ["E", "edit card"],
              ["Esc", "exit session"],
            ].map(([k, v]) => (
              <div key={k} style={{ display: "flex", justifyContent: "space-between", padding: "5px 0", fontSize: 12 }}>
                <span className="mono" style={{ fontSize: 11, padding: "1px 5px", border: "1px solid var(--line)", borderRadius: 3 }}>{k}</span>
                <span style={{ color: "var(--ink-mute)" }}>{v}</span>
              </div>
            ))}
          </div>
        </div>

      </div>
    </>
  );
}

window.ReviewView = ReviewView;
