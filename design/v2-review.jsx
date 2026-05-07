/* global React, FED_DATA */
const { useState: useStateRv, useEffect: useEffectRv } = React;

function ReviewView({ scope, onExit, onScopeChange }) {
  // FSRS-shaped queue. scope = "all" | deckId
  const [revealed, setRevealed] = useStateRv(false);
  const [idx, setIdx] = useStateRv(0);
  const [pickerOpen, setPickerOpen] = useStateRv(false);

  const isAll = scope === "all";
  const deck = isAll ? null : FED_DATA.SAMPLE_DECKS.find(d => d.id === scope);
  const total = isAll ? 87 : Math.min(deck?.learning || 12, 24);
  const sourceLabel = isAll ? "All decks" : deck?.title;
  const sourceLang = isAll ? null : deck?.lang;

  // Mock card stream
  const cards = [
    { lemma: "toissapäivä", gloss: "the day before yesterday", grammar: "Essive sg (-nä)", pos: "NOUN",
      sentence: "Toissapäivänä menin pankkiin.", highlight: "Toissapäivänä", deck: "Konsta Punkkinen", lang: "FI",
      morph: [{ stem: "toissapäivä", suffix: "nä" }],
      examples: [
        { fi: "Toissapäivänä satoi koko päivän.", en: "The day before yesterday it rained all day.", source: "Konsta Punkkinen · ch. 4" },
        { fi: "Olen ollut sairaana toissapäivästä asti.", en: "I've been sick since the day before yesterday.", source: "HS · health column" },
      ],
      fsrs: { stability: 0.8, difficulty: 4.6, retrievability: 0.0, intervals: { again: "<1m", hard: "8m", good: "1d", easy: "4d" } } },
    { lemma: "pankki", gloss: "bank", grammar: "Illative sg (-iin)", pos: "NOUN",
      sentence: "Toissapäivänä menin pankkiin.", highlight: "pankkiin", deck: "Konsta Punkkinen", lang: "FI",
      morph: [{ stem: "pankki", suffix: "in" }],
      examples: [
        { fi: "Suomen suurin pankki sulki 30 konttoria.", en: "Finland's largest bank closed 30 branches.", source: "YLE Uutiset · 2024" },
        { fi: "Mene pankkiin ja kysy lainasta.", en: "Go to the bank and ask about a loan.", source: "Suomen mestari · 2" },
      ],
      fsrs: { stability: 4.1, difficulty: 3.2, retrievability: 0.62, intervals: { again: "1m", hard: "1d", good: "4d", easy: "12d" } } },
    { lemma: "raamatukogu", gloss: "library", grammar: "Inessive sg", pos: "NOUN",
      sentence: "Eile käisin raamatukogus.", highlight: "raamatukogus", deck: "Postimees", lang: "ET",
      morph: [{ stem: "raamatukogu", suffix: "s" }],
      examples: [
        { fi: "Tartu Ülikooli raamatukogu on suletud pühapäeval.", en: "The Tartu University library is closed on Sunday.", source: "Postimees · 2025-01" },
        { fi: "Lapsed armastavad raamatukogu lugemisnurka.", en: "Children love the library's reading corner.", source: "ERR · kultuur" },
      ],
      fsrs: { stability: 1.2, difficulty: 5.1, retrievability: 0.0, intervals: { again: "<1m", hard: "10m", good: "1d", easy: "3d" } } },
  ];
  const c = cards[idx % cards.length];

  function next() { setRevealed(false); setIdx(i => i + 1); }

  useEffectRv(() => {
    const onKey = (e) => {
      if (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA") return;
      if (e.key === " ") { e.preventDefault(); setRevealed(true); }
      else if (revealed && ["1","2","3","4"].includes(e.key)) { next(); }
      else if (e.key === "Escape") onExit();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [revealed]);

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Review</h1>
          <div className="sub" style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
            <span>Reviewing</span>
            <button
              type="button"
              className="scope-pill"
              onClick={() => setPickerOpen(o => !o)}
              style={{
                all: "unset", cursor: "pointer",
                display: "inline-flex", alignItems: "center", gap: 6,
                padding: "3px 10px",
                border: "1px solid var(--line)",
                borderRadius: 3,
                background: "var(--bg-deep)",
                color: "var(--ink)",
                fontFamily: "var(--font-mono)",
                fontSize: 12,
                fontWeight: 500,
              }}>
              {sourceLang && <span className={`tag ${sourceLang.toLowerCase()}`} style={{ padding: "1px 5px", fontSize: 9 }}>{sourceLang}</span>}
              <span>{sourceLabel}</span>
              <span style={{ color: "var(--ink-mute)", marginLeft: 2 }}>▾</span>
            </button>
            <span style={{ color: "var(--ink-mute)" }}>·</span>
            <span>FSRS · {idx + 1} of {total} · est {Math.round((total - idx) * 0.3)} min</span>

            {pickerOpen && (
              <>
                <div
                  onClick={() => setPickerOpen(false)}
                  style={{ position: "fixed", inset: 0, zIndex: 50 }}
                />
                <div style={{
                  position: "absolute",
                  top: 70, left: 0,
                  zIndex: 51,
                  minWidth: 320,
                  background: "var(--bg)",
                  border: "1px solid var(--line)",
                  borderRadius: 6,
                  boxShadow: "0 12px 32px oklch(0% 0 0 / 0.35)",
                  padding: 6,
                }}>
                  <div className="mono" style={{ fontSize: 9.5, color: "var(--ink-mute)", letterSpacing: "0.16em", textTransform: "uppercase", padding: "8px 10px 4px" }}>
                    Switch scope
                  </div>
                  <button
                    onClick={() => { onScopeChange && onScopeChange("all"); setPickerOpen(false); setIdx(0); setRevealed(false); }}
                    style={{
                      all: "unset", cursor: "pointer", display: "flex", alignItems: "center", gap: 10,
                      padding: "8px 10px", borderRadius: 4, width: "100%", boxSizing: "border-box",
                      background: isAll ? "var(--bg-deep)" : "transparent",
                    }}
                    onMouseEnter={e => e.currentTarget.style.background = "var(--bg-deep)"}
                    onMouseLeave={e => e.currentTarget.style.background = isAll ? "var(--bg-deep)" : "transparent"}>
                    <span style={{ fontWeight: 500, flex: 1 }}>All decks</span>
                    <span className="mono num" style={{ fontSize: 11, color: "var(--accent)", fontWeight: 600 }}>87 due</span>
                    {isAll && <span className="mono" style={{ color: "var(--accent)", fontSize: 11 }}>✓</span>}
                  </button>
                  <div style={{ height: 1, background: "var(--line-soft)", margin: "4px 8px" }} />
                  {FED_DATA.SAMPLE_DECKS.map(d => (
                    <button key={d.id}
                      onClick={() => { onScopeChange && onScopeChange(d.id); setPickerOpen(false); setIdx(0); setRevealed(false); }}
                      style={{
                        all: "unset", cursor: "pointer", display: "flex", alignItems: "center", gap: 10,
                        padding: "8px 10px", borderRadius: 4, width: "100%", boxSizing: "border-box",
                        background: scope === d.id ? "var(--bg-deep)" : "transparent",
                      }}
                      onMouseEnter={e => e.currentTarget.style.background = "var(--bg-deep)"}
                      onMouseLeave={e => e.currentTarget.style.background = scope === d.id ? "var(--bg-deep)" : "transparent"}>
                      <span className={`tag ${d.lang.toLowerCase()}`} style={{ padding: "1px 5px", fontSize: 9 }}>{d.lang}</span>
                      <span style={{ flex: 1, fontWeight: 500 }}>{d.title}</span>
                      <span className="mono num" style={{ fontSize: 11, color: "var(--accent)", fontWeight: 600 }}>{Math.min(d.learning, 24)} due</span>
                      {scope === d.id && <span className="mono" style={{ color: "var(--accent)", fontSize: 11 }}>✓</span>}
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>
        </div>
        <div className="actions">
          <button className="btn">Pause</button>
          <button className="btn" onClick={onExit}>Exit · <span className="kbd">Esc</span></button>
        </div>
      </div>

      <div className="page-body" style={{ maxWidth: 820, margin: "0 auto" }}>
        {/* Progress strip */}
        <div style={{ display: "flex", gap: 2, height: 4, marginBottom: 24 }}>
          {Array.from({ length: total }).map((_, i) => (
            <div key={i} style={{
              flex: 1,
              background: i < idx ? "var(--known)" : i === idx ? "var(--accent)" : "var(--bg-raise)",
              borderRadius: 1,
            }} />
          ))}
        </div>

        {/* Card */}
        <div className="panel" style={{ padding: "48px 48px 36px", minHeight: 420, display: "flex", flexDirection: "column" }}>
          <div style={{ display: "flex", gap: 6, marginBottom: 28 }}>
            <span className={`tag ${c.lang.toLowerCase()}`}>{c.lang}</span>
            <span className="tag">{c.pos}</span>
            <span className={`tag ${c.fsrs.retrievability === 0 ? "new" : "learning"}`}>
              {c.fsrs.retrievability === 0 ? "NEW" : "DUE"}
            </span>
            <div style={{ flex: 1 }} />
            <span className="mono" style={{ fontSize: 11, color: "var(--ink-mute)" }}>{c.deck}</span>
          </div>

          <div style={{ flex: 1, display: "flex", flexDirection: "column", justifyContent: "center", alignItems: "center", textAlign: "center" }}>
            <div className="disp" style={{ fontSize: 36, lineHeight: 1.3, letterSpacing: "-0.02em", maxWidth: 700 }}>
              {(() => {
                const t = c.sentence; const i = t.indexOf(c.highlight);
                if (i === -1) return t;
                return <>{t.slice(0, i)}<span className="hl">{c.highlight}</span>{t.slice(i + c.highlight.length)}</>;
              })()}
            </div>

            {revealed && (
              <div style={{ borderTop: "1px solid var(--line)", paddingTop: 28, marginTop: 32, width: "100%", maxWidth: 540 }}>
                <div className="disp" style={{ fontSize: 40, fontWeight: 600, color: "var(--accent)", letterSpacing: "-0.02em", lineHeight: 1.05 }}>
                  {c.gloss}
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

                {c.examples && c.examples.length > 0 && (
                  <div style={{ marginTop: 28, textAlign: "left" }}>
                    <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 8, textAlign: "center" }}>
                      Other examples
                    </div>
                    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                      {c.examples.map((ex, i) => {
                        const lemma = c.lemma; const txt = ex.fi;
                        const re = new RegExp(`(${lemma}\\w*)`, "i");
                        const m = txt.match(re);
                        const parts = m ? [txt.slice(0, m.index), m[0], txt.slice(m.index + m[0].length)] : [txt];
                        return (
                          <div key={i} style={{
                            padding: "10px 14px",
                            background: "var(--bg-deep)",
                            border: "1px solid var(--line)",
                            borderRadius: 4,
                          }}>
                            <div className="disp" style={{ fontSize: 16, lineHeight: 1.45, color: "var(--ink)" }}>
                              {parts.length === 3
                                ? <>{parts[0]}<span style={{ color: "var(--accent)", fontWeight: 600 }}>{parts[1]}</span>{parts[2]}</>
                                : parts[0]}
                            </div>
                            <div style={{ fontSize: 13, color: "var(--ink-soft)", marginTop: 4, lineHeight: 1.4 }}>
                              {ex.en}
                            </div>
                            <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", marginTop: 4, letterSpacing: "0.04em" }}>
                              {ex.source}
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        {!revealed ? (
          <button className="btn btn-primary" style={{ width: "100%", padding: "14px", justifyContent: "center", fontSize: 14, marginTop: 16 }} onClick={() => setRevealed(true)}>
            Show answer · <span className="kbd">Space</span>
          </button>
        ) : (
          <>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 8, marginTop: 16 }}>
              {[
                { k: "Again", t: c.fsrs.intervals.again, c: "oklch(0.62 0.22 28)", n: "1" },
                { k: "Hard",  t: c.fsrs.intervals.hard,  c: "var(--learning)",     n: "2" },
                { k: "Good",  t: c.fsrs.intervals.good,  c: "var(--known)",        n: "3" },
                { k: "Easy",  t: c.fsrs.intervals.easy,  c: "var(--accent)",       n: "4" },
              ].map(b => (
                <button key={b.k} className="btn" onClick={next} style={{
                  flexDirection: "column", padding: "14px 12px",
                  borderColor: b.c, color: b.c, fontWeight: 600, fontSize: 14, gap: 4, alignItems: "stretch",
                }}>
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                    <span>{b.k}</span>
                    <span className="kbd" style={{ borderColor: "currentColor" }}>{b.n}</span>
                  </div>
                  <div className="mono" style={{ fontSize: 11, opacity: 0.7, fontWeight: 400, textAlign: "left" }}>next · {b.t}</div>
                </button>
              ))}
            </div>
            <div className="mono" style={{ fontSize: 10.5, color: "var(--ink-mute)", marginTop: 12, textAlign: "center", letterSpacing: "0.06em" }}>
              FSRS · stability {c.fsrs.stability.toFixed(1)}d · difficulty {c.fsrs.difficulty.toFixed(1)} · retrievability {Math.round(c.fsrs.retrievability * 100)}%
            </div>
          </>
        )}
      </div>
    </>
  );
}

window.ReviewViewV2 = ReviewView;
