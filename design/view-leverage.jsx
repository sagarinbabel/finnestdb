/* global React, FED_DATA, Tag, Spark, HeatStrip */
const { useState: useStateLv } = React;

function LeverageView() {
  const [scope, setScope] = useStateLv("all");
  const rank = FED_DATA.LEVERAGE_RANK;
  const maxTokens = Math.max(...rank.map(r => r.tokens));
  const totalUnlock = rank.slice(0, 10).reduce((a, r) => a + r.unlock, 0);

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Leverage</h1>
          <div className="sub">
            Highest-impact words across your library — learn these first to maximize comprehension.
          </div>
        </div>
        <div className="actions">
          <button className="btn">Add top 10 to queue</button>
          <button className="btn btn-primary">Export ranked list</button>
        </div>
      </div>

      <div className="page-body">
        {/* Hero stats */}
        <div style={{ display: "grid", gridTemplateColumns: "1.4fr 1fr 1fr", gap: 16, marginBottom: 24 }}>
          <div className="panel" style={{ padding: 22 }}>
            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 10 }}>
              If you learn the top 10 leverage words
            </div>
            <div style={{ display: "flex", alignItems: "baseline", gap: 16 }}>
              <div className="disp num" style={{ fontSize: 64, fontWeight: 700, color: "var(--accent)", letterSpacing: "-0.03em", lineHeight: 1 }}>
                +{Math.round(totalUnlock * 100)}<span style={{ fontSize: 32 }}>%</span>
              </div>
              <div style={{ flex: 1 }}>
                <div className="disp" style={{ fontSize: 18, lineHeight: 1.3, color: "var(--ink-soft)" }}>
                  comprehension gain<br/>
                  <span style={{ color: "var(--ink-mute)", fontSize: 14 }}>across all 5 active decks</span>
                </div>
              </div>
            </div>
            <div style={{ marginTop: 18 }}>
              <HeatStrip coverage={0.82} segments={120} accent="var(--accent)" />
            </div>
          </div>

          <div className="panel" style={{ padding: 22 }}>
            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 10 }}>
              Known · 30d trend
            </div>
            <div className="disp num" style={{ fontSize: 38, fontWeight: 600, lineHeight: 1, letterSpacing: "-0.02em" }}>
              {FED_DATA.KNOWN_SERIES[FED_DATA.KNOWN_SERIES.length - 1].toLocaleString()}
            </div>
            <div className="mono" style={{ fontSize: 11, color: "var(--accent-2)", marginTop: 6 }}>
              +{FED_DATA.KNOWN_SERIES[FED_DATA.KNOWN_SERIES.length - 1] - FED_DATA.KNOWN_SERIES[0]} in 30d
            </div>
            <div style={{ marginTop: 14 }}>
              <Spark data={FED_DATA.KNOWN_SERIES} color="var(--accent-2)" width={260} height={56} />
            </div>
          </div>

          <div className="panel" style={{ padding: 22 }}>
            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 10 }}>
              Today
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <span style={{ color: "var(--ink-mute)" }}>Due now</span>
                <span className="mono num" style={{ fontSize: 18, fontWeight: 600, color: "var(--accent)" }}>{FED_DATA.TODAY_QUEUE.due}</span>
              </div>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <span style={{ color: "var(--ink-mute)" }}>New capacity</span>
                <span className="mono num" style={{ fontSize: 18, fontWeight: 600 }}>{FED_DATA.TODAY_QUEUE.new_capacity}</span>
              </div>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <span style={{ color: "var(--ink-mute)" }}>Streak</span>
                <span className="mono num" style={{ fontSize: 18, fontWeight: 600, color: "var(--accent-2)" }}>{FED_DATA.TODAY_QUEUE.streak_days}d</span>
              </div>
            </div>
          </div>
        </div>

        {/* Scope filter */}
        <div style={{ display: "flex", alignItems: "center", marginBottom: 12, gap: 12 }}>
          <span className="mono" style={{ fontSize: 10, letterSpacing: "0.14em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
            Scope
          </span>
          <div style={{ display: "flex", gap: 6 }}>
            {[
              { v: "all", l: "All decks" },
              { v: "active", l: "Active only" },
              { v: "fi", l: "Finnish" },
              { v: "et", l: "Estonian" },
            ].map(o => (
              <button key={o.v} onClick={() => setScope(o.v)} className="btn" style={{
                background: scope === o.v ? "var(--ink)" : "transparent",
                color: scope === o.v ? "var(--bg-deep)" : "var(--ink-soft)",
                borderColor: scope === o.v ? "var(--ink)" : "var(--line)",
                fontSize: 11.5,
              }}>{o.l}</button>
            ))}
          </div>
        </div>

        {/* Leverage table */}
        <div className="panel">
          <div className="panel-head">
            <span className="panel-title">Ranked by tokens unlocked across library</span>
          </div>
          <table className="data-table">
            <thead>
              <tr>
                <th style={{ width: 36, textAlign: "right" }}>#</th>
                <th></th>
                <th>Lemma</th>
                <th>Definition</th>
                <th>POS</th>
                <th style={{ width: 200 }}>Tokens unlocked</th>
                <th style={{ width: 120 }}>Decks</th>
                <th style={{ width: 80, textAlign: "right" }}>+%</th>
                <th style={{ width: 100, textAlign: "right" }}></th>
              </tr>
            </thead>
            <tbody>
              {rank.map((r, i) => (
                <tr key={r.lemma}>
                  <td className="mono num" style={{ textAlign: "right", color: "var(--ink-mute)" }}>{String(i+1).padStart(2,"0")}</td>
                  <td><span className={`dot ${r.status}`} /></td>
                  <td className="mono" style={{ fontWeight: 600, fontSize: 14 }}>{r.lemma}</td>
                  <td style={{ color: "var(--ink-soft)" }}>{r.gloss}</td>
                  <td><Tag>{r.pos}</Tag></td>
                  <td>
                    <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                      <div style={{ flex: 1, height: 6, background: "var(--bg-raise)", borderRadius: 1, overflow: "hidden" }}>
                        <div style={{
                          width: `${(r.tokens / maxTokens) * 100}%`,
                          height: "100%",
                          background: r.status === "known" ? "var(--accent-2)" : r.status === "learning" ? "var(--accent-3)" : "var(--accent)",
                        }} />
                      </div>
                      <span className="mono num" style={{ fontSize: 12, fontWeight: 600, minWidth: 44, textAlign: "right" }}>{r.tokens.toLocaleString()}</span>
                    </div>
                  </td>
                  <td>
                    <div style={{ display: "flex", gap: 3 }}>
                      {r.decks.map(d => {
                        const deck = FED_DATA.SAMPLE_DECKS.find(x => x.id === d);
                        return (
                          <span key={d} title={deck?.title} style={{
                            width: 16, height: 16,
                            background: deck?.lang === "ET" ? "var(--et)" : "var(--fi)",
                            borderRadius: 2,
                            opacity: 0.85,
                          }} />
                        );
                      })}
                      {Array.from({ length: 5 - r.decks.length }).map((_, j) => (
                        <span key={`e${j}`} style={{ width: 16, height: 16, background: "var(--bg-raise)", borderRadius: 2 }} />
                      ))}
                    </div>
                  </td>
                  <td className="mono num" style={{ textAlign: "right", color: "var(--accent)", fontWeight: 600 }}>
                    +{(r.unlock * 100).toFixed(1)}%
                  </td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn" style={{ fontSize: 11, padding: "3px 10px" }}>+ Queue</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

window.LeverageView = LeverageView;
