/* global React, ReactDOM, FED_DATA, LandingViewProto, Inspector, AuthModal,
   DecksViewProto, DeckDetailView, ReviewViewProto, LeverageViewV2, useTweaks, TweaksPanel,
   TweakSection, TweakRadio, TweakToggle, ColdStart, KnownWordsImport, CorrectionModal */

const { useState: useStateApp, useEffect: useEffectApp } = React;

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "theme": "paper",
  "aalto": "bold",
  "startSignedIn": false,
  "coldStart": true,
  "ephemeral": false
}/*EDITMODE-END*/;

function App() {
  const [tweaks, setTweak] = useTweaks(TWEAK_DEFAULTS);
  const [user, setUser] = useStateApp(tweaks.startSignedIn ? { name: "sagar", initial: "S" } : null);
  const [route, setRoute] = useStateApp("landing");
  const [deckId, setDeckId] = useStateApp(null);
  const [reviewScope, setReviewScope] = useStateApp(null);
  const [authOpen, setAuthOpen] = useStateApp(false);
  const [authReason, setAuthReason] = useStateApp(null);
  const [inspectWord, setInspectWord] = useStateApp(null);

  // new prototype state
  const [hasDecks, setHasDecks] = useStateApp(true); // false → cold-start dashboard
  const [saveOpen, setSaveOpen] = useStateApp(false);
  const [saveCtx, setSaveCtx] = useStateApp(null); // { lang, count }
  const [correctOpen, setCorrectOpen] = useStateApp(false);
  const [correctCtx, setCorrectCtx] = useStateApp(null); // { lemma, pos, sentence, gloss }
  const [knownWordsOpen, setKnownWordsOpen] = useStateApp(false);
  const [reviewDone, setReviewDone] = useStateApp(false);

  useEffectApp(() => {
    document.documentElement.setAttribute("data-theme", tweaks.theme === "ink" ? "ink" : "paper");
    document.documentElement.setAttribute("data-aalto", tweaks.aalto === "bold" ? "bold" : "restrained");
  }, [tweaks]);

  // When toggling cold-start, reflect into hasDecks
  useEffectApp(() => { setHasDecks(!tweaks.coldStart); }, [tweaks.coldStart]);

  // ─── routing helpers ──────────────────────────────────────
  function openSaveAs(ctx) {
    if (!user) {
      setAuthReason("Create a free account to save these words. Your parse carries forward.");
      setAuthOpen(true);
      return;
    }
    setSaveCtx(ctx);
    setSaveOpen(true);
  }
  function openCorrection(ctx) {
    setCorrectCtx(ctx);
    setCorrectOpen(true);
  }
  function signIn() {
    setUser({ name: "sagar", initial: "S" });
    setAuthOpen(false);
    // From the flow: "carry forward anonymous parses" - drop them on the dashboard
    setRoute(hasDecks ? "decks" : "decks");
  }
  function signOut() { setUser(null); setRoute("landing"); }

  function gotoCreateDeck(ctx) { openSaveAs(ctx || { lang: "FI", count: 24 }); }

  function onSaveAsCreated(result) {
    // From the flow: SP → DD (deck detail)
    if (result.kind === "new") {
      setDeckId("new-konsta-04");
      setRoute("deckDetail");
    } else {
      setDeckId(result.deckId);
      setRoute("deckDetail");
    }
  }

  const NAV = user ? [
    { id: "landing",  label: "Parse" },
    { id: "decks",    label: "Decks" },
    { id: "review",   label: "Review", badge: "87", action: () => { setReviewScope("all"); setReviewDone(false); setRoute("review"); } },
    { id: "leverage", label: "Leverage" },
  ] : [];

  let body;
  if (route === "landing") {
    body = <LandingViewProto
      onCreateDeck={gotoCreateDeck}
      onOpenWord={setInspectWord}
      onCorrect={openCorrection}
      ephemeral={tweaks.ephemeral}
      onEphemeralChange={(v) => setTweak("ephemeral", v)}
      isAnonymous={!user}
      aaltoMode={tweaks.aalto} />;
  } else if (route === "decks") {
    if (!hasDecks) {
      body = <ColdStart
        onPasteText={() => setRoute("landing")}
        onImportKnown={() => setKnownWordsOpen(true)}
        onSeedTop1000={() => {}} />;
    } else {
      body = <DecksViewProto
        onOpenDeck={(id) => { setDeckId(id); setRoute("deckDetail"); }}
        onReviewDeck={(id) => { setReviewScope(id); setReviewDone(false); setRoute("review"); }}
        onReviewAll={() => { setReviewScope("all"); setReviewDone(false); setRoute("review"); }}
        onNewFromText={() => setRoute("landing")}
        onImportKnown={() => setKnownWordsOpen(true)} />;
    }
  } else if (route === "deckDetail") {
    body = <DeckDetailView deckId={deckId}
      onBack={() => setRoute("decks")}
      onReview={(id) => { setReviewScope(id); setReviewDone(false); setRoute("review"); }} />;
  } else if (route === "review") {
    body = <ReviewViewProto
      scope={reviewScope}
      done={reviewDone}
      onScopeChange={setReviewScope}
      onCorrect={openCorrection}
      onComplete={() => setReviewDone(true)}
      onBackToDash={() => { setReviewDone(false); setRoute(user ? "decks" : "landing"); }}
      onExit={() => { setReviewDone(false); setRoute(user ? "decks" : "landing"); }} />;
  } else if (route === "leverage") {
    body = <LeverageViewV2 />;
  }

  const isCurrent = (id) => route === id || (id === "decks" && route === "deckDetail");

  return (
    <div className="shell">
      <header className="topbar">
        <div className="brand" onClick={() => setRoute(user ? "decks" : "landing")}>
          finnest<span className="dot">.</span>
        </div>

        {user && (
          <nav className="topnav">
            {NAV.map(n => (
              <button key={n.id}
                className={isCurrent(n.id) ? "active" : ""}
                onClick={() => n.action ? n.action() : setRoute(n.id)}>
                {n.label}{n.badge ? <span className="badge">{n.badge}</span> : null}
              </button>
            ))}
          </nav>
        )}

        <div className="topbar-spacer" />

        {user && hasDecks && (
          <div className="topbar-stats">
            <span>known <b>{FED_DATA.KNOWN_SERIES[FED_DATA.KNOWN_SERIES.length-1].toLocaleString()}</b></span>
            <span>due <b style={{ color: "var(--blue)" }}>{FED_DATA.TODAY_QUEUE.due}</b></span>
            <span>retention <b className="pos">{Math.round(FED_DATA.TODAY_QUEUE.retention_30d * 100)}%</b></span>
          </div>
        )}

        <button className="icon-btn"
          title={tweaks.theme === "paper" ? "Switch to Sanatorium (dark)" : "Switch to Paimio (light)"}
          onClick={() => setTweak("theme", tweaks.theme === "paper" ? "ink" : "paper")}>
          {tweaks.theme === "paper" ? "◐" : "◑"}
        </button>

        {user ? (
          <button className="avatar-btn" onClick={signOut} title="Sign out">
            <span className="av">{user.initial}</span>
            <span className="name">{user.name}</span>
          </button>
        ) : (
          <button className="signin" onClick={() => { setAuthReason(null); setAuthOpen(true); }}>
            Sign in
          </button>
        )}
      </header>

      <main className="main">{body}</main>

      <Inspector word={inspectWord} onClose={() => setInspectWord(null)} />
      <AuthModal open={authOpen} onClose={() => setAuthOpen(false)} onSignIn={signIn} reason={authReason} />

      {/* New prototype modals */}
      <SaveAsModal open={saveOpen} onClose={() => setSaveOpen(false)}
        lang={saveCtx?.lang || "FI"} count={saveCtx?.count || 0}
        onCreated={onSaveAsCreated} />
      <CorrectionModal open={correctOpen} onClose={() => setCorrectOpen(false)} target={correctCtx} />
      <KnownWordsImport open={knownWordsOpen} onClose={() => setKnownWordsOpen(false)}
        onImported={() => { setHasDecks(true); setTweak("coldStart", false); setRoute("decks"); }} />

      <div id="toast" className="toast">-</div>

      <TweaksPanel title="Tweaks" defaultPosition={{ right: 24, bottom: 24 }}>
        <TweakSection label="Demo state">
          <TweakRadio value={user ? "in" : "out"} onChange={v => v === "in" ? signIn() : signOut()}
            options={[
              { value: "out", label: "Signed out (anon)" },
              { value: "in", label: "Signed in" },
            ]} />
        </TweakSection>
        <TweakSection label="Account">
          <TweakRadio value={tweaks.coldStart ? "cold" : "warm"} onChange={v => setTweak("coldStart", v === "cold")}
            options={[
              { value: "warm", label: "Has decks" },
              { value: "cold", label: "Cold-start" },
            ]} />
        </TweakSection>
        <TweakSection label="Parse mode">
          <TweakRadio value={tweaks.ephemeral ? "eph" : "save"} onChange={v => setTweak("ephemeral", v === "eph")}
            options={[
              { value: "save", label: "Save to history" },
              { value: "eph", label: "Ephemeral" },
            ]} />
        </TweakSection>
        <TweakSection label="Aalto intensity">
          <TweakRadio value={tweaks.aalto} onChange={v => setTweak("aalto", v)}
            options={[
              { value: "restrained", label: "Restrained" },
              { value: "bold", label: "Bolder" },
            ]} />
        </TweakSection>
        <TweakSection label="Theme">
          <TweakRadio value={tweaks.theme} onChange={v => setTweak("theme", v)}
            options={[
              { value: "paper", label: "Paimio" },
              { value: "ink", label: "Sanatorium" },
            ]} />
        </TweakSection>
      </TweaksPanel>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
