/* global React, FED_DATA */
const { useState: useStateLd, useMemo: useMemoLd, useRef: useRefLd, useEffect: useEffectLd } = React;

// Detect language by character analysis (mirrors the README's heuristic)
function detectLanguage(text) {
  if (!text || text.trim().length < 10) return { lang: null, confidence: 0 };
  const letters = text.replace(/[^a-zA-ZäöõüšžÄÖÕÜŠŽ]/g, "");
  if (letters.length < 10) return { lang: null, confidence: 0 };
  const hasOtilde = /õ/i.test(text);
  if (hasOtilde) return { lang: "ET", confidence: 0.95 };
  const aOcount = (text.match(/[äöÄÖ]/g) || []).length;
  const ratio = aOcount / letters.length;
  if (ratio > 0.015) return { lang: "FI", confidence: Math.min(0.95, ratio * 30) };
  return { lang: null, confidence: 0 };
}

function LandingView({ onCreateDeck, onOpenWord }) {
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
    setTimeout(() => {setParsing(false);setParsed(true);}, 450);
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
      <div style={{ maxWidth: 1080, margin: "0 auto", padding: "32px 32px 80px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 24 }}>
          <button className="btn btn-ghost" onClick={() => setParsed(false)}>← New text</button>
          <span className={`lang-badge ${detect.lang.toLowerCase()}`}>
            <span className="glyph">{detect.lang === "FI" ? "Aa" : "Õõ"}</span>
            {detect.lang === "FI" ? "Finnish" : "Estonian"}
          </span>
          <span className="mono" style={{ fontSize: 11, color: "var(--ink-mute)" }}>
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
              <tr key={w.lemma} onClick={() => {setSelectedLemma(w);onOpenWord(w);}}>
                  <td><span className={`dot ${w.status}`} /></td>
                  <td className="mono" style={{ fontWeight: 500 }}>
                    {w.pos === "MWE" ?
                  <span style={{ color: "var(--accent)" }}>{w.lemma.length > 30 ? w.lemma.slice(0, 28) + "…" : w.lemma}</span> :
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

        <div style={{ marginTop: 16, display: "flex", justifyContent: "center", gap: 12, color: "var(--ink-mute)", fontSize: 12.5, fontFamily: "var(--font-mono)" }}>
          <span>Click any row for definition · grammar · context</span>
        </div>
      </div>);

  }

  // ─── PASTE LANDING ───
  return (
    <div className="landing-wrap">
      <div className="hero-eyebrow"><span className="pulse" />free · no account · no history saved</div>
      <h1 className="hero-h" style={{ fontSize: "40px" }}>
        Paste your Finnish or Estonian Text Here
      </h1>
      <p className="hero-sub">Paste any piece of text, Get the words, meanings, export them for free.

If you create an account, you get deck creation, known word count tracking, reviews, uploading md, txt, epub files, longer text lengths.  
      </p>

      <div className={`paste-box ${focused ? "focused" : ""}`}>
        <textarea
          ref={taRef}
          placeholder="Paste your text here…&#10;&#10;Toissapäivänä menin pankkiin. Osoittautui, että se oli kiinni."
          value={text}
          onChange={(e) => {setText(e.target.value);setParsed(false);}}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}>Statistikaameti andmetel oli Eesti jaekaubandusettevõtete müügitulu märtsis 975 miljonit eurot. Võrreldes 2025. aasta sama kuuga suurenes müügimaht seitse protsenti ning kasvu mõjutasid enim mootorikütuse jaemüügiga tegelevad ettevõtted.

Statistikaameti analüütiku Johanna Linda Pihlaku sõnul suurenes vaatamata kütusehinna järsule tõusule mootorikütuse jaemüügiga tegelevate ettevõtete müügimaht eelmise aasta märtsiga võrreldes 15 protsenti.

"Üheks mõjutajaks oli tarbijate ebakindlus ning hinnatõusu jätkumist kartes hakati kütust varuks ostma," tõi Pihlak välja.

Tööstuskaupade kaupluste müügimaht suurenes 2025. aasta märtsiga võrreldes 10 protsenti.

Samas toidukaupade kaupluste müügimahu vähenemine märtsis jätkus ning kahanes eelmise aasta sama kuuga võrreldes kaks protsenti. "Nende ettevõtete müügimahu vähenemisele avaldas mõju toidukaupade hinnatõus," selgitas Pihlak.</textarea>

        <div className="paste-foot">
          <span className={`lang-badge ${detect.lang ? detect.lang.toLowerCase() : "un"}`}>
            <span className="glyph">{detect.lang === "FI" ? "Aa" : detect.lang === "ET" ? "Õõ" : "??"}</span>
            {detect.lang === "FI" ? "Finnish" : detect.lang === "ET" ? "Estonian" : "Detecting…"}
          </span>
          <span className="meter">
            <b>{charCount.toLocaleString()}</b> / 300,000 chars
          </span>
          <div style={{ flex: 1 }} />
          <button className="btn btn-primary btn-lg" disabled={!text.trim() || !detect.lang || parsing} style={{ opacity: !text.trim() || !detect.lang || parsing ? 0.4 : 1, cursor: !text.trim() || !detect.lang ? "not-allowed" : "pointer" }} onClick={handleParse}>

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
          <div className="freemium-num">01</div>
          <div className="freemium-h">Parse anything, free</div>
          <div className="freemium-p">Up to 300k characters per paste. No login. Nothing saved server-side.</div>
        </div>
        <div className="freemium-cell">
          <div className="freemium-num">02</div>
          <div className="freemium-h">Copy or download</div>
          <div className="freemium-p">Word list as plain text or CSV - drop straight into Anki, spreadsheet or software of choice.</div>
        </div>
        <div className="freemium-cell">
          <div className="freemium-num">03</div>
          <div className="freemium-h">Save decks · sign in</div>
          <div className="freemium-p">Persist progress, FSRS review across decks, leverage ranking. Free Google sign-in.</div>
        </div>
      </div>
    </div>);
}

window.LandingView = LandingView;
window.detectLanguage = detectLanguage;