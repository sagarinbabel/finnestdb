/* global React, ReactDOM, FED_DATA, ParseView, LibraryView, DeckView, ReviewView, LeverageView, useTweaks, TweaksPanel, TweakSection, TweakRadio, TweakToggle, TweakColor */

const { useState: useStateA, useEffect: useEffectA } = React;

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "theme": "ink",
  "accent": "vermillion",
  "density": "comfy",
  "displayFont": "fraunces",
  "monoFont": "plex"
}/*EDITMODE-END*/;

const NAV = [
  { id: "parse",    label: "Parse",    kbd: "P", icon: "tri" },
  { id: "library",  label: "Library",  kbd: "L", icon: "sq" },
  { id: "review",   label: "Review",   kbd: "R", icon: "cir", badge: "87" },
  { id: "leverage", label: "Leverage", kbd: "G", icon: "dia" },
];

function App() {
  const [tweaks, setTweak] = useTweaks(TWEAK_DEFAULTS);
  const [view, setView] = useStateA("parse");
  const [deckId, setDeckId] = useStateA(null);

  // Apply tweak: theme + accent + fonts
  useEffectA(() => {
    const root = document.documentElement;
    root.setAttribute("data-theme", tweaks.theme === "paper" ? "paper" : "ink");

    // Accent swap
    const accentMap = {
      vermillion: { a: "0.72 0.19 38", a2: "0.78 0.13 145", a3: "0.80 0.14 85", a4: "0.70 0.14 260" },
      cobalt:     { a: "0.62 0.18 255", a2: "0.78 0.13 145", a3: "0.80 0.14 85", a4: "0.72 0.19 38" },
      lime:       { a: "0.86 0.20 130", a2: "0.78 0.13 200", a3: "0.80 0.14 85", a4: "0.70 0.14 320" },
      magenta:    { a: "0.66 0.27 348", a2: "0.78 0.13 145", a3: "0.80 0.14 60", a4: "0.70 0.14 260" },
    };
    const m = accentMap[tweaks.accent] || accentMap.vermillion;
    root.style.setProperty("--accent", `oklch(${m.a})`);
    root.style.setProperty("--accent-2", `oklch(${m.a2})`);
    root.style.setProperty("--accent-3", `oklch(${m.a3})`);
    root.style.setProperty("--accent-4", `oklch(${m.a4})`);

    // Display font
    const dispMap = {
      fraunces: '"Fraunces", "Times New Roman", serif',
      plex: '"IBM Plex Sans", system-ui, sans-serif',
      mono: '"IBM Plex Mono", ui-monospace, monospace',
    };
    root.style.setProperty("--font-disp", dispMap[tweaks.displayFont] || dispMap.fraunces);

    // Density
    if (tweaks.density === "tight") {
      root.style.setProperty("--rail", "200px");
    } else {
      root.style.setProperty("--rail", "220px");
    }
  }, [tweaks]);

  // Keyboard nav
  useEffectA(() => {
    const onKey = (e) => {
      if (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA") return;
      const key = e.key.toLowerCase();
      const target = NAV.find(n => n.kbd.toLowerCase() === key);
      if (target) { setView(target.id); setDeckId(null); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  let mainContent;
  if (view === "library" && deckId) {
    mainContent = <DeckView deckId={deckId} onBack={() => setDeckId(null)} />;
  } else if (view === "library") {
    mainContent = <LibraryView onOpen={(id) => setDeckId(id)} />;
  } else if (view === "review") {
    mainContent = <ReviewView />;
  } else if (view === "leverage") {
    mainContent = <LeverageView />;
  } else {
    mainContent = <ParseView tweaks={tweaks} />;
  }

  const navTitle = NAV.find(n => n.id === view)?.label || "";

  return (
    <div className="app">
      {/* Rail */}
      <aside className="rail">
        <div className="rail-brand">
          <div className="mark">finnest<span className="dot">.</span></div>
          <div className="tag">α</div>
        </div>
        <div className="rail-section">Workbench</div>
        <nav className="rail-nav">
          {NAV.map(n => (
            <button
              key={n.id}
              className={`rail-link${view === n.id ? " active" : ""}`}
              onClick={() => { setView(n.id); setDeckId(null); }}
            >
              <span className={`rail-icon ${n.icon}`} />
              {n.label}
              {n.badge ? <span className="badge">{n.badge}</span> : <span className="kbd">{n.kbd}</span>}
            </button>
          ))}
        </nav>

        <div className="rail-section">Decks</div>
        <nav className="rail-nav" style={{ overflow: "auto", flexShrink: 1 }}>
          {FED_DATA.SAMPLE_DECKS.map(d => (
            <button
              key={d.id}
              className={`rail-link${view === "library" && deckId === d.id ? " active" : ""}`}
              onClick={() => { setView("library"); setDeckId(d.id); }}
              style={{ fontSize: 12.5 }}
            >
              <span style={{
                width: 8, height: 8, flex: "none",
                background: d.lang === "FI" ? "var(--fi)" : "var(--et)",
                borderRadius: 1,
              }} />
              <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", flex: 1, textAlign: "left" }}>
                {d.title}
              </span>
            </button>
          ))}
        </nav>

        <div className="rail-foot">
          <div className="avatar">SB</div>
          <div className="who">
            sagar<br/>
            <span className="em">{FED_DATA.TODAY_QUEUE.streak_days}d streak</span>
          </div>
        </div>
      </aside>

      {/* Topbar */}
      <header className="topbar">
        <div className="crumbs">
          <span>finnest</span>
          <span className="sep">/</span>
          <span className="now">{navTitle}</span>
          {deckId && <><span className="sep">/</span><span className="now">{FED_DATA.SAMPLE_DECKS.find(d=>d.id===deckId)?.title}</span></>}
        </div>
        <div className="topbar-spacer" />
        <div className="topbar-stats">
          <span>known <b>{FED_DATA.KNOWN_SERIES[FED_DATA.KNOWN_SERIES.length-1].toLocaleString()}</b></span>
          <span>due <b style={{ color: "var(--accent)" }}>{FED_DATA.TODAY_QUEUE.due}</b></span>
          <span>retention <b className="pos">{Math.round(FED_DATA.TODAY_QUEUE.retention_30d * 100)}%</b></span>
        </div>
        <div className="search">
          <span className="glyph">⌕</span>
          <input placeholder="Search lemma, deck, idiom…" />
          <span className="kbd">⌘K</span>
        </div>
        <button className="icon-btn" title="Theme" onClick={() => setTweak("theme", tweaks.theme === "paper" ? "ink" : "paper")}>
          {tweaks.theme === "paper" ? "◐" : "◑"}
        </button>
      </header>

      {/* Main */}
      <main className="main">
        {mainContent}
      </main>

      {/* Tweaks */}
      <TweaksPanel title="Tweaks" defaultPosition={{ right: 24, bottom: 24 }}>
        <TweakSection label="Theme">
          <TweakRadio value={tweaks.theme} onChange={v => setTweak("theme", v)}
            options={[{ value: "ink", label: "Ink" }, { value: "paper", label: "Paper" }]} />
        </TweakSection>
        <TweakSection label="Accent">
          <TweakRadio value={tweaks.accent} onChange={v => setTweak("accent", v)}
            options={[
              { value: "vermillion", label: "Vermillion" },
              { value: "cobalt", label: "Cobalt" },
              { value: "lime", label: "Lime" },
              { value: "magenta", label: "Magenta" },
            ]} />
        </TweakSection>
        <TweakSection label="Display font">
          <TweakRadio value={tweaks.displayFont} onChange={v => setTweak("displayFont", v)}
            options={[
              { value: "fraunces", label: "Fraunces" },
              { value: "plex", label: "Plex Sans" },
              { value: "mono", label: "Plex Mono" },
            ]} />
        </TweakSection>
        <TweakSection label="Density">
          <TweakRadio value={tweaks.density} onChange={v => setTweak("density", v)}
            options={[{ value: "comfy", label: "Comfy" }, { value: "tight", label: "Tight" }]} />
        </TweakSection>
      </TweaksPanel>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
