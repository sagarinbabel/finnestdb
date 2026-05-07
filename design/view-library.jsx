/* global React, FED_DATA, Tag, CoverageBar, Spark, HeatStrip, Ring */
const { useState: useStateL } = React;

function LibraryView({ onOpen }) {
  const decks = FED_DATA.SAMPLE_DECKS;
  const totals = decks.reduce((acc, d) => ({
    tokens: acc.tokens + d.tokens,
    unique: acc.unique + d.unique,
    known: acc.known + d.known,
    new: acc.new + d.new,
  }), { tokens: 0, unique: 0, known: 0, new: 0 });

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Library</h1>
          <div className="sub">
            {decks.length} decks · {totals.tokens.toLocaleString()} tokens · {totals.unique.toLocaleString()} unique lemmas
          </div>
        </div>
        <div className="actions">
          <button className="btn">Import .apkg</button>
          <button className="btn">Upload .epub</button>
          <button className="btn btn-primary">+ New deck</button>
        </div>
      </div>

      <div className="page-body">
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16, marginBottom: 24 }}>
          {[
            { l: "Total tokens", v: totals.tokens.toLocaleString(), s: "across all decks" },
            { l: "Unique lemmas", v: totals.unique.toLocaleString(), s: `${totals.known.toLocaleString()} known` },
            { l: "Coverage", v: Math.round(totals.known/totals.unique*100) + "%", s: "weighted average" },
            { l: "Backlog", v: totals.new.toLocaleString(), s: "new words ready" },
          ].map(s => (
            <div key={s.l} className="panel" style={{ padding: "16px 18px" }}>
              <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 6 }}>
                {s.l}
              </div>
              <div className="disp num" style={{ fontSize: 30, fontWeight: 600, lineHeight: 1, letterSpacing: "-0.02em" }}>
                {s.v}
              </div>
              <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", marginTop: 6 }}>
                {s.s}
              </div>
            </div>
          ))}
        </div>

        {/* Deck cards */}
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(360px, 1fr))", gap: 16 }}>
          {decks.map(d => (
            <div
              key={d.id}
              className="panel"
              style={{ cursor: "pointer", transition: "border-color 0.1s, transform 0.1s", overflow: "hidden" }}
              onMouseEnter={e => { e.currentTarget.style.borderColor = "var(--ink-mute)"; }}
              onMouseLeave={e => { e.currentTarget.style.borderColor = "var(--line)"; }}
              onClick={() => onOpen(d.id)}
            >
              {/* Color band */}
              <div style={{
                height: 4,
                background: d.lang === "FI" ? "var(--fi)" : "var(--et)",
              }} />
              <div style={{ padding: "16px 18px 14px" }}>
                <div style={{ display: "flex", alignItems: "flex-start", gap: 10, marginBottom: 4 }}>
                  <div style={{ flex: 1 }}>
                    <div className="disp" style={{ fontSize: 19, fontWeight: 600, letterSpacing: "-0.01em", lineHeight: 1.2 }}>
                      {d.title}
                    </div>
                    <div className="mono" style={{ fontSize: 10.5, color: "var(--ink-mute)", marginTop: 4, letterSpacing: "0.04em" }}>
                      {d.source} · {d.created}
                    </div>
                  </div>
                  <Tag kind={d.lang.toLowerCase()}>{d.lang}</Tag>
                </div>

                <div style={{ marginTop: 14, marginBottom: 8 }}>
                  <CoverageBar known={d.known} learning={d.learning} neww={d.new} height={6} />
                </div>

                <div style={{ display: "flex", justifyContent: "space-between", fontSize: 11, fontFamily: "var(--font-mono)", color: "var(--ink-mute)", marginBottom: 14 }}>
                  <span><b style={{ color: "var(--accent-2)" }}>{d.known}</b> known</span>
                  <span><b style={{ color: "var(--accent-3)" }}>{d.learning}</b> learning</span>
                  <span><b style={{ color: "var(--accent)" }}>{d.new}</b> new</span>
                  <span>cov <b style={{ color: "var(--ink)" }}>{Math.round(d.coverage * 100)}%</b></span>
                </div>

                <HeatStrip coverage={d.coverage} segments={50} accent={d.lang === "FI" ? "var(--fi)" : "var(--et)"} />

                <div style={{ display: "flex", gap: 6, marginTop: 14 }}>
                  <button className="btn" style={{ flex: 1, justifyContent: "center" }}>
                    Review · <b className="num" style={{ color: "var(--accent)", marginLeft: 4 }}>{Math.min(d.learning, 24)}</b>
                  </button>
                  <button className="btn">Open</button>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}

window.LibraryView = LibraryView;
