/* global React, FED_DATA */

// Plausible inflected forms + total possible cases per lemma - for the "forms seen" column.
// Each form has its own learning status (known/learning/new) - learners study them as separate cards.
// In Finnish/Estonian, nouns have ~14 cases × sg/pl; verbs have many tense/person combos.
const FORMS_DATA = {
  "olla":     { total: 28, forms: [
    { f: "on", s: "known" }, { f: "oli", s: "known" }, { f: "olen", s: "known" },
    { f: "ollut", s: "learning" }, { f: "olisi", s: "learning" }, { f: "olkoon", s: "new" },
  ]},
  "tehdä":    { total: 32, forms: [
    { f: "tekee", s: "known" }, { f: "teki", s: "learning" }, { f: "tehnyt", s: "learning" },
    { f: "tehdään", s: "new" }, { f: "tehkää", s: "new" },
  ]},
  "voida":    { total: 32, forms: [
    { f: "voi", s: "known" }, { f: "voin", s: "learning" },
    { f: "voisi", s: "learning" }, { f: "voitiin", s: "new" },
  ]},
  "saada":    { total: 32, forms: [
    { f: "saa", s: "learning" }, { f: "sain", s: "learning" },
    { f: "saanut", s: "new" }, { f: "saadaan", s: "new" },
  ]},
  "sanoa":    { total: 32, forms: [
    { f: "sanoo", s: "learning" }, { f: "sanoi", s: "new" }, { f: "sanonut", s: "new" },
  ]},
  "tulla":    { total: 32, forms: [
    { f: "tulee", s: "known" }, { f: "tuli", s: "learning" }, { f: "tullut", s: "learning" },
    { f: "tulisi", s: "new" }, { f: "tulkaa", s: "new" },
  ]},
  "lukea":    { total: 32, forms: [
    { f: "lukee", s: "known" }, { f: "luki", s: "known" }, { f: "lukenut", s: "learning" },
    { f: "luettu", s: "new" }, { f: "luetaan", s: "new" },
  ]},
  "kysyä":    { total: 32, forms: [
    { f: "kysyy", s: "learning" }, { f: "kysyi", s: "new" }, { f: "kysynyt", s: "new" },
  ]},
  "ajatella": { total: 32, forms: [
    { f: "ajattelee", s: "new" }, { f: "ajatteli", s: "new" }, { f: "ajatellut", s: "new" },
  ]},
  "kaupunki": { total: 28, forms: [
    { f: "kaupunki", s: "known" }, { f: "kaupungin", s: "learning" },
    { f: "kaupungissa", s: "learning" }, { f: "kaupunkiin", s: "new" }, { f: "kaupungit", s: "new" },
  ]},
  "vuosi":    { total: 28, forms: [
    { f: "vuosi", s: "known" }, { f: "vuoden", s: "known" }, { f: "vuonna", s: "known" },
    { f: "vuotta", s: "learning" }, { f: "vuosia", s: "learning" }, { f: "vuosien", s: "new" },
  ]},
  "ihminen":  { total: 28, forms: [
    { f: "ihminen", s: "learning" }, { f: "ihmisen", s: "learning" },
    { f: "ihmistä", s: "new" }, { f: "ihmiset", s: "new" }, { f: "ihmisiä", s: "new" },
  ]},
  "lapsi":    { total: 28, forms: [
    { f: "lapsi", s: "new" }, { f: "lapsen", s: "new" },
    { f: "lasta", s: "new" }, { f: "lapset", s: "new" }, { f: "lapsia", s: "new" },
  ]},
  "huone":    { total: 28, forms: [
    { f: "huone", s: "new" }, { f: "huoneen", s: "new" }, { f: "huoneessa", s: "new" },
  ]},
  "ovi":      { total: 28, forms: [
    { f: "ovi", s: "learning" }, { f: "oven", s: "learning" },
    { f: "ovessa", s: "new" }, { f: "ovea", s: "new" },
  ]},
};

function LeverageViewV2() {
  const rank = FED_DATA.LEVERAGE_RANK;
  const maxTokens = Math.max(...rank.map(r => r.tokens));
  const totalUnlock = rank.slice(0, 10).reduce((a, r) => a + r.unlock, 0);

  return (
    <>
      <div className="page-head">
        <div>
          <h1>Leverage</h1>
          <div className="sub">Words ranked by how often they appear across your decks - learn these first for the biggest comprehension jump.</div>
        </div>
        <div className="actions">
          <button className="btn">Add top 10 to queue</button>
          <button className="btn btn-primary">Export ranked list</button>
        </div>
      </div>

      <div className="page-body">
        <div className="panel" style={{ padding: "22px 26px", marginBottom: 16 }}>
          <div className="mono" style={{ fontSize: 10, color: "var(--ink-mute)", letterSpacing: "0.14em", textTransform: "uppercase", marginBottom: 10 }}>
            If you learn the top 10
          </div>
          <div style={{ display: "flex", alignItems: "baseline", gap: 16 }}>
            <div className="disp num" style={{ fontSize: 56, fontWeight: 700, color: "var(--accent)", letterSpacing: "-0.03em", lineHeight: 1 }}>
              +{Math.round(totalUnlock * 100)}<span style={{ fontSize: 28 }}>%</span>
            </div>
            <div className="disp" style={{ fontSize: 17, lineHeight: 1.3, color: "var(--ink-soft)" }}>
              comprehension gain across your 5 active decks
            </div>
          </div>
        </div>

        <div className="panel">
          <table className="data-table">
            <thead>
              <tr>
                <th style={{ width: 36, textAlign: "right" }}>#</th>
                <th></th>
                <th>Dictionary form</th>
                <th>Definition</th>
                <th>Type</th>
                <th style={{ width: 280 }}>Forms · learn each as a card</th>
                <th style={{ width: 160 }}>Times it appears</th>
                <th style={{ width: 110 }}>Decks</th>
                <th style={{ width: 70, textAlign: "right" }}>+%</th>
                <th style={{ width: 90, textAlign: "right" }}></th>
              </tr>
            </thead>
            <tbody>
              {rank.map((r, i) => {
                const fd = FORMS_DATA[r.lemma] || { total: 28, forms: [{ f: r.lemma, s: r.status }] };
                const known = fd.forms.filter(x => x.s === "known").length;
                const learning = fd.forms.filter(x => x.s === "learning").length;
                return (
                <tr key={r.lemma}>
                  <td className="mono num" style={{ textAlign: "right", color: "var(--ink-mute)" }}>{String(i+1).padStart(2,"0")}</td>
                  <td><span className={`dot ${r.status}`} /></td>
                  <td>
                    <div className="mono" style={{ fontWeight: 600, fontSize: 14 }}>{r.lemma}</div>
                    <div className="mono" style={{ fontSize: 10.5, color: "var(--ink-mute)", marginTop: 3, letterSpacing: "0.04em" }}>
                      <b style={{ color: "var(--known)" }}>{known}</b> known · <b style={{ color: "var(--learning)" }}>{learning}</b> learning · {fd.forms.length}/{fd.total} seen
                    </div>
                  </td>
                  <td style={{ color: "var(--ink-soft)" }}>{r.gloss}</td>
                  <td><span className="tag">{r.pos}</span></td>
                  <td>
                    <div style={{ display: "flex", flexWrap: "wrap", gap: 4 }}>
                      {fd.forms.map(x => (
                        <span key={x.f} className={`form-pill ${x.s}`} title={`${x.s} - click to study`}>
                          <span className="form-dot" />
                          {x.f}
                        </span>
                      ))}
                      {fd.forms.length < fd.total && (
                        <span className="form-pill more" title={`${fd.total - fd.forms.length} forms not yet seen`}>
                          +{fd.total - fd.forms.length}
                        </span>
                      )}
                    </div>
                  </td>
                  <td>
                    <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                      <div style={{ flex: 1, height: 6, background: "var(--bg-raise)", borderRadius: 1, overflow: "hidden" }}>
                        <div style={{
                          width: `${(r.tokens / maxTokens) * 100}%`,
                          height: "100%",
                          background: r.status === "known" ? "var(--known)" : r.status === "learning" ? "var(--learning)" : "var(--accent)",
                        }} />
                      </div>
                      <span className="mono num" style={{ fontSize: 12, fontWeight: 600, minWidth: 44, textAlign: "right" }}>{r.tokens.toLocaleString()}</span>
                    </div>
                  </td>
                  <td>
                    <div style={{ display: "flex", gap: 3 }}>
                      {r.decks.map(d => {
                        const deck = FED_DATA.SAMPLE_DECKS.find(x => x.id === d);
                        return <span key={d} title={deck?.title} style={{ width: 14, height: 14, background: deck?.lang === "ET" ? "var(--et)" : "var(--fi)", borderRadius: 2 }} />;
                      })}
                    </div>
                  </td>
                  <td className="mono num" style={{ textAlign: "right", color: "var(--accent)", fontWeight: 600 }}>+{(r.unlock * 100).toFixed(1)}%</td>
                  <td style={{ textAlign: "right" }}><button className="btn" style={{ fontSize: 11, padding: "3px 10px" }}>+ Queue</button></td>
                </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

window.LeverageViewV2 = LeverageViewV2;
