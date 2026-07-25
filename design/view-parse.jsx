/* global React, FED_DATA, Sentence, Tag, StatusDot, Segmented */
const { useState: usePS, useMemo: useMPS, useRef: useRPS, useEffect: useEPS } = React;

function ParseView() {
  const D = window.FED_DATA;
  const [text, setText] = usePS(D.FI_TEXT_1);
  const [lang, setLang] = usePS("FI");
  const [parser, setParser] = usePS("custom");
  const [parsed, setParsed] = usePS(D.SAMPLE_PARSE_FI);
  const [parsing, setParsing] = usePS(false);
  const [selectedLemma, setSelectedLemma] = usePS("toissapäivä");
  const [sortBy, setSortBy] = usePS("order");
  const [filter, setFilter] = usePS("all");
  const [showText, setShowText] = usePS(true);

  function runParse() {
    setParsing(true);
    setTimeout(() => {
      // For demo, always return canned data; in real app this would call /api/parse
      setParsed({ ...D.SAMPLE_PARSE_FI, parser, language: lang });
      setParsing(false);
    }, 380);
  }

  // Detect language by Estonian õ
  const detectedLang = useMPS(() => {
    if (/õ/.test(text)) return "ET";
    const letters = text.match(/[a-zäöõü]/gi) || [];
    const ao = (text.match(/[äö]/g) || []).length;
    if (letters.length > 0 && ao / letters.length > 0.015) return "FI";
    return null;
  }, [text]);

  const selectedWord = parsed.words.find(w => w.lemma === selectedLemma) || parsed.words[0];

  const filteredWords = useMPS(() => {
    let ws = [...parsed.words];
    if (filter !== "all") ws = ws.filter(w => filter === "mwe" ? w.pos === "MWE" : w.status === filter);
    if (sortBy === "leverage") ws.sort((a, b) => b.leverage - a.leverage);
    else if (sortBy === "alpha") ws.sort((a, b) => a.lemma.localeCompare(b.lemma));
    else if (sortBy === "count") ws.sort((a, b) => b.count - a.count);
    return ws;
  }, [parsed, filter, sortBy]);

  // Word→sentence-rendered text with highlights for current selection
  function renderText() {
    return parsed.sentences.map(s => {
      // Find words mapped to this sentence
      const wordsHere = parsed.words.filter(w => w.sentence_id === s.id);
      let html = s.text;
      // Build replacements
      const replacements = [];
      wordsHere.forEach(w => {
        w.forms.forEach(f => {
          // Truncated MWE form
          const formStr = f.replace("…", "");
          const idx = html.indexOf(formStr);
          if (idx >= 0 && formStr.length > 1) {
            replacements.push({ idx, len: formStr.length, word: w });
          }
        });
      });
      // Sort by index, render
      replacements.sort((a, b) => a.idx - b.idx);
      const parts = [];
      let cursor = 0;
      replacements.forEach((r, i) => {
        if (r.idx < cursor) return;
        parts.push(html.slice(cursor, r.idx));
        const isSelected = r.word.lemma === selectedLemma;
        const status = r.word.pos === "MWE" ? "mwe" : r.word.status;
        parts.push(
          <span
            key={`${s.id}-${i}`}
            className={`hl ${status}`}
            onClick={() => setSelectedLemma(r.word.lemma)}
            style={{
              cursor: "pointer",
              outline: isSelected ? "2px solid var(--accent)" : "none",
              outlineOffset: 2,
            }}
            title={`${r.word.lemma} · ${r.word.gloss}`}
          >
            {html.slice(r.idx, r.idx + r.len)}
          </span>
        );
        cursor = r.idx + r.len;
      });
      parts.push(html.slice(cursor));
      return (
        <span key={s.id}>
          {parts}{" "}
        </span>
      );
    });
  }

  return (
    <div>
      {/* Page head */}
      <div className="page-head">
        <div>
          <h1 className="disp">Parse inspector</h1>
          <div className="sub mono">parse · resolve · inspect - Finnish & Estonian</div>
        </div>
        <div className="actions row gap-8">
          <Segmented
            value={lang}
            onChange={setLang}
            options={[{ value: "FI", label: "Finnish" }, { value: "ET", label: "Estonian" }]}
          />
          <Segmented
            value={parser}
            onChange={setParser}
            options={[
              { value: "basic", label: "Basic" },
              { value: "custom", label: "Custom" },
              { value: "omorfi", label: "Omorfi" },
            ]}
          />
          <button className="btn" onClick={() => setShowText(s => !s)}>
            {showText ? "Hide source" : "Show source"}
          </button>
          <button className="btn btn-primary" onClick={runParse}>
            {parsing ? "Parsing…" : "Parse"} <span className="kbd">⌘↵</span>
          </button>
        </div>
      </div>

      {/* Body */}
      <div className="page-body" style={{ display: "grid", gridTemplateColumns: "1fr 380px", gap: 20, padding: "20px 32px 60px" }}>
        {/* LEFT - input + results */}
        <div style={{ display: "flex", flexDirection: "column", gap: 16, minWidth: 0 }}>
          {showText && (
            <div className="panel">
              <div className="panel-head" style={{ justifyContent: "space-between" }}>
                <div className="row gap-8">
                  <span className="panel-title">Source text</span>
                  <Tag kind={lang.toLowerCase()}>{lang}</Tag>
                  {detectedLang && detectedLang !== lang && (
                    <span className="mono" style={{ color: "var(--accent-3)", fontSize: 11 }}>
                      ⚠ detected {detectedLang}
                    </span>
                  )}
                </div>
                <span className="mono" style={{ fontSize: 11, color: "var(--ink-mute)" }}>
                  {text.length.toLocaleString()} / 300,000 chars
                </span>
              </div>
              <textarea
                value={text}
                onChange={e => setText(e.target.value)}
                style={{
                  width: "100%",
                  minHeight: 130,
                  background: "transparent",
                  border: "none",
                  resize: "vertical",
                  padding: 16,
                  color: "var(--ink)",
                  fontFamily: "var(--font-mono)",
                  fontSize: 13.5,
                  lineHeight: 1.6,
                  outline: "none",
                }}
              />
            </div>
          )}

          {/* Annotated rendering */}
          <div className="panel">
            <div className="panel-head" style={{ justifyContent: "space-between" }}>
              <span className="panel-title">Annotated · click any word</span>
              <div className="row gap-12 mono" style={{ fontSize: 10.5, color: "var(--ink-mute)" }}>
                <span><span className="dot known" /> known</span>
                <span><span className="dot learning" /> learning</span>
                <span><span className="dot new" /> new</span>
                <span style={{ color: "var(--accent-4)" }}>━ MWE</span>
              </div>
            </div>
            <div style={{ padding: 24, fontFamily: "var(--font-disp)", fontSize: 19, lineHeight: 1.7, fontWeight: 400 }}>
              {renderText()}
            </div>

            {/* Stats footer */}
            <div className="row" style={{ padding: "10px 16px", borderTop: "1px solid var(--line)", justifyContent: "space-between", fontFamily: "var(--font-mono)", fontSize: 11, color: "var(--ink-mute)" }}>
              <div className="row gap-16">
                <span>parser <b style={{ color: "var(--ink)" }}>{parsed.parser}</b></span>
                <span>tokens <b style={{ color: "var(--ink)" }} className="num">{parsed.total_tokens}</b></span>
                <span>lemmas <b style={{ color: "var(--ink)" }} className="num">{parsed.unique_lemmas}</b></span>
                <span>coverage <b style={{ color: "var(--accent-2)" }} className="num">{(parsed.coverage * 100).toFixed(0)}%</b></span>
              </div>
              <span>↳ {parsed.duration_ms}ms</span>
            </div>
          </div>

          {/* Word table */}
          <div className="panel">
            <div className="panel-head" style={{ justifyContent: "space-between" }}>
              <div className="row gap-12">
                <span className="panel-title">Words · {filteredWords.length}</span>
                <Segmented
                  value={filter}
                  onChange={setFilter}
                  options={[
                    { value: "all", label: "All" },
                    { value: "new", label: "New" },
                    { value: "learning", label: "Learning" },
                    { value: "known", label: "Known" },
                    { value: "mwe", label: "MWE" },
                  ]}
                />
              </div>
              <Segmented
                value={sortBy}
                onChange={setSortBy}
                options={[
                  { value: "order", label: "Order" },
                  { value: "leverage", label: "Leverage" },
                  { value: "count", label: "Count" },
                  { value: "alpha", label: "A–Z" },
                ]}
              />
            </div>
            <div style={{ maxHeight: 420, overflow: "auto" }}>
              <table className="data-table">
                <thead>
                  <tr>
                    <th style={{ width: 24 }}></th>
                    <th>Lemma</th>
                    <th>POS</th>
                    <th>Forms</th>
                    <th>Definition</th>
                    <th>Grammar</th>
                    <th style={{ textAlign: "right" }}>Tokens</th>
                    <th style={{ textAlign: "right" }}>Leverage</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredWords.map((w, i) => {
                    const sel = w.lemma === selectedLemma;
                    return (
                      <tr key={w.lemma} className={sel ? "selected" : ""} onClick={() => setSelectedLemma(w.lemma)} style={{ cursor: "pointer" }}>
                        <td><StatusDot status={w.status === "new" || w.pos === "MWE" ? (w.pos === "MWE" ? "" : "new") : w.status} /></td>
                        <td className="mono" style={{ fontWeight: 600, color: "var(--ink)" }}>
                          {w.lemma.length > 28 ? w.lemma.slice(0, 26) + "…" : w.lemma}
                        </td>
                        <td><Tag kind={w.pos === "MWE" ? "mwe" : ""}>{w.pos}</Tag></td>
                        <td className="mono" style={{ color: "var(--ink-soft)", fontSize: 12 }}>
                          {w.forms.join(", ")}
                        </td>
                        <td style={{ color: "var(--ink-soft)" }}>{w.gloss}</td>
                        <td className="mono" style={{ fontSize: 11.5, color: "var(--ink-mute)" }}>{w.grammar}</td>
                        <td className="mono num" style={{ textAlign: "right" }}>{w.count}</td>
                        <td className="mono num" style={{ textAlign: "right", color: w.leverage > 100 ? "var(--accent-2)" : "var(--ink-mute)" }}>
                          {w.leverage}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        </div>

        {/* RIGHT - inspector */}
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <WordInspector word={selectedWord} sentences={parsed.sentences} />
          <ParseStatsPanel parsed={parsed} />
        </div>
      </div>
    </div>
  );
}

function WordInspector({ word, sentences }) {
  if (!word) return null;
  const sentence = sentences.find(s => s.id === word.sentence_id);
  return (
    <div className="panel">
      <div className="panel-head" style={{ justifyContent: "space-between" }}>
        <span className="panel-title">Inspector</span>
        <Tag kind={word.pos === "MWE" ? "mwe" : ""}>{word.pos}</Tag>
      </div>
      <div style={{ padding: 18 }}>
        <div className="disp" style={{ fontSize: 30, fontWeight: 600, letterSpacing: "-0.02em", lineHeight: 1.05, color: "var(--ink)" }}>
          {word.lemma}
        </div>
        <div style={{ marginTop: 6, color: "var(--accent)", fontSize: 14, fontStyle: "italic" }}>
          {word.gloss}
        </div>

        <div style={{ marginTop: 16, display: "flex", flexDirection: "column", gap: 10 }}>
          <KV k="grammar" v={word.grammar} />
          <KV k="forms in text" v={word.forms.join(", ")} mono />
          <KV k="tokens" v={word.count} mono />
          <KV k="leverage" v={`${word.leverage} unlocks`} mono accent />
          <KV k="status" v={<><StatusDot status={word.status} /> &nbsp;{word.status}</>} />
        </div>

        {/* Morph decomposition */}
        <div style={{ marginTop: 18 }}>
          <div className="mono" style={{ fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)", marginBottom: 8 }}>
            morphology
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {word.morph.map((m, i) => (
              <div key={i} style={{
                display: "flex", alignItems: "center", gap: 10,
                padding: "8px 10px",
                background: "var(--bg-raise)",
                border: "1px solid var(--line)",
                borderRadius: 3,
              }}>
                <span className="mono num" style={{ width: 16, textAlign: "right", color: "var(--ink-mute)", fontSize: 11 }}>{i + 1}</span>
                <span className="mono" style={{ color: "var(--ink)", fontWeight: 600 }}>{m.stem}</span>
                {m.suffix && (
                  <>
                    <span className="mono" style={{ color: "var(--ink-mute)" }}>+</span>
                    <span className="mono" style={{ color: "var(--accent)", fontWeight: 600 }}>-{m.suffix}</span>
                  </>
                )}
                <span className="grow" />
                <span className="mono" style={{ fontSize: 11, color: "var(--ink-mute)" }}>{m.note}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Example sentence */}
        {sentence && (
          <div style={{ marginTop: 18 }}>
            <div className="mono" style={{ fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)", marginBottom: 8 }}>
              first occurrence
            </div>
            <div className="disp" style={{ fontSize: 16, lineHeight: 1.6, color: "var(--ink-soft)", padding: "10px 12px", background: "var(--bg-raise)", borderRadius: 3, borderLeft: "2px solid var(--accent)" }}>
              <Sentence text={sentence.text} highlight={word.forms[0].replace("…", "")} status={word.pos === "MWE" ? "mwe" : word.status} />
            </div>
          </div>
        )}

        <div style={{ marginTop: 16, display: "flex", gap: 6 }}>
          <button className="btn btn-primary" style={{ flex: 1, justifyContent: "center" }}>+ Add to deck</button>
          <button className="btn">Mark known</button>
          <button className="btn">Ignore</button>
        </div>
      </div>
    </div>
  );
}

function KV({ k, v, mono, accent }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 8, alignItems: "baseline" }}>
      <span className="mono" style={{ fontSize: 10.5, letterSpacing: "0.12em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
        {k}
      </span>
      <span className={mono ? "mono num" : ""} style={{ color: accent ? "var(--accent-2)" : "var(--ink)", fontSize: 13, textAlign: "right" }}>{v}</span>
    </div>
  );
}

function ParseStatsPanel({ parsed }) {
  const D = window.FED_DATA;
  const distNew = parsed.words.filter(w => w.status === "new").length;
  const distLearning = parsed.words.filter(w => w.status === "learning").length;
  const distKnown = parsed.words.filter(w => w.status === "known").length;
  const mweCount = parsed.words.filter(w => w.pos === "MWE").length;

  return (
    <div className="panel">
      <div className="panel-head"><span className="panel-title">Run summary</span></div>
      <div style={{ padding: 18, display: "flex", flexDirection: "column", gap: 16 }}>
        <div className="row gap-16">
          <window.Stat label="Coverage" value={`${(parsed.coverage * 100).toFixed(0)}%`} accent="var(--accent-2)" sub={`${parsed.total_tokens} tokens`} />
          <window.Stat label="Latency" value={`${parsed.duration_ms}ms`} sub="end-to-end" />
        </div>

        <div>
          <div className="mono" style={{ fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)", marginBottom: 8 }}>
            distribution
          </div>
          <window.CoverageBar known={distKnown} learning={distLearning} neww={distNew} height={10} />
          <div className="row gap-16 mono" style={{ marginTop: 8, fontSize: 11, color: "var(--ink-mute)" }}>
            <span><span className="dot known" /> {distKnown} known</span>
            <span><span className="dot learning" /> {distLearning} learning</span>
            <span><span className="dot new" /> {distNew} new</span>
          </div>
        </div>

        <div>
          <div className="mono" style={{ fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)", marginBottom: 8 }}>
            stages
          </div>
          <Pipeline parsed={parsed} />
        </div>

        {mweCount > 0 && (
          <div>
            <div className="row" style={{ justifyContent: "space-between", marginBottom: 8 }}>
              <span className="mono" style={{ fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
                Multi-word expressions
              </span>
              <Tag kind="mwe">{mweCount}</Tag>
            </div>
            {parsed.words.filter(w => w.pos === "MWE").map(w => (
              <div key={w.lemma} style={{ padding: "8px 10px", background: "var(--bg-raise)", borderRadius: 3, fontStyle: "italic", fontSize: 12.5, color: "var(--ink-soft)", borderLeft: "2px solid var(--accent-4)" }}>
                "{w.lemma}"
                <div className="mono" style={{ fontSize: 10.5, fontStyle: "normal", color: "var(--ink-mute)", marginTop: 2 }}>
                  → {w.gloss}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function Pipeline({ parsed }) {
  const stages = [
    { name: "tokenize", ms: 4, status: "done" },
    { name: "lookup forms", ms: 18, status: "done" },
    { name: "strip suffix", ms: 9, status: "done" },
    { name: "split compounds", ms: 7, status: "done" },
    { name: "lookup glosses", ms: 9, status: "done" },
  ];
  const total = stages.reduce((s, x) => s + x.ms, 0);
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      {stages.map(s => (
        <div key={s.name} className="row" style={{ gap: 8 }}>
          <span className="mono" style={{ fontSize: 11, color: "var(--ink-soft)", width: 110 }}>{s.name}</span>
          <div style={{ flex: 1, height: 4, background: "var(--bg-raise)", borderRadius: 2, overflow: "hidden" }}>
            <div style={{ width: `${(s.ms / total) * 100}%`, height: "100%", background: "var(--accent)" }} />
          </div>
          <span className="mono num" style={{ fontSize: 11, color: "var(--ink-mute)", width: 36, textAlign: "right" }}>{s.ms}ms</span>
        </div>
      ))}
    </div>
  );
}

window.ParseView = ParseView;
