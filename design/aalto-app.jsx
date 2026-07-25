/* global React, ReactDOM, FED_DATA, LandingView, Inspector, AuthModal, DecksView, DeckDetailView, ReviewViewV2, LeverageViewV2, useTweaks, TweaksPanel, TweakSection, TweakRadio */

const { useState: useStateApp, useEffect: useEffectApp } = React;

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "theme": "paper",
  "aalto": "bold",
  "startSignedIn": false
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

  useEffectApp(() => {
    document.documentElement.setAttribute("data-theme", tweaks.theme === "ink" ? "ink" : "paper");
    document.documentElement.setAttribute("data-aalto", tweaks.aalto === "bold" ? "bold" : "restrained");
  }, [tweaks]);

  function gotoCreateDeck() {
    if (user) setRoute("decks");
    else { setAuthReason("Create a deck to save this word list, get FSRS review, and unlock cross-deck leverage."); setAuthOpen(true); }
  }
  function signIn() { setUser({ name: "sagar", initial: "S" }); setAuthOpen(false); setRoute("decks"); }
  function signOut() { setUser(null); setRoute("landing"); }

  const NAV = user ? [
    { id: "landing",  label: "Parse" },
    { id: "decks",    label: "Decks" },
    { id: "review",   label: "Review", badge: "87", action: () => { setReviewScope("all"); setRoute("review"); } },
    { id: "leverage", label: "Leverage" },
  ] : [];

  let body;
  if (route === "landing") {
    body = <LandingView onCreateDeck={gotoCreateDeck} onOpenWord={setInspectWord} aaltoMode={tweaks.aalto} />;
  } else if (route === "decks") {
    body = <DecksView
      onOpenDeck={(id) => { setDeckId(id); setRoute("deckDetail"); }}
      onReviewDeck={(id) => { setReviewScope(id); setRoute("review"); }}
      onReviewAll={() => { setReviewScope("all"); setRoute("review"); }}
    />;
  } else if (route === "deckDetail") {
    body = <DeckDetailView deckId={deckId} onBack={() => setRoute("decks")} onReview={(id) => { setReviewScope(id); setRoute("review"); }} />;
  } else if (route === "review") {
    body = <ReviewViewV2 scope={reviewScope} onScopeChange={setReviewScope} onExit={() => setRoute(user ? "decks" : "landing")} />;
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

        {user && (
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

      <div id="toast" className="toast">-</div>

      <TweaksPanel title="Tweaks" defaultPosition={{ right: 24, bottom: 24 }}>
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
              { value: "paper", label: "Paimio (light)" },
              { value: "ink", label: "Sanatorium (dark)" },
            ]} />
        </TweakSection>
        <TweakSection label="Demo state">
          <TweakRadio value={user ? "in" : "out"} onChange={v => v === "in" ? signIn() : signOut()}
            options={[
              { value: "out", label: "Signed out" },
              { value: "in", label: "Signed in" },
            ]} />
        </TweakSection>
      </TweaksPanel>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
