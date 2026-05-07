/* global React, FED_DATA */
const { useState: useStateD2 } = React;

function CoverageBarV2({ known, learning, neww, height = 6 }) {
  const total = known + learning + neww;
  return (
    <div style={{ display: "flex", height, borderRadius: 2, overflow: "hidden", background: "var(--bg-raise)", border: "1px solid var(--line)" }}>
      <div style={{ flex: known / total, background: "var(--known)" }} />
      <div style={{ flex: learning / total, background: "var(--learning)" }} />
      <div style={{ flex: neww / total, background: "var(--new)" }} />
    </div>
  );
}

function DecksView({ onOpenDeck, onReviewDeck, onReviewAll }) {
  const decks = FED_DATA.SAMPLE_DECKS;
  const totalDue = decks.reduce((a, d) => a + Math.min(d.learning, 24), 0);

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Your decks</h1>
          <div className="sub">{decks.length} decks · {totalDue} cards due across all</div>
        </div>
        <div className="actions">
          <button className="btn">Import .apkg</button>
          <button className="btn">Upload .epub</button>
          <button className="btn btn-primary" onClick={onReviewAll}>
            Review all · {totalDue}
          </button>
        </div>
      </div>

      <div className="page-body">
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(360px, 1fr))", gap: 16 }}>
          {decks.map(d => {
            const due = Math.min(d.learning, 24);
            return (
              <div key={d.id} className="panel" style={{ overflow: "hidden", cursor: "pointer" }}
                   onMouseEnter={e => e.currentTarget.style.borderColor = "var(--ink-mute)"}
                   onMouseLeave={e => e.currentTarget.style.borderColor = "var(--line)"}>
                <div style={{ height: 3, background: d.lang === "FI" ? "var(--fi)" : "var(--et)" }} />
                <div style={{ padding: "16px 18px 14px" }} onClick={() => onOpenDeck(d.id)}>
                  <div style={{ display: "flex", alignItems: "flex-start", gap: 10, marginBottom: 6 }}>
                    <div style={{ flex: 1 }}>
                      <div className="disp" style={{ fontSize: 19, fontWeight: 600, letterSpacing: "-0.01em", lineHeight: 1.2 }}>
                        {d.title}
                      </div>
                      <div className="mono" style={{ fontSize: 10.5, color: "var(--ink-mute)", marginTop: 4, letterSpacing: "0.04em" }}>
                        {d.created}
                      </div>
                    </div>
                    <span className={`tag ${d.lang.toLowerCase()}`}>{d.lang}</span>
                  </div>

                  <div style={{ marginTop: 14, marginBottom: 8 }}>
                    <CoverageBarV2 known={d.known} learning={d.learning} neww={d.new} />
                  </div>

                  <div style={{ display: "flex", justifyContent: "space-between", fontSize: 11, fontFamily: "var(--font-mono)", color: "var(--ink-mute)" }}>
                    <span><b style={{ color: "var(--known)" }}>{d.known}</b> known</span>
                    <span><b style={{ color: "var(--learning)" }}>{d.learning}</b> learning</span>
                    <span><b style={{ color: "var(--new)" }}>{d.new}</b> new</span>
                  </div>
                </div>
                <div style={{ borderTop: "1px solid var(--line)", padding: "10px 12px", display: "flex", gap: 6, background: "var(--bg-deep)" }}>
                  <button className="btn" style={{ flex: 1, justifyContent: "center" }} onClick={(e) => { e.stopPropagation(); onReviewDeck(d.id); }}>
                    Review {due > 0 && <b className="num" style={{ color: "var(--accent)", marginLeft: 4 }}>{due}</b>}
                  </button>
                  <button className="btn btn-ghost" onClick={(e) => { e.stopPropagation(); onOpenDeck(d.id); }}>Details</button>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </>
  );
}

function DeckDetailView({ deckId, onBack, onReview }) {
  const deck = FED_DATA.SAMPLE_DECKS.find(d => d.id === deckId) || FED_DATA.SAMPLE_DECKS[0];
  const sentences = FED_DATA.SAMPLE_PARSE_FI.sentences;
  const projected = Math.min(deck.coverage + 0.18, 0.97);

  return (
    <>
      <div className="page-head">
        <div>
          <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", marginBottom: 6, cursor: "pointer" }} onClick={onBack}>
            ← Decks
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <span className={`tag ${deck.lang.toLowerCase()}`}>{deck.lang}</span>
            <h1>{deck.title}</h1>
          </div>
          <div className="sub">{deck.source} · created {deck.created}</div>
        </div>
        <div className="actions">
          <button className="btn">Export</button>
          <button className="btn btn-primary" onClick={() => onReview && onReview(deck.id)}>Start review · {deck.learning}</button>
        </div>
      </div>

      <div className="page-body">
        <div className="panel" style={{ marginBottom: 16 }}>
          <div className="panel-head"><span className="panel-title">Comprehension</span></div>
          <div style={{ padding: 24, display: "grid", gridTemplateColumns: "1fr auto 1fr", gap: 24, alignItems: "center" }}>
            <div>
              <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 4 }}>Now</div>
              <div className="disp num" style={{ fontSize: 52, fontWeight: 600, lineHeight: 1, letterSpacing: "-0.03em" }}>
                {Math.round(deck.coverage * 100)}<span style={{ fontSize: 26, color: "var(--ink-mute)" }}>%</span>
              </div>
              <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", marginTop: 6 }}>
                {deck.known.toLocaleString()} of {deck.unique.toLocaleString()} lemmas known
              </div>
            </div>
            <div style={{ width: 80, height: 1, background: "var(--accent)", position: "relative" }}>
              <div style={{ position: "absolute", right: -1, top: -3, width: 0, height: 0, borderLeft: "6px solid var(--accent)", borderTop: "3.5px solid transparent", borderBottom: "3.5px solid transparent" }} />
              <div className="mono" style={{ position: "absolute", left: "50%", transform: "translateX(-50%)", top: -22, fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.1em", textTransform: "uppercase", whiteSpace: "nowrap" }}>
                +{Math.round((projected - deck.coverage) * 100)}%
              </div>
            </div>
            <div>
              <div className="mono" style={{ fontSize: 10, color: "var(--accent)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 4 }}>If you learn top 100</div>
              <div className="disp num" style={{ fontSize: 52, fontWeight: 600, lineHeight: 1, letterSpacing: "-0.03em", color: "var(--accent)" }}>
                {Math.round(projected * 100)}<span style={{ fontSize: 26 }}>%</span>
              </div>
            </div>
          </div>
        </div>

        <div className="panel">
          <div className="panel-head"><span className="panel-title">Sentences · {sentences.length}</span></div>
          <div style={{ padding: "8px 0" }}>
            {sentences.map((s, i) => (
              <div key={s.id} style={{
                padding: "10px 18px",
                borderBottom: i < sentences.length - 1 ? "1px solid var(--line-soft)" : "none",
                display: "flex", gap: 14, alignItems: "baseline",
              }}>
                <span className="mono num" style={{ fontSize: 11, color: "var(--ink-mute)", flex: "none", width: 28 }}>{String(i+1).padStart(2,"0")}</span>
                <span className="disp" style={{ fontSize: 16, lineHeight: 1.5 }}>{s.text}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </>
  );
}

window.DecksView = DecksView;
window.DeckDetailView = DeckDetailView;
