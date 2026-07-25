/* global React, FED_DATA */
/*
 * proto-flows.jsx - additional flows from the user-flow diagram (PR #152):
 *   - SignupRibbon  (anonymous results → SU)
 *   - SaveAsModal   (results → SP{} → DD)
 *   - CorrectionModal  (✎ Wrong? - flag-only OR right-answer)
 *   - ColdStart     (first time, 0 decks)
 *   - KnownWordsImport  (cold start → KW → DL)
 *   - EphemeralBanner (parse opt-out toggle)
 *   - DonePanel     (review session done → back to dashboard)
 * Each is wired into AaltoApp via aalto-app.jsx.
 */

const { useState: useStateF, useEffect: useEffectF, useRef: useRefF } = React;

// ─── tiny shared helpers ─────────────────────────────────────────────
function showToastF(msg) {
  const t = document.getElementById("toast");
  if (!t) return;
  t.textContent = msg;
  t.classList.add("show");
  clearTimeout(showToastF._t);
  showToastF._t = setTimeout(() => t.classList.remove("show"), 2200);
}

function ModalShell({ open, onClose, children, width = 480 }) {
  useEffectF(() => {
    if (!open) return;
    const onKey = (e) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div className="modal-mask" onClick={onClose}>
      <div className="modal" style={{ maxWidth: width }} onClick={e => e.stopPropagation()}>
        {children}
      </div>
    </div>
  );
}

// ─── SignupRibbon ───────────────────────────────────────────────────
// Sticky bar that appears above anonymous results.
function SignupRibbon({ count, onSave, onDismiss }) {
  return (
    <div style={{
      display: "flex", alignItems: "center", gap: 18, flexWrap: "wrap",
      padding: "14px 20px",
      background: "var(--ink)", color: "var(--paper)",
      borderRadius: "var(--r-md)",
      marginBottom: 22,
      border: "1px solid var(--ink)",
    }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, flex: 1, minWidth: 280 }}>
        <span style={{
          fontFamily: "var(--font-disp)", fontStyle: "italic", fontSize: 22, fontWeight: 600,
          color: "var(--birch)",
        }}>
          ✦
        </span>
        <div>
          <div style={{ fontFamily: "var(--font-disp)", fontSize: 17, fontWeight: 500, lineHeight: 1.2 }}>
            Save these <em style={{ color: "var(--birch)" }}>{count}</em> words?
          </div>
          <div className="mono" style={{ fontSize: 11, color: "oklch(from var(--paper) l c h / 0.65)", marginTop: 2, letterSpacing: "0.04em" }}>
            We'll carry this parse into your first deck - free, no card required.
          </div>
        </div>
      </div>
      <button className="btn btn-primary" onClick={onSave}
        style={{ background: "var(--birch)", borderColor: "var(--birch)", color: "var(--ink)" }}>
        Save these words →
      </button>
      <button onClick={onDismiss}
        style={{
          all: "unset", cursor: "pointer", padding: "6px 10px",
          color: "oklch(from var(--paper) l c h / 0.55)",
          fontSize: 12,
          fontFamily: "var(--font-mono)",
        }}>
        Not now
      </button>
    </div>
  );
}

// ─── SaveAsModal - Save as → new deck OR add to existing ─────────────
function SaveAsModal({ open, onClose, lang, count, onCreated }) {
  const [mode, setMode] = useStateF("new"); // new | existing
  const [name, setName] = useStateF("");
  const [pickDeckId, setPickDeckId] = useStateF(null);

  useEffectF(() => {
    if (!open) return;
    setMode("new");
    setName(lang === "FI" ? "Finnish · paste " + new Date().toISOString().slice(5,10) : "Estonian · paste " + new Date().toISOString().slice(5,10));
    setPickDeckId(null);
  }, [open, lang]);

  if (!open) return null;

  const sameLangDecks = (FED_DATA.SAMPLE_DECKS || []).filter(d => d.lang === lang);

  function commit() {
    if (mode === "new") {
      if (!name.trim()) return;
      showToastF(`Created deck "${name.trim()}" · ${count} cards`);
      onCreated && onCreated({ kind: "new", name: name.trim() });
    } else {
      if (!pickDeckId) return;
      const d = sameLangDecks.find(x => x.id === pickDeckId);
      showToastF(`Added ${count} cards → ${d.title}`);
      onCreated && onCreated({ kind: "existing", deckId: pickDeckId });
    }
    onClose();
  }

  return (
    <ModalShell open={open} onClose={onClose} width={520}>
      <div style={{ padding: "22px 26px 10px", borderBottom: "1px solid var(--line-soft)" }}>
        <div className="mono" style={{ fontSize: 10, letterSpacing: "0.18em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
          Save as…
        </div>
        <div className="disp" style={{ fontSize: 24, fontWeight: 500, letterSpacing: "-0.02em", marginTop: 4 }}>
          <em style={{ fontStyle: "italic", color: "var(--blue)" }}>{count}</em> words from this parse
        </div>
      </div>

      <div style={{ padding: "18px 26px 4px", display: "flex", gap: 8 }}>
        {[
          { v: "new", label: "New deck" },
          { v: "existing", label: `Add to existing (${sameLangDecks.length})`, disabled: sameLangDecks.length === 0 },
        ].map(opt => (
          <button key={opt.v}
            disabled={opt.disabled}
            onClick={() => setMode(opt.v)}
            style={{
              all: "unset",
              cursor: opt.disabled ? "not-allowed" : "pointer",
              flex: 1,
              padding: "10px 14px",
              borderRadius: "var(--r-md)",
              border: "1px solid " + (mode === opt.v ? "var(--blue)" : "var(--line)"),
              background: mode === opt.v ? "oklch(from var(--blue) l c h / 0.08)" : "var(--bg)",
              color: opt.disabled ? "var(--ink-mute)" : (mode === opt.v ? "var(--blue)" : "var(--ink-soft)"),
              fontSize: 13, fontWeight: 500, textAlign: "center",
              opacity: opt.disabled ? 0.5 : 1,
            }}>
            {opt.label}
          </button>
        ))}
      </div>

      <div style={{ padding: "16px 26px 8px" }}>
        {mode === "new" ? (
          <>
            <label className="mono" style={{ display: "block", fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)", marginBottom: 6 }}>
              Deck name
            </label>
            <input
              autoFocus
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="My Finnish deck"
              style={{
                width: "100%",
                padding: "10px 14px",
                background: "var(--bg)",
                border: "1px solid var(--line)",
                borderRadius: "var(--r-md)",
                fontFamily: "var(--font-disp)",
                fontSize: 16,
                color: "var(--ink)",
                outline: "none",
                boxSizing: "border-box",
              }}
              onFocus={e => e.target.style.borderColor = "var(--blue)"}
              onBlur={e => e.target.style.borderColor = "var(--line)"}
            />
            <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", marginTop: 12, letterSpacing: "0.02em", lineHeight: 1.5 }}>
              {count} cards · language <span style={{ color: "var(--blue)" }}>{lang === "FI" ? "Finnish" : "Estonian"}</span> · FSRS scheduling on
            </div>
          </>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 6, maxHeight: 240, overflow: "auto" }}>
            {sameLangDecks.map(d => (
              <button key={d.id} onClick={() => setPickDeckId(d.id)}
                style={{
                  all: "unset", cursor: "pointer",
                  display: "flex", alignItems: "center", gap: 10,
                  padding: "10px 12px",
                  borderRadius: "var(--r-md)",
                  border: "1px solid " + (pickDeckId === d.id ? "var(--blue)" : "var(--line-soft)"),
                  background: pickDeckId === d.id ? "oklch(from var(--blue) l c h / 0.06)" : "transparent",
                }}>
                <span className={`tag ${d.lang.toLowerCase()}`} style={{ padding: "1px 6px", fontSize: 10 }}>{d.lang}</span>
                <span style={{ flex: 1, fontWeight: 500 }}>{d.title}</span>
                <span className="mono" style={{ fontSize: 11, color: "var(--ink-mute)" }}>
                  {d.unique} lemmas
                </span>
              </button>
            ))}
            {sameLangDecks.length === 0 && (
              <div className="mono" style={{ fontSize: 12, color: "var(--ink-mute)", padding: "16px 4px" }}>
                No {lang === "FI" ? "Finnish" : "Estonian"} decks yet - switch to "New deck".
              </div>
            )}
          </div>
        )}
      </div>

      <div style={{ padding: "10px 22px 18px", display: "flex", gap: 8, justifyContent: "flex-end" }}>
        <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn btn-primary"
          disabled={mode === "new" ? !name.trim() : !pickDeckId}
          style={{ opacity: (mode === "new" ? !name.trim() : !pickDeckId) ? 0.4 : 1 }}
          onClick={commit}>
          {mode === "new" ? "Create deck →" : "Add to deck →"}
        </button>
      </div>
    </ModalShell>
  );
}

// ─── CorrectionModal - ✎ Wrong? from any results row / review card ────
function CorrectionModal({ open, onClose, target }) {
  // target = { lemma, pos, gloss, sentence, source }
  const [path, setPath] = useStateF("flag"); // "flag" | "edit"
  const [lemma, setLemma] = useStateF("");
  const [pos, setPos] = useStateF("");
  const [grammar, setGrammar] = useStateF("");
  const [notes, setNotes] = useStateF("");

  useEffectF(() => {
    if (!open) return;
    setPath("flag");
    setLemma(target?.lemma || "");
    setPos(target?.pos || "");
    setGrammar(target?.grammar || "");
    setNotes("");
  }, [open, target]);

  if (!open) return null;

  function submit() {
    if (path === "flag") {
      showToastF("Submitted ✓ flag-only - thanks");
    } else {
      if (!lemma.trim() || !pos) { showToastF("Need at least lemma + POS"); return; }
      showToastF(`Submitted ✓ correction → ${lemma} (${pos})`);
    }
    onClose();
  }

  return (
    <ModalShell open={open} onClose={onClose} width={540}>
      <div style={{ padding: "22px 26px 10px", borderBottom: "1px solid var(--line-soft)" }}>
        <div className="mono" style={{ fontSize: 10, letterSpacing: "0.18em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
          ✎ Report a parse problem
        </div>
        <div className="disp" style={{ fontSize: 22, fontWeight: 500, letterSpacing: "-0.02em", marginTop: 4 }}>
          <span style={{ color: "var(--ink-mute)" }}>This row says</span>{" "}
          <em style={{ fontStyle: "italic", color: "var(--blue)" }}>
            {target?.lemma || "-"}
          </em>
        </div>
        {target?.sentence && (
          <div className="disp" style={{
            marginTop: 12, padding: "10px 14px",
            background: "var(--bg-deep)", border: "1px solid var(--line-soft)",
            borderRadius: "var(--r-sm)", fontSize: 14, lineHeight: 1.45,
            fontStyle: "italic", color: "var(--ink-soft)",
          }}>
            "{target.sentence}"
          </div>
        )}
      </div>

      {/* Two-path radio - bigger hit-target than a tiny radio dot */}
      <div style={{ padding: "16px 26px 4px", display: "flex", flexDirection: "column", gap: 8 }}>
        {[
          { v: "flag", title: "I don't know the answer", sub: "Just flag this row - we'll triage it.", recommended: true },
          { v: "edit", title: "Right answer is…", sub: "Propose lemma · POS · optional grammar + notes.", recommended: false },
        ].map(opt => (
          <button key={opt.v} onClick={() => setPath(opt.v)}
            style={{
              all: "unset", cursor: "pointer",
              display: "flex", alignItems: "flex-start", gap: 12,
              padding: "12px 14px",
              borderRadius: "var(--r-md)",
              border: "1px solid " + (path === opt.v ? "var(--blue)" : "var(--line)"),
              background: path === opt.v ? "oklch(from var(--blue) l c h / 0.06)" : "transparent",
              position: "relative",
            }}>
            <span style={{
              flex: "none",
              width: 16, height: 16, borderRadius: "50%",
              border: "1.5px solid " + (path === opt.v ? "var(--blue)" : "var(--ink-mute)"),
              display: "grid", placeItems: "center",
              marginTop: 2,
            }}>
              {path === opt.v && <span style={{ width: 7, height: 7, background: "var(--blue)", borderRadius: "50%" }} />}
            </span>
            <div style={{ flex: 1, minWidth: 0, paddingRight: opt.recommended ? 70 : 0 }}>
              <div style={{ fontWeight: 600, color: path === opt.v ? "var(--ink)" : "var(--ink-soft)" }}>{opt.title}</div>
              <div style={{ fontSize: 12.5, color: "var(--ink-mute)", marginTop: 2 }}>{opt.sub}</div>
            </div>
            {opt.recommended && (
              <span className="mono" style={{ position: "absolute", top: 12, right: 12, fontSize: 9.5, padding: "1px 6px", borderRadius: 999, background: "var(--cream)", color: "var(--ink-mute)", letterSpacing: "0.1em", textTransform: "uppercase" }}>
                new path
              </span>
            )}
          </button>
        ))}
      </div>

      {path === "edit" && (
        <div style={{ padding: "14px 26px 4px", display: "grid", gap: 10 }}>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 140px", gap: 10 }}>
            <div>
              <label className="mono" style={{ display: "block", fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)", marginBottom: 4 }}>Proposed lemma</label>
              <input value={lemma} onChange={e => setLemma(e.target.value)}
                placeholder="e.g. toissapäivä"
                style={{ width: "100%", padding: "8px 12px", background: "var(--bg)", border: "1px solid var(--line)", borderRadius: "var(--r-sm)", fontFamily: "var(--font-disp)", fontSize: 15, color: "var(--ink)", outline: "none", boxSizing: "border-box" }} />
            </div>
            <div>
              <label className="mono" style={{ display: "block", fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)", marginBottom: 4 }}>POS</label>
              <select value={pos} onChange={e => setPos(e.target.value)}
                style={{ width: "100%", padding: "8px 10px", background: "var(--bg)", border: "1px solid var(--line)", borderRadius: "var(--r-sm)", fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--ink)", outline: "none", boxSizing: "border-box" }}>
                <option value="">-</option>
                {["NOUN","VERB","ADJ","ADV","PRON","NUM","CONJ","PROPN","MWE","INTJ"].map(p => <option key={p} value={p}>{p}</option>)}
              </select>
            </div>
          </div>
          <div>
            <label className="mono" style={{ display: "block", fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)", marginBottom: 4 }}>Grammar (optional)</label>
            <input value={grammar} onChange={e => setGrammar(e.target.value)}
              placeholder="e.g. Essive sg"
              style={{ width: "100%", padding: "8px 12px", background: "var(--bg)", border: "1px solid var(--line)", borderRadius: "var(--r-sm)", fontFamily: "var(--font-mono)", fontSize: 12, color: "var(--ink)", outline: "none", boxSizing: "border-box" }} />
          </div>
          <div>
            <label className="mono" style={{ display: "block", fontSize: 10, letterSpacing: "0.16em", textTransform: "uppercase", color: "var(--ink-mute)", marginBottom: 4 }}>Notes (optional)</label>
            <textarea value={notes} onChange={e => setNotes(e.target.value)}
              placeholder="Why is this the right answer? Anything that'd help triage."
              rows={3}
              style={{ width: "100%", padding: "8px 12px", background: "var(--bg)", border: "1px solid var(--line)", borderRadius: "var(--r-sm)", fontFamily: "var(--font-sans)", fontSize: 13, color: "var(--ink)", outline: "none", boxSizing: "border-box", resize: "vertical" }} />
          </div>
        </div>
      )}

      <div style={{ padding: "14px 22px 18px", display: "flex", gap: 8, justifyContent: "space-between", alignItems: "center", borderTop: "1px solid var(--line-soft)", marginTop: 8 }}>
        <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-mute)", letterSpacing: "0.06em" }}>
          → /admin/feedback queue
        </span>
        <div style={{ display: "flex", gap: 8 }}>
          <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={submit}>
            Submit ✓
          </button>
        </div>
      </div>
    </ModalShell>
  );
}

// ─── ColdStart - first-time / 0 decks dashboard ─────────────────────
function ColdStart({ onPasteText, onImportKnown, onSeedTop1000 }) {
  return (
    <div style={{ maxWidth: 920, margin: "0 auto", padding: "56px 40px 80px" }}>
      <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", letterSpacing: "0.18em", textTransform: "uppercase", marginBottom: 14 }}>
        Welcome - let's get you started
      </div>
      <h1 style={{
        fontFamily: "var(--font-disp)", fontWeight: 500, fontSize: 44,
        letterSpacing: "-0.025em", lineHeight: 1.05, color: "var(--ink)",
        marginBottom: 14,
      }}>
        Three ways to seed your first <em style={{ fontStyle: "italic", color: "var(--blue)" }}>deck</em>.
      </h1>
      <p style={{ color: "var(--ink-soft)", fontSize: 16, fontFamily: "var(--font-disp)", maxWidth: 620, marginBottom: 36, lineHeight: 1.5 }}>
        Pick one - or do all three. Decks layer, so anything you import becomes part of your "known words" baseline.
      </p>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: 16 }}>
        {/* Card 1 - paste */}
        <div className="panel" style={{ padding: "24px 22px 22px", display: "flex", flexDirection: "column", gap: 14, cursor: "pointer", transition: "border-color 0.12s" }}
          onMouseEnter={e => e.currentTarget.style.borderColor = "var(--blue)"}
          onMouseLeave={e => e.currentTarget.style.borderColor = "var(--line)"}
          onClick={onPasteText}>
          <div className="mono" style={{ fontSize: 10, letterSpacing: "0.18em", textTransform: "uppercase", color: "var(--blue)" }}>
            Recommended
          </div>
          <div className="disp" style={{ fontSize: 22, fontWeight: 500, letterSpacing: "-0.015em", lineHeight: 1.15 }}>
            Paste a text you actually want to read.
          </div>
          <div style={{ color: "var(--ink-soft)", fontSize: 13.5, lineHeight: 1.5, flex: 1 }}>
            A news article, a chapter, lyrics. We'll lift every lemma and rank by leverage.
          </div>
          <div>
            <button className="btn btn-primary">Paste a text →</button>
          </div>
        </div>

        {/* Card 2 - known words */}
        <div className="panel" style={{ padding: "24px 22px 22px", display: "flex", flexDirection: "column", gap: 14, cursor: "pointer" }}
          onMouseEnter={e => e.currentTarget.style.borderColor = "var(--ink-mute)"}
          onMouseLeave={e => e.currentTarget.style.borderColor = "var(--line)"}
          onClick={onImportKnown}>
          <div className="mono" style={{ fontSize: 10, letterSpacing: "0.18em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
            Already studying?
          </div>
          <div className="disp" style={{ fontSize: 22, fontWeight: 500, letterSpacing: "-0.015em", lineHeight: 1.15 }}>
            Import known words from Anki, CSV, or a list.
          </div>
          <div style={{ color: "var(--ink-soft)", fontSize: 13.5, lineHeight: 1.5, flex: 1 }}>
            Upload a wordlist and we'll mark them as known. Future parses skip what you already know.
          </div>
          <div>
            <button className="btn">Import known words →</button>
          </div>
        </div>

        {/* Card 3 - seed deck (gated) */}
        <div className="panel" style={{
          padding: "24px 22px 22px", display: "flex", flexDirection: "column", gap: 14,
          borderStyle: "dashed",
          background: "transparent",
          opacity: 0.85,
          position: "relative",
        }}>
          <div className="mono" style={{ fontSize: 10, letterSpacing: "0.18em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
            Coming soon · gated
          </div>
          <div className="disp" style={{ fontSize: 22, fontWeight: 500, letterSpacing: "-0.015em", lineHeight: 1.15, color: "var(--ink-soft)" }}>
            Top-1000 lemmas seed deck.
          </div>
          <div style={{ color: "var(--ink-mute)", fontSize: 13.5, lineHeight: 1.5, flex: 1 }}>
            Bootstrap with the most frequent 1,000 lemmas in Finnish or Estonian. Frequency-ranked, FSRS-ready.
          </div>
          <div>
            <button className="btn" disabled style={{ opacity: 0.6, cursor: "not-allowed", whiteSpace: "nowrap" }} onClick={onSeedTop1000}>
              Notify me when ready
            </button>
          </div>
        </div>
      </div>

      {/* Sub-action: skip */}
      <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", letterSpacing: "0.06em", marginTop: 32, textAlign: "center" }}>
        Skip and explore an empty dashboard →
      </div>
    </div>
  );
}

// ─── KnownWordsImport ────────────────────────────────────────────────
function KnownWordsImport({ open, onClose, onImported }) {
  const [tab, setTab] = useStateF("paste"); // paste | csv | anki
  const [pasted, setPasted] = useStateF("");
  const fileRef = useRefF(null);
  const [fileName, setFileName] = useStateF(null);

  useEffectF(() => {
    if (!open) return;
    setTab("paste"); setPasted(""); setFileName(null);
  }, [open]);

  if (!open) return null;

  const wordCount = pasted.split(/[,\s\n]+/).filter(Boolean).length;

  function commit() {
    let n = 0;
    if (tab === "paste") n = wordCount;
    else if (fileName) n = Math.floor(Math.random() * 800) + 600;
    if (n === 0) { showToastF("Add some words first"); return; }
    showToastF(`Imported ${n.toLocaleString()} known words`);
    onImported && onImported({ count: n });
    onClose();
  }

  return (
    <ModalShell open={open} onClose={onClose} width={580}>
      <div style={{ padding: "22px 26px 10px", borderBottom: "1px solid var(--line-soft)" }}>
        <div className="mono" style={{ fontSize: 10, letterSpacing: "0.18em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
          Import known words
        </div>
        <div className="disp" style={{ fontSize: 22, fontWeight: 500, letterSpacing: "-0.02em", marginTop: 4 }}>
          What do you <em style={{ fontStyle: "italic", color: "var(--blue)" }}>already</em> know?
        </div>
        <div style={{ fontSize: 13, color: "var(--ink-soft)", marginTop: 6, lineHeight: 1.45 }}>
          Words you import here are marked <span className="hl known">known</span> across every parse and deck.
        </div>
      </div>

      <div style={{ padding: "14px 26px 0", display: "flex", gap: 4, borderBottom: "1px solid var(--line-soft)" }}>
        {[
          { v: "paste", label: "Paste a list" },
          { v: "csv", label: "CSV upload" },
          { v: "anki", label: "Anki .apkg" },
        ].map(opt => (
          <button key={opt.v} onClick={() => setTab(opt.v)}
            style={{
              all: "unset", cursor: "pointer",
              padding: "10px 14px 12px",
              fontSize: 13, color: tab === opt.v ? "var(--ink)" : "var(--ink-mute)",
              fontWeight: 500,
              borderBottom: "2px solid " + (tab === opt.v ? "var(--blue)" : "transparent"),
              marginBottom: -1,
            }}>{opt.label}</button>
        ))}
      </div>

      <div style={{ padding: "18px 26px" }}>
        {tab === "paste" && (
          <>
            <textarea value={pasted} onChange={e => setPasted(e.target.value)}
              placeholder="kissa, koira, talo, mennä, olla, syödä …&#10;or one per line"
              rows={7}
              style={{ width: "100%", padding: "10px 14px", background: "var(--bg)", border: "1px solid var(--line)", borderRadius: "var(--r-md)", fontFamily: "var(--font-mono)", fontSize: 13, color: "var(--ink)", outline: "none", boxSizing: "border-box", resize: "vertical" }} />
            <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", marginTop: 8, letterSpacing: "0.04em" }}>
              <b style={{ color: "var(--ink)", fontWeight: 500 }}>{wordCount.toLocaleString()}</b> words detected · we'll lemmatize them
            </div>
          </>
        )}
        {tab === "csv" && (
          <div onClick={() => fileRef.current?.click()}
            style={{
              padding: "32px 20px", borderRadius: "var(--r-md)",
              border: "1.5px dashed " + (fileName ? "var(--blue)" : "var(--line)"),
              textAlign: "center", cursor: "pointer",
              background: fileName ? "oklch(from var(--blue) l c h / 0.05)" : "transparent",
            }}>
            <div className="disp" style={{ fontSize: 18, fontWeight: 500, color: fileName ? "var(--blue)" : "var(--ink-soft)" }}>
              {fileName ? `✓ ${fileName}` : "Drop a CSV here, or click to choose"}
            </div>
            <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", marginTop: 8, letterSpacing: "0.04em" }}>
              First column · headers ignored · max 50,000 rows
            </div>
            <input ref={fileRef} type="file" accept=".csv,.txt"
              style={{ display: "none" }}
              onChange={e => setFileName(e.target.files[0]?.name)} />
          </div>
        )}
        {tab === "anki" && (
          <div onClick={() => fileRef.current?.click()}
            style={{
              padding: "32px 20px", borderRadius: "var(--r-md)",
              border: "1.5px dashed " + (fileName ? "var(--blue)" : "var(--line)"),
              textAlign: "center", cursor: "pointer",
              background: fileName ? "oklch(from var(--blue) l c h / 0.05)" : "transparent",
            }}>
            <div className="disp" style={{ fontSize: 18, fontWeight: 500, color: fileName ? "var(--blue)" : "var(--ink-soft)" }}>
              {fileName ? `✓ ${fileName}` : "Drop an .apkg file"}
            </div>
            <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", marginTop: 8, letterSpacing: "0.04em" }}>
              Cards with <i>review_count ≥ 3</i> count as "known"
            </div>
            <input ref={fileRef} type="file" accept=".apkg"
              style={{ display: "none" }}
              onChange={e => setFileName(e.target.files[0]?.name)} />
          </div>
        )}
      </div>

      <div style={{ padding: "10px 22px 18px", display: "flex", gap: 8, justifyContent: "flex-end", borderTop: "1px solid var(--line-soft)" }}>
        <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn btn-primary" onClick={commit}>
          Import →
        </button>
      </div>
    </ModalShell>
  );
}

// ─── EphemeralBanner - top of parse view, opt-out toggle ─────────────
function EphemeralToggle({ value, onChange }) {
  return (
    <button onClick={() => onChange(!value)}
      style={{
        all: "unset", cursor: "pointer",
        display: "inline-flex", alignItems: "center", gap: 8,
        padding: "5px 12px",
        borderRadius: 999,
        border: "1px solid " + (value ? "var(--ink)" : "var(--line)"),
        background: value ? "var(--ink)" : "var(--bg)",
        color: value ? "var(--paper)" : "var(--ink-soft)",
        fontFamily: "var(--font-mono)",
        fontSize: 11, letterSpacing: "0.04em",
        transition: "all 0.12s",
      }}
      title={value ? "Ephemeral - this parse won't be saved to history" : "Saving to history (toggle off for ephemeral)"}>
      <span style={{
        width: 7, height: 7, borderRadius: "50%",
        background: value ? "var(--birch)" : "var(--ink-mute)",
        boxShadow: value ? "0 0 0 2px oklch(from var(--birch) l c h / 0.25)" : "none",
      }} />
      Ephemeral {value ? "ON" : "OFF"}
    </button>
  );
}

// ─── DonePanel - review session complete ─────────────────────────────
function ReviewDoneCard({ count, onBack }) {
  return (
    <div style={{ padding: "80px 40px", display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center", maxWidth: 540, margin: "0 auto" }}>
      <div className="disp" style={{ fontSize: 64, color: "var(--blue)", lineHeight: 1, fontStyle: "italic" }}>
        ✓
      </div>
      <h2 style={{ fontFamily: "var(--font-disp)", fontSize: 36, fontWeight: 500, letterSpacing: "-0.025em", marginTop: 14 }}>
        All done.
      </h2>
      <div className="mono" style={{ fontSize: 12, color: "var(--ink-mute)", letterSpacing: "0.08em", marginTop: 8 }}>
        {count} cards · next due in ~6h
      </div>
      <button className="btn btn-primary btn-lg" style={{ marginTop: 28 }} onClick={onBack}>
        Back to dashboard
      </button>
    </div>
  );
}

// ─── exports ────────────────────────────────────────────────────────
Object.assign(window, {
  SignupRibbon, SaveAsModal, CorrectionModal,
  ColdStart, KnownWordsImport, EphemeralToggle, ReviewDoneCard,
});
