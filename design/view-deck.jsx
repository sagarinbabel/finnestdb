/* global React, FED_DATA, Tag, CoverageBar, HeatStrip, Sentence */
const { useState: useStateD } = React;

function DeckView({ deckId, onBack }) {
  const deck = FED_DATA.SAMPLE_DECKS.find(d => d.id === deckId) || FED_DATA.SAMPLE_DECKS[0];
  const words = FED_DATA.SAMPLE_PARSE_FI.words;
  const sentences = FED_DATA.SAMPLE_PARSE_FI.sentences;

  return (
    <>
      <div className="page-head">
        <div>
          <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", letterSpacing: "0.06em", marginBottom: 6, cursor: "pointer" }} onClick={onBack}>
            ← Library
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <Tag kind={deck.lang.toLowerCase()}>{deck.lang}</Tag>
            <h1>{deck.title}</h1>
          </div>
          <div className="sub">{deck.source} · created {deck.created}</div>
        </div>
        <div className="actions">
          <button className="btn">Edit</button>
          <button className="btn">Export</button>
          <button className="btn btn-primary">Start review · {deck.learning}</button>
        </div>
      </div>

      <div className="page-body" style={{ display: "grid", gridTemplateColumns: "1fr 320px", gap: 20 }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 16, minWidth: 0 }}>

          {/* Comprehension projection — Surasura-inspired */}
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">Comprehension projection</span>
            </div>
            <div style={{ padding: 20, display: "grid", gridTemplateColumns: "1fr auto 1fr", gap: 24, alignItems: "center" }}>
              <div>
                <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 4 }}>Now</div>
                <div className="disp num" style={{ fontSize: 52, fontWeight: 600, lineHeight: 1, letterSpacing: "-0.03em" }}>
                  {Math.round(deck.coverage * 100)}<span style={{ fontSize: 26, color: "var(--ink-mute)" }}>%</span>
                </div>
                <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", marginTop: 6 }}>
                  {deck.known.toLocaleString()} known of {deck.unique.toLocaleString()} lemmas
                </div>
              </div>
              <div style={{ width: 60, height: 1, background: "var(--accent)", position: "relative" }}>
                <div style={{ position: "absolute", right: -1, top: -3, width: 0, height: 0, borderLeft: "6px solid var(--accent)", borderTop: "3.5px solid transparent", borderBottom: "3.5px solid transparent" }} />
                <div className="mono" style={{ position: "absolute", left: "50%", transform: "translateX(-50%)", top: -22, fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.1em", textTransform: "uppercase", whiteSpace: "nowrap" }}>
                  +{Math.round((Math.min(deck.coverage + 0.18, 0.97) - deck.coverage) * 100)}%
                </div>
              </div>
              <div>
                <div className="mono" style={{ fontSize: 10, color: "var(--accent)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 4 }}>If you learn top 100</div>
                <div className="disp num" style={{ fontSize: 52, fontWeight: 600, lineHeight: 1, letterSpacing: "-0.03em", color: "var(--accent)" }}>
                  {Math.round(Math.min(deck.coverage + 0.18, 0.97) * 100)}<span style={{ fontSize: 26 }}>%</span>
                </div>
                <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", marginTop: 6 }}>
                  ~{deck.leverage_top} more tokens unlocked
                </div>
              </div>
            </div>
            <div style={{ padding: "12px 16px 16px" }}>
              <HeatStrip coverage={deck.coverage} segments={120} accent="var(--accent-2)" />
            </div>
          </div>

          {/* Words table */}
          <div className="panel">
            <div className="panel-head">
              <span className="panel-title">Words · {deck.unique.toLocaleString()}</span>
              <Tag kind="known">{deck.known} known</Tag>
              <Tag kind="learning">{deck.learning} learning</Tag>
              <Tag kind="new">{deck.new} new</Tag>
            </div>
            <div style={{ maxHeight: 420, overflow: "auto" }}>
              <table className="data-table">
                <thead>
                  <tr>
                    <th></th>
                    <th>Lemma</th>
                    <th>Definition</th>
                    <th>Grammar</th>
                    <th style={{ textAlign: "right" }}>×</th>
                    <th style={{ textAlign: "right" }}>Lever</th>
                  </tr>
                </thead>
                <tbody>
                  {words.slice(0, 12).map(w => (
                    <tr key={w.lemma}>
                      <td><span className={`dot ${w.status}`} /></td>
                      <td className="mono" style={{ fontWeight: 500 }}>{w.lemma.length > 28 ? w.lemma.slice(0, 26) + "…" : w.lemma}</td>
                      <td style={{ color: "var(--ink-soft)" }}>{w.gloss}</td>
                      <td className="mono" style={{ fontSize: 11, color: "var(--ink-mute)" }}>{w.grammar}</td>
                      <td className="mono num" style={{ textAlign: "right" }}>{w.count}</td>
                      <td className="mono num" style={{ textAlign: "right", color: w.leverage > 100 ? "var(--accent)" : "var(--ink-mute)" }}>{w.leverage}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Sentences */}
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

        {/* Right rail */}
        <div style={{ display: "flex", flexDirection: "column", gap: 16, position: "sticky", top: 16, alignSelf: "start" }}>
          <div className="panel" style={{ padding: 18 }}>
            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 14 }}>
              Deck stats
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
              {[
                { l: "Tokens", v: deck.tokens.toLocaleString() },
                { l: "Unique lemmas", v: deck.unique.toLocaleString() },
                { l: "Sentences", v: "1,204" },
                { l: "MWEs detected", v: "23" },
                { l: "Parse time", v: "1.8s" },
                { l: "Avg word length", v: "8.4 chars" },
              ].map(r => (
                <div key={r.l} style={{ display: "flex", justifyContent: "space-between", fontSize: 13, paddingBottom: 8, borderBottom: "1px dashed var(--line-soft)" }}>
                  <span style={{ color: "var(--ink-mute)" }}>{r.l}</span>
                  <span className="mono num" style={{ fontWeight: 500 }}>{r.v}</span>
                </div>
              ))}
            </div>
          </div>

          <div className="panel" style={{ padding: 18 }}>
            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 12 }}>
              Top leverage in deck
            </div>
            {FED_DATA.LEVERAGE_RANK.slice(0, 6).map(w => (
              <div key={w.lemma} style={{ display: "flex", alignItems: "center", gap: 10, padding: "8px 0", borderBottom: "1px dashed var(--line-soft)" }}>
                <span className={`dot ${w.status}`} />
                <span className="mono" style={{ fontWeight: 500 }}>{w.lemma}</span>
                <span style={{ color: "var(--ink-mute)", fontSize: 12, flex: 1 }}>{w.gloss}</span>
                <span className="mono num" style={{ fontSize: 11, color: "var(--accent)", fontWeight: 600 }}>{w.tokens}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </>
  );
}

window.DeckView = DeckView;
