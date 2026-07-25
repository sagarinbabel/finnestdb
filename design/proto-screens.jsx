/* global React, FED_DATA, LandingView, DecksView, ReviewViewV2,
   SignupRibbon, EphemeralToggle, ReviewDoneCard */
/*
 * proto-screens.jsx - wraps the existing v2 screens to inject the
 * cross-cutting flow-diagram features:
 *   • LandingViewProto:  + ✎ Wrong? on results rows, sign-up ribbon
 *                        for anonymous, ephemeral toggle
 *   • DecksViewProto:    + "+ New from text", + Known-words panel
 *   • ReviewViewProto:   + ✎ Wrong? on review card, "done" state
 */
const { useState: useStatePS, useEffect: useEffectPS } = React;

// ─── LandingViewProto ───────────────────────────────────────────────
function LandingViewProto({ onCreateDeck, onOpenWord, onCorrect, ephemeral, onEphemeralChange, isAnonymous, aaltoMode }) {
  const [dismissedRibbon, setDismissedRibbon] = useStatePS(false);
  const result = FED_DATA.SAMPLE_PARSE_FI;

  // Hook: monitor when results table is on screen, then inject ribbon + Wrong? buttons
  useEffectPS(() => {
    const tick = () => {
      // Find the results-table panel in the landing-rendered DOM
      const tbodies = document.querySelectorAll("main .data-table tbody");
      tbodies.forEach(tb => {
        if (tb.dataset.protoEnhanced === "1") return;
        // Add a "wrong" affordance to each row
        tb.querySelectorAll("tr").forEach((tr) => {
          if (tr.querySelector(".wrong-btn")) return;
          const last = tr.lastElementChild;
          if (!last) return;
          const btn = document.createElement("button");
          btn.className = "wrong-btn";
          btn.title = "Report a parse problem";
          btn.textContent = "✎";
          btn.style.cssText = `
            all: unset; cursor: pointer; opacity: 0;
            margin-left: 8px; padding: 2px 6px; border-radius: 4px;
            color: var(--ink-mute); font-size: 12px; line-height: 1;
            transition: opacity 0.12s, color 0.12s, background 0.12s;
            border: 1px solid transparent;
          `;
          btn.addEventListener("mouseenter", () => { btn.style.color = "var(--blue)"; btn.style.background = "oklch(from var(--blue) l c h / 0.08)"; btn.style.borderColor = "oklch(from var(--blue) l c h / 0.3)"; });
          btn.addEventListener("mouseleave", () => { btn.style.color = "var(--ink-mute)"; btn.style.background = "transparent"; btn.style.borderColor = "transparent"; });
          btn.addEventListener("click", (e) => {
            e.stopPropagation();
            const lemma = tr.querySelector("td:nth-child(2)")?.textContent?.trim();
            const sentence = "Toissapäivänä menin pankkiin.";
            const word = result.words.find(w => w.lemma === lemma) || result.words[0];
            onCorrect && onCorrect({ lemma: word.lemma, pos: word.pos, grammar: word.grammar, sentence });
          });
          last.appendChild(btn);
          tr.addEventListener("mouseenter", () => { btn.style.opacity = "1"; });
          tr.addEventListener("mouseleave", () => { btn.style.opacity = "0"; });
        });
        tb.dataset.protoEnhanced = "1";
      });
    };
    const id = setInterval(tick, 300);
    tick();
    return () => clearInterval(id);
  }, [onCorrect]);

  // Hook: inject ephemeral toggle next to lang badge in paste-foot
  useEffectPS(() => {
    const tick = () => {
      const foot = document.querySelector(".paste-foot");
      if (!foot || foot.dataset.protoToggle === "1") return;
      const slot = document.createElement("span");
      slot.id = "__eph-slot";
      slot.style.cssText = "display:inline-flex;align-items:center;margin-left:8px;";
      foot.insertBefore(slot, foot.children[1] || null);
      foot.dataset.protoToggle = "1";
    };
    const id = setInterval(tick, 300);
    tick();
    return () => clearInterval(id);
  }, []);

  // Render: base LandingView + (after results) inject ribbon by stuffing it into DOM
  useEffectPS(() => {
    const tick = () => {
      const panel = document.querySelector("main .data-table");
      if (!panel) return;
      const wrap = panel.closest(".panel");
      if (!wrap) return;
      let ribbonHost = document.getElementById("__signup-ribbon");
      if (isAnonymous && !dismissedRibbon) {
        if (!ribbonHost) {
          ribbonHost = document.createElement("div");
          ribbonHost.id = "__signup-ribbon";
          wrap.parentElement.insertBefore(ribbonHost, wrap);
        }
        const root = ribbonHost._root || (ribbonHost._root = ReactDOM.createRoot(ribbonHost));
        root.render(
          <SignupRibbon count={result.words.length}
            onSave={() => onCreateDeck({ lang: result.language, count: result.words.length })}
            onDismiss={() => setDismissedRibbon(true)} />
        );
      } else if (ribbonHost) {
        ribbonHost.remove();
      }
    };
    const id = setInterval(tick, 300);
    tick();
    return () => clearInterval(id);
  }, [isAnonymous, dismissedRibbon, onCreateDeck, result]);

  // Render the ephemeral toggle into its slot once it exists
  useEffectPS(() => {
    const tick = () => {
      const slot = document.getElementById("__eph-slot");
      if (!slot) return;
      const root = slot._root || (slot._root = ReactDOM.createRoot(slot));
      root.render(<EphemeralToggle value={ephemeral} onChange={onEphemeralChange} />);
    };
    const id = setInterval(tick, 300);
    tick();
    return () => clearInterval(id);
  }, [ephemeral, onEphemeralChange]);

  return <LandingView onCreateDeck={(ctx) => onCreateDeck({ lang: result.language, count: result.words.length })} onOpenWord={onOpenWord} aaltoMode={aaltoMode} />;
}

// ─── DecksViewProto ─────────────────────────────────────────────────
// Wraps DecksView and injects: "+ New from text" header button + Known-words side panel
function DecksViewProto({ onOpenDeck, onReviewDeck, onReviewAll, onNewFromText, onImportKnown }) {
  useEffectPS(() => {
    const tick = () => {
      const actions = document.querySelector("main .page-head .actions");
      if (!actions || actions.dataset.protoNewFromText === "1") return;
      const btn = document.createElement("button");
      btn.className = "btn";
      btn.textContent = "+ New from text";
      btn.addEventListener("click", () => onNewFromText && onNewFromText());
      actions.insertBefore(btn, actions.firstChild);
      actions.dataset.protoNewFromText = "1";
    };
    const id = setInterval(tick, 300);
    tick();
    return () => clearInterval(id);
  }, [onNewFromText]);

  // Inject the known-words side panel above the deck grid
  useEffectPS(() => {
    const tick = () => {
      const body = document.querySelector("main .page-body");
      if (!body || document.getElementById("__known-panel")) return;
      const grid = body.firstElementChild;
      if (!grid) return;
      const host = document.createElement("div");
      host.id = "__known-panel";
      host.style.cssText = "margin-bottom: 16px;";
      body.insertBefore(host, grid);
      const root = ReactDOM.createRoot(host);
      root.render(<KnownWordsPanel onImport={onImportKnown} />);
    };
    const id = setInterval(tick, 300);
    tick();
    return () => clearInterval(id);
  }, [onImportKnown]);

  return <DecksView onOpenDeck={onOpenDeck} onReviewDeck={onReviewDeck} onReviewAll={onReviewAll} />;
}

function KnownWordsPanel({ onImport }) {
  const known = FED_DATA.KNOWN_SERIES[FED_DATA.KNOWN_SERIES.length - 1];
  const fi = Math.floor(known * 0.62);
  const et = known - fi;
  return (
    <div className="panel" style={{ padding: "16px 20px", display: "flex", alignItems: "center", gap: 24, flexWrap: "wrap" }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
        <span className="mono" style={{ fontSize: 10, letterSpacing: "0.18em", textTransform: "uppercase", color: "var(--ink-mute)" }}>
          Known words
        </span>
        <span className="disp" style={{ fontSize: 28, fontWeight: 600, fontStyle: "italic", color: "var(--blue)", letterSpacing: "-0.02em", lineHeight: 1 }}>
          {known.toLocaleString()}
        </span>
      </div>
      <div style={{ height: 24, width: 1, background: "var(--line)" }} />
      <div className="mono" style={{ fontSize: 11, color: "var(--ink-mute)", letterSpacing: "0.04em", display: "flex", gap: 16 }}>
        <span><span className="tag fi" style={{ padding: "1px 5px", fontSize: 9 }}>FI</span> <b style={{ color: "var(--ink)", fontWeight: 500 }}>{fi.toLocaleString()}</b></span>
        <span><span className="tag et" style={{ padding: "1px 5px", fontSize: 9 }}>ET</span> <b style={{ color: "var(--ink)", fontWeight: 500 }}>{et.toLocaleString()}</b></span>
      </div>
      <div style={{ flex: 1 }} />
      <button className="btn" onClick={onImport}>Import known words</button>
      <button className="btn btn-ghost">Export</button>
    </div>
  );
}

// ─── ReviewViewProto ────────────────────────────────────────────────
function ReviewViewProto({ scope, onScopeChange, onCorrect, onExit, done, onComplete, onBackToDash }) {
  // Inject ✎ Wrong? button into the review card
  useEffectPS(() => {
    if (done) return;
    const tick = () => {
      const cards = document.querySelectorAll("main .panel");
      cards.forEach(card => {
        if (card.dataset.protoReviewWrong === "1") return;
        if (!card.querySelector(".disp") || !card.textContent.includes("toissapäivä") && !card.textContent.includes("pankki") && !card.textContent.includes("raamatu")) {
          // it's the review card if it has the .hl span inside a big .disp; check more loosely
        }
        const hl = card.querySelector(".disp .hl");
        if (!hl) return;
        const tagRow = card.firstElementChild;
        if (!tagRow) return;
        // Append a wrong button at the end of the tag row
        const btn = document.createElement("button");
        btn.textContent = "✎ Wrong?";
        btn.style.cssText = `
          all: unset; cursor: pointer; padding: 3px 10px;
          font-family: var(--font-mono); font-size: 11px; letter-spacing: 0.04em;
          color: var(--ink-mute); border: 1px solid var(--line); border-radius: 999px;
          margin-left: 6px;
          transition: color 0.12s, border-color 0.12s, background 0.12s;
        `;
        btn.addEventListener("mouseenter", () => { btn.style.color = "var(--blue)"; btn.style.borderColor = "var(--blue)"; });
        btn.addEventListener("mouseleave", () => { btn.style.color = "var(--ink-mute)"; btn.style.borderColor = "var(--line)"; });
        btn.addEventListener("click", () => {
          const lemma = hl.textContent.trim();
          const sentence = card.querySelector(".disp")?.textContent?.trim() || "";
          onCorrect && onCorrect({ lemma, sentence, pos: "NOUN" });
        });
        tagRow.appendChild(btn);
        card.dataset.protoReviewWrong = "1";
      });
    };
    const id = setInterval(tick, 300);
    tick();
    return () => clearInterval(id);
  }, [onCorrect, done]);

  // Tap a "Done" affordance to enter completion view (since the underlying review loops)
  useEffectPS(() => {
    if (done) return;
    const tick = () => {
      const head = document.querySelector("main .page-head .actions");
      if (!head || head.dataset.protoDone === "1") return;
      const btn = document.createElement("button");
      btn.className = "btn";
      btn.textContent = "End session ✓";
      btn.title = "Mark this session done (demo)";
      btn.addEventListener("click", () => onComplete && onComplete());
      head.insertBefore(btn, head.firstChild);
      head.dataset.protoDone = "1";
    };
    const id = setInterval(tick, 300);
    tick();
    return () => clearInterval(id);
  }, [done, onComplete]);

  if (done) {
    return <ReviewDoneCard count={scope === "all" ? 87 : 24} onBack={onBackToDash} />;
  }

  return <ReviewViewV2 scope={scope} onScopeChange={onScopeChange} onExit={onExit} />;
}

Object.assign(window, { LandingViewProto, DecksViewProto, ReviewViewProto, KnownWordsPanel });
