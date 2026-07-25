/* global React */
// Shared finnest mobile data - same words/cards across iOS, Android, mobile web.

const FED_M = {
  brand: { fi: "#3B82F6", et: "#06B6D4", accent: "#FF6B47", known: "#22C55E", learning: "#F59E0B", new: "#A855F7" },

  // Sample wordlist after parsing - 8 rows is enough for mobile screens
  words: [
    { lemma: "toissapäivä", forms: ["Toissapäivänä"], pos: "NOUN", gloss: "the day before yesterday", grammar: "Essive sg", count: 1, status: "new" },
    { lemma: "mennä",       forms: ["menin"],          pos: "VERB", gloss: "to go",                    grammar: "Past 1sg",  count: 1, status: "learning" },
    { lemma: "pankki",      forms: ["pankkiin"],       pos: "NOUN", gloss: "bank",                     grammar: "Illative sg", count: 1, status: "learning" },
    { lemma: "osoittautua", forms: ["Osoittautui"],    pos: "VERB", gloss: "to turn out, prove to be", grammar: "Past 3sg",  count: 1, status: "new" },
    { lemma: "kiinni",      forms: ["kiinni"],         pos: "ADV",  gloss: "closed, shut",             grammar: "-",         count: 1, status: "known" },
    { lemma: "muistaa",     forms: ["muistin"],        pos: "VERB", gloss: "to remember",              grammar: "Past 1sg",  count: 1, status: "learning" },
    { lemma: "hetkeksi",    forms: ["Hetkeksi"],       pos: "ADV",  gloss: "for a moment",             grammar: "Translative", count: 1, status: "new" },
    { lemma: "aukioloaika", forms: ["aukioloajat"],    pos: "NOUN", gloss: "opening hours",            grammar: "Plural nom", count: 1, status: "new" },
  ],

  // Sample review card
  card: {
    lemma: "pankki", lang: "FI", pos: "NOUN", grammar: "Illative sg (-iin)",
    gloss: "bank", sentence: "Toissapäivänä menin pankkiin.", highlight: "pankkiin",
    deck: "Konsta Punkkinen", morph: [{ stem: "pankki", suffix: "in" }],
    examples: [
      { fi: "Suomen suurin pankki sulki 30 konttoria.", en: "Finland's largest bank closed 30 branches." },
      { fi: "Mene pankkiin ja kysy lainasta.",          en: "Go to the bank and ask about a loan." },
    ],
  },

  sampleText: `Toissapäivänä menin pankkiin. Osoittautui, että se oli kiinni. Muistin liian myöhään, että aukioloajat ovat lyhentyneet. Hetkeksi tunsin itseni hölmöksi.`,
};

window.FED_M = FED_M;
