/* global React, FED_DATA */
const { useState: useStateLd, useMemo: useMemoLd, useRef: useRefLd, useEffect: useEffectLd } = React;

function detectLanguage(text) {
  if (!text || text.trim().length < 4) return { lang: null, confidence: 0 };
  const letters = text.replace(/[^a-zA-ZäöõüšžÄÖÕÜŠŽ]/g, "");
  if (letters.length < 4) return { lang: null, confidence: 0 };
  const hasOtilde = /õ/i.test(text);
  if (hasOtilde) return { lang: "ET", confidence: 0.95 };
  const aOcount = (text.match(/[äöÄÖ]/g) || []).length;
  const ratio = aOcount / letters.length;
  if (ratio > 0.005) return { lang: "FI", confidence: Math.min(0.95, ratio * 30) };
  // Fallback: any reasonable text with no diacritics still gets a soft FI guess
  // so the demo Parse button is always reachable.
  if (letters.length >= 8) return { lang: "FI", confidence: 0.4 };
  return { lang: null, confidence: 0 };
}

// Savoy-vase silhouette — Aalto's signature 1936 form, rendered as svg
function SavoyVase(props) {
  return (
    <svg viewBox="0 0 220 320" fill="none" xmlns="http://www.w3.org/2000/svg" {...props}>
      <path d="
        M 30 12
        C 50 28, 60 60, 78 90
        C 92 116, 56 142, 70 178
        C 82 208, 122 198, 138 226
        C 152 250, 110 270, 130 296
        C 142 312, 180 308, 200 308
        L 200 320 L 30 320 Z"
        fill="currentColor" opacity="0.85" />
      <path d="
        M 30 12
        C 50 28, 60 60, 78 90
        C 92 116, 56 142, 70 178
        C 82 208, 122 198, 138 226
        C 152 250, 110 270, 130 296
        C 142 312, 180 308, 200 308"
        stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" fill="none" opacity="0.4" />
    </svg>
  );
}

function LandingView({ onCreateDeck, onOpenWord, aaltoMode }) {
  const [text, setText] = useStateLd("");
  const [parsed, setParsed] = useStateLd(false);
  const [parsing, setParsing] = useStateLd(false);
  const [focused, setFocused] = useStateLd(false);
  const [selectedLemma, setSelectedLemma] = useStateLd(null);
  const taRef = useRefLd(null);

  const detect = useMemoLd(() => detectLanguage(text), [text]);
  const charCount = text.length;
  const result = parsed ? FED_DATA.SAMPLE_PARSE_FI : null;

  function handleParse() {
    if (!text.trim() || !detect.lang) return;
    setParsing(true);
    setTimeout(() => { setParsing(false); setParsed(true); }, 450);
  }

  function loadDemo(which) {
    const demo = which === "ET" ? FED_DATA.ET_TEXT_1 : which === "FI2" ? FED_DATA.FI_TEXT_2 : FED_DATA.FI_TEXT_1;
    setText(demo);
    setParsed(false);
  }

  function copyAll() {
    if (!result) return;
    const txt = result.words.map((w) => `${w.lemma}\t${w.pos}\t${w.gloss}`).join("\n");
    navigator.clipboard.writeText(txt);
    showToast("Copied " + result.words.length + " lemmas");
  }
  function downloadCSV() {
    if (!result) return;
    const rows = [["lemma", "pos", "forms", "definition", "grammar", "count"]];
    result.words.forEach((w) => rows.push([w.lemma, w.pos, w.forms.join("|"), w.gloss, w.grammar, w.count]));
    const csv = rows.map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(",")).join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `finnest-${(detect.lang || "xx").toLowerCase()}-wordlist.csv`;
    a.click();
    showToast("Downloaded CSV");
  }

  function showToast(msg) {
    const t = document.getElementById("toast");
    if (!t) return;
    t.textContent = msg;
    t.classList.add("show");
    clearTimeout(showToast._t);
    showToast._t = setTimeout(() => t.classList.remove("show"), 1800);
  }

  // ─── PARSED RESULTS VIEW ───
  if (parsed && result) {
    return (
      <div style={{ maxWidth: 1180, margin: "0 auto", padding: "40px 40px 80px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 14, marginBottom: 28, flexWrap: "wrap" }}>
          <button className="btn btn-ghost" onClick={() => setParsed(false)}>← New text</button>
          <span className={`lang-badge ${detect.lang.toLowerCase()}`}>
            <span className="glyph">{detect.lang === "FI" ? "Aa" : "Õõ"}</span>
            {detect.lang === "FI" ? "Finnish" : "Estonian"}
          </span>
          <span className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", letterSpacing: "0.04em" }}>
            {result.words.length} unique lemmas · parsed in {result.duration_ms}ms
          </span>
          <div style={{ flex: 1 }} />
          <button className="btn" onClick={copyAll}>Copy list</button>
          <button className="btn" onClick={downloadCSV}>Download CSV</button>
          <button className="btn btn-primary" onClick={onCreateDeck}>
            Create deck →
          </button>
        </div>

        <div className="panel">
          <table className="data-table">
            <thead>
              <tr>
                <th style={{ width: 24 }}></th>
                <th>Lemma</th>
                <th>Type</th>
                <th>Forms in text</th>
                <th>Definition</th>
                <th style={{ width: 140 }}>Grammar</th>
                <th style={{ width: 50, textAlign: "right" }}>×</th>
              </tr>
            </thead>
            <tbody>
              {result.words.map((w) =>
              <tr key={w.lemma} onClick={() => { setSelectedLemma(w); onOpenWord(w); }}>
                  <td><span className={`dot ${w.status}`} /></td>
                  <td className="disp" style={{ fontWeight: 500, fontSize: 15 }}>
                    {w.pos === "MWE" ?
                      <span style={{ color: "var(--blue)", fontStyle: "italic" }}>{w.lemma.length > 30 ? w.lemma.slice(0, 28) + "…" : w.lemma}</span> :
                      w.lemma}
                  </td>
                  <td><span className="tag">{w.pos}</span></td>
                  <td className="mono" style={{ color: "var(--ink-soft)", fontSize: 12 }}>{w.forms.join(", ")}</td>
                  <td style={{ color: "var(--ink-soft)" }}>{w.gloss}</td>
                  <td className="mono" style={{ fontSize: 11, color: "var(--ink-mute)" }}>{w.grammar}</td>
                  <td className="mono num" style={{ textAlign: "right", color: "var(--ink-soft)" }}>{w.count}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        <div style={{ marginTop: 18, display: "flex", justifyContent: "center", gap: 12, color: "var(--ink-mute)", fontSize: 12, fontFamily: "var(--font-mono)", letterSpacing: "0.04em" }}>
          <span>Click any row for definition · grammar · context</span>
        </div>
      </div>
    );
  }

  // ─── PASTE LANDING ───
  return (
    <div className="landing-wrap">
      {/* Vertical AALTO mark — only renders in bolder variant via CSS */}
      <div className="aalto-mark" aria-hidden="true">
        <span className="vlabel">Alvar Aalto · Paimio · 1933</span>
        <div className="vstrip" />
      </div>

      {/* Savoy-vase silhouette — bolder variant only */}
      <SavoyVase className="vase-svg" aria-hidden="true" />

      <div className="hero-eyebrow">
        <span className="pulse" />free · no account · no history saved
      </div>

      <h1 className="hero-h">
        Paste your <em>Suomi</em> or <em>Eesti</em>.<br />
        Lift the words out.
      </h1>

      <p className="hero-sub">
        Drop in any Finnish or Estonian text — news, a chapter, a conversation.
        Get every lemma, its forms, and a clean definition. Export, or sign in to
        keep a deck and review with FSRS.
      </p>

      <div className={`paste-box ${focused ? "focused" : ""}`}>
        <textarea
          ref={taRef}
          placeholder="Paste your text here…&#10;&#10;Toissapäivänä menin pankkiin. Osoittautui, että se oli kiinni."
          value={text}
          onChange={(e) => { setText(e.target.value); setParsed(false); }}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
        />
        <div className="paste-foot">
          <span className={`lang-badge ${detect.lang ? detect.lang.toLowerCase() : "un"}`}>
            <span className="glyph">{detect.lang === "FI" ? "Aa" : detect.lang === "ET" ? "Õõ" : "??"}</span>
            {detect.lang === "FI" ? "Finnish" : detect.lang === "ET" ? "Estonian" : "Detecting…"}
          </span>
          <span className="meter">
            <b>{charCount.toLocaleString()}</b> / 300,000 chars
          </span>
          <div style={{ flex: 1 }} />
          <button className="btn btn-primary btn-lg"
            disabled={!text.trim() || !detect.lang || parsing}
            style={{ opacity: !text.trim() || !detect.lang || parsing ? 0.4 : 1, cursor: !text.trim() || !detect.lang ? "not-allowed" : "pointer" }}
            onClick={handleParse}>
            {parsing ? "Parsing…" : "Parse"} <span className="kbd">⌘↵</span>
          </button>
        </div>
      </div>

      <div className="demo-chips">
        <span>or try →</span>
        <button className="demo-chip" onClick={() => loadDemo("FI")}>FI · everyday</button>
        <button className="demo-chip" onClick={() => loadDemo("FI2")}>FI · linguistics</button>
        <button className="demo-chip" onClick={() => loadDemo("ET")}>ET · short story</button>
      </div>

      <div className="freemium-strip">
        <div className="freemium-cell">
          <div className="freemium-num">i.</div>
          <div className="freemium-h">Parse anything, free</div>
          <div className="freemium-p">Up to 300k characters per paste. No login. Nothing saved server-side.</div>
        </div>
        <div className="freemium-cell">
          <div className="freemium-num">ii.</div>
          <div className="freemium-h">Copy or download</div>
          <div className="freemium-p">Word list as plain text or CSV — drop straight into Anki, a spreadsheet, or your tool of choice.</div>
        </div>
        <div className="freemium-cell">
          <div className="freemium-num">iii.</div>
          <div className="freemium-h">Save decks · sign in</div>
          <div className="freemium-p">Persist progress, FSRS review across decks, leverage ranking. Free Google sign-in.</div>
        </div>
      </div>

      <div className="colophon">
        <span>Suomi · Eesti — humanist, restrained, blue.</span>
        <span className="swatches">
          <span className="sw w" title="paper" />
          <span className="sw b" title="Nordic blue" />
          <span className="sw k" title="ink" />
          <span className="sw r" title="birch" />
        </span>
      </div>
    </div>
  );
}

window.LandingView = LandingView;
window.detectLanguage = detectLanguage;
