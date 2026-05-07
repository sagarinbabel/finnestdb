/* global React */
function AuthModal({ open, onClose, onSignIn, reason }) {
  if (!open) return null;
  return (
    <div className="modal-mask" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <div style={{ padding: "32px 32px 24px", textAlign: "center" }}>
          <div style={{
            width: 48, height: 48, margin: "0 auto 18px",
            border: "1px solid var(--line)", borderRadius: 12,
            display: "grid", placeItems: "center",
            background: "var(--bg-deep)",
          }}>
            <span className="disp" style={{ fontSize: 28, fontWeight: 800, color: "var(--accent)", letterSpacing: "-0.04em" }}>f.</span>
          </div>
          <div className="disp" style={{ fontSize: 24, fontWeight: 600, letterSpacing: "-0.02em", marginBottom: 8 }}>
            Sign in to save this deck
          </div>
          <div style={{ color: "var(--ink-soft)", fontSize: 14, lineHeight: 1.5, marginBottom: 24 }}>
            {reason || "Free accounts get unlimited decks, FSRS review, and cross-deck leverage ranking."}
          </div>

          <button
            onClick={onSignIn}
            style={{
              all: "unset",
              cursor: "pointer",
              width: "100%",
              padding: "12px 16px",
              background: "var(--ink)",
              color: "var(--bg-deep)",
              borderRadius: 6,
              fontWeight: 600,
              fontSize: 14,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              gap: 10,
              boxSizing: "border-box",
            }}
          >
            <svg width="18" height="18" viewBox="0 0 18 18">
              <path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.717v2.258h2.908c1.702-1.567 2.684-3.875 2.684-6.615z"/>
              <path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z"/>
              <path fill="#FBBC05" d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z"/>
              <path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58z"/>
            </svg>
            Continue with Google
          </button>

          <div className="mono" style={{ fontSize: 10.5, color: "var(--ink-mute)", marginTop: 20, letterSpacing: "0.04em", lineHeight: 1.5 }}>
            By signing in you agree to the terms.<br/>
            We never sell or share your texts.
          </div>
        </div>
        <div style={{ borderTop: "1px solid var(--line)", padding: "12px 16px", display: "flex", justifyContent: "space-between", alignItems: "center", background: "var(--bg-deep)" }}>
          <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-mute)", letterSpacing: "0.06em" }}>
            <span className="tag pro" style={{ marginRight: 8 }}>FREE</span> unlimited decks · review · sync
          </span>
          <button className="btn btn-ghost" onClick={onClose} style={{ fontSize: 11.5 }}>Maybe later</button>
        </div>
      </div>
    </div>
  );
}

window.AuthModal = AuthModal;
