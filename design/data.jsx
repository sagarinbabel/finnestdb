// Sample data drawn from finnestdb docs - Finnish/Estonian text + parse outputs
// This is mock data shaped like what the real parsecore would return.

const FI_TEXT_1 = `Toissapäivänä menin pankkiin. Osoittautui, että se oli kiinni.
No, leuka rintaan ja kohti uusia pettymyksiä! Kirjassani luin,
että lentokenttäbussi lähtee aamukahdeksalta. Kaupassa myydään
tuoretta leipää, mutta kahvilassa istuminen on mukavampaa.`;

const FI_TEXT_2 = `Helsingin yliopistossa opiskelee tuhansia ulkomaalaisia.
Suomen kieli on agglutinoiva, mikä tarkoittaa että sanoihin
liitetään useita päätteitä peräkkäin. Esimerkiksi sana
"taloissammekinko" sisältää viisi morfeemia.`;

const ET_TEXT_1 = `Eile käisin raamatukogus. Sõbranna soovitas mulle
uut romaani, mille autor on tundmatu. Õhtul jõin kohvi
ja lugesin kuni keskööni. Tallinna vanalinn on alati
ilus, eriti sügiseti kui lehed langevad.`;

// Parse result for FI_TEXT_1 - modeled on parsecore output shape
const SAMPLE_PARSE_FI = {
  text: FI_TEXT_1,
  language: "FI",
  parser: "custom",
  duration_ms: 47,
  coverage: 0.91,
  total_tokens: 32,
  unique_lemmas: 24,
  sentences: [
    { id: 0, text: "Toissapäivänä menin pankkiin." },
    { id: 1, text: "Osoittautui, että se oli kiinni." },
    { id: 2, text: "No, leuka rintaan ja kohti uusia pettymyksiä!" },
    { id: 3, text: "Kirjassani luin, että lentokenttäbussi lähtee aamukahdeksalta." },
    { id: 4, text: "Kaupassa myydään tuoretta leipää, mutta kahvilassa istuminen on mukavampaa." },
  ],
  words: [
    {
      lemma: "toissapäivä", pos: "NOUN", forms: ["Toissapäivänä"], gloss: "the day before yesterday",
      grammar: "Essive sg", count: 1, sentence_id: 0, status: "new", leverage: 12,
      morph: [{ stem: "toissapäivä", suffix: "nä", note: "essive case" }]
    },
    {
      lemma: "mennä", pos: "VERB", forms: ["menin"], gloss: "to go",
      grammar: "1sg past", count: 1, sentence_id: 0, status: "known", leverage: 184,
      morph: [{ stem: "men", suffix: "in", note: "past 1sg" }]
    },
    {
      lemma: "pankki", pos: "NOUN", forms: ["pankkiin"], gloss: "bank (financial institution)",
      grammar: "Illative sg", count: 1, sentence_id: 0, status: "learning", leverage: 41,
      morph: [{ stem: "pankki", suffix: "in", note: "illative case" }]
    },
    {
      lemma: "osoittautua", pos: "VERB", forms: ["Osoittautui"], gloss: "to turn out, prove to be",
      grammar: "3sg past", count: 1, sentence_id: 1, status: "new", leverage: 22,
      morph: [{ stem: "osoittautu", suffix: "i", note: "past 3sg" }]
    },
    {
      lemma: "se", pos: "PRON", forms: ["se"], gloss: "it, that",
      grammar: "Nominative sg", count: 1, sentence_id: 1, status: "known", leverage: 904,
      morph: [{ stem: "se", suffix: "", note: "base form" }]
    },
    {
      lemma: "olla", pos: "VERB", forms: ["oli", "on"], gloss: "to be",
      grammar: "3sg past + pres", count: 2, sentence_id: 1, status: "known", leverage: 1240,
      morph: [{ stem: "ol", suffix: "i", note: "past 3sg" }]
    },
    {
      lemma: "kiinni", pos: "ADV", forms: ["kiinni"], gloss: "closed, shut",
      grammar: "Adverb", count: 1, sentence_id: 1, status: "learning", leverage: 31,
      morph: [{ stem: "kiinni", suffix: "", note: "base form" }]
    },
    {
      lemma: "leuka rintaan ja kohti uusia pettymyksiä", pos: "MWE", forms: ["leuka rintaan…"],
      gloss: "chin up and onwards to new disappointments (idiom)",
      grammar: "Multi-word expression", count: 1, sentence_id: 2, status: "new", leverage: 4,
      morph: [{ stem: "[idiom]", suffix: "", note: "5-token MWE - PMI 8.4" }]
    },
    {
      lemma: "kirja", pos: "NOUN", forms: ["Kirjassani"], gloss: "book",
      grammar: "Inessive sg + 1sg poss", count: 1, sentence_id: 3, status: "learning", leverage: 67,
      morph: [
        { stem: "kirja", suffix: "ssa", note: "inessive case" },
        { stem: "+", suffix: "ni", note: "1sg possessive (stripped)" }
      ]
    },
    {
      lemma: "lukea", pos: "VERB", forms: ["luin"], gloss: "to read",
      grammar: "1sg past", count: 1, sentence_id: 3, status: "known", leverage: 312,
      morph: [{ stem: "lu", suffix: "in", note: "past 1sg" }]
    },
    {
      lemma: "lentokenttäbussi", pos: "NOUN", forms: ["lentokenttäbussi"],
      gloss: "airport bus",
      grammar: "Compound (3-part)", count: 1, sentence_id: 3, status: "new", leverage: 2,
      morph: [
        { stem: "lento", suffix: "", note: "compound part 1 - flight" },
        { stem: "kenttä", suffix: "", note: "compound part 2 - field" },
        { stem: "bussi", suffix: "", note: "compound part 3 - bus" }
      ]
    },
    {
      lemma: "lähteä", pos: "VERB", forms: ["lähtee"], gloss: "to leave, depart",
      grammar: "3sg present", count: 1, sentence_id: 3, status: "learning", leverage: 88,
      morph: [{ stem: "lähte", suffix: "e", note: "3sg present" }]
    },
    {
      lemma: "aamukahdeksan", pos: "NUM", forms: ["aamukahdeksalta"],
      gloss: "eight in the morning",
      grammar: "Ablative sg", count: 1, sentence_id: 3, status: "new", leverage: 3,
      morph: [
        { stem: "aamu", suffix: "", note: "compound - morning" },
        { stem: "kahdeksa", suffix: "lta", note: "ablative case" }
      ]
    },
    {
      lemma: "kauppa", pos: "NOUN", forms: ["Kaupassa"], gloss: "shop, store",
      grammar: "Inessive sg", count: 1, sentence_id: 4, status: "learning", leverage: 54,
      morph: [{ stem: "kaupa", suffix: "ssa", note: "inessive - gradation pp→p" }]
    },
    {
      lemma: "myydä", pos: "VERB", forms: ["myydään"], gloss: "to sell",
      grammar: "Passive present", count: 1, sentence_id: 4, status: "new", leverage: 28,
      morph: [{ stem: "myy", suffix: "dään", note: "passive present" }]
    },
    {
      lemma: "tuore", pos: "ADJ", forms: ["tuoretta"], gloss: "fresh",
      grammar: "Partitive sg", count: 1, sentence_id: 4, status: "new", leverage: 19,
      morph: [{ stem: "tuore", suffix: "tta", note: "partitive case" }]
    },
    {
      lemma: "leipä", pos: "NOUN", forms: ["leipää"], gloss: "bread",
      grammar: "Partitive sg", count: 1, sentence_id: 4, status: "learning", leverage: 24,
      morph: [{ stem: "leipä", suffix: "ä", note: "partitive case" }]
    },
    {
      lemma: "kahvila", pos: "NOUN", forms: ["kahvilassa"], gloss: "café",
      grammar: "Inessive sg", count: 1, sentence_id: 4, status: "known", leverage: 71,
      morph: [{ stem: "kahvila", suffix: "ssa", note: "inessive case" }]
    },
    {
      lemma: "istua", pos: "VERB", forms: ["istuminen"], gloss: "to sit",
      grammar: "Verbal noun (4th inf)", count: 1, sentence_id: 4, status: "new", leverage: 36,
      morph: [{ stem: "istu", suffix: "minen", note: "4th infinitive nominal" }]
    },
    {
      lemma: "mukava", pos: "ADJ", forms: ["mukavampaa"], gloss: "comfortable, pleasant",
      grammar: "Comparative partitive", count: 1, sentence_id: 4, status: "learning", leverage: 14,
      morph: [
        { stem: "mukava", suffix: "mpa", note: "comparative" },
        { stem: "+", suffix: "a", note: "partitive case" }
      ]
    },
  ],
};

const SAMPLE_DECKS = [
  {
    id: "d-fi-konsta",
    title: "Konsta Punkkinen - vol I",
    lang: "FI",
    source: "Pasted text · 47k chars",
    created: "2026-04-12",
    tokens: 8420,
    unique: 1842,
    known: 612,
    learning: 384,
    new: 846,
    coverage: 0.71,
    leverage_top: 412,
    color: 1
  },
  {
    id: "d-fi-news",
    title: "Yle Uutiset - Apr 26",
    lang: "FI",
    source: "URL · yle.fi/news",
    created: "2026-04-26",
    tokens: 3104,
    unique: 891,
    known: 421,
    learning: 102,
    new: 368,
    coverage: 0.86,
    leverage_top: 198,
    color: 2
  },
  {
    id: "d-et-postimees",
    title: "Postimees - kolm artiklit",
    lang: "ET",
    source: "Pasted text · 12k chars",
    created: "2026-04-21",
    tokens: 2145,
    unique: 712,
    known: 188,
    learning: 94,
    new: 430,
    coverage: 0.62,
    leverage_top: 287,
    color: 3
  },
  {
    id: "d-fi-tove",
    title: "Tove Jansson - Muumipeikko",
    lang: "FI",
    source: "EPUB · muumipeikko.epub",
    created: "2026-03-30",
    tokens: 24800,
    unique: 4210,
    known: 1840,
    learning: 612,
    new: 1758,
    coverage: 0.78,
    leverage_top: 891,
    color: 4
  },
  {
    id: "d-et-vanasona",
    title: "Eesti vanasõnad",
    lang: "ET",
    source: "Anki import",
    created: "2026-04-02",
    tokens: 1820,
    unique: 1102,
    known: 412,
    learning: 168,
    new: 522,
    coverage: 0.69,
    leverage_top: 188,
    color: 5
  },
];

// Cross-deck leverage - same lemma showing up in multiple decks
const LEVERAGE_RANK = [
  { lemma: "olla",       pos: "VERB", gloss: "to be",                tokens: 1240, decks: ["d-fi-konsta","d-fi-news","d-fi-tove"], unlock: 0.041, status: "known" },
  { lemma: "tehdä",      pos: "VERB", gloss: "to do, make",           tokens: 612,  decks: ["d-fi-konsta","d-fi-news","d-fi-tove"], unlock: 0.018, status: "learning" },
  { lemma: "voida",      pos: "VERB", gloss: "to be able to, can",    tokens: 487,  decks: ["d-fi-konsta","d-fi-news","d-fi-tove"], unlock: 0.014, status: "learning" },
  { lemma: "saada",      pos: "VERB", gloss: "to get, receive",       tokens: 421,  decks: ["d-fi-konsta","d-fi-news","d-fi-tove"], unlock: 0.012, status: "new" },
  { lemma: "sanoa",      pos: "VERB", gloss: "to say",                tokens: 388,  decks: ["d-fi-konsta","d-fi-tove"], unlock: 0.011, status: "new" },
  { lemma: "tulla",      pos: "VERB", gloss: "to come",               tokens: 366,  decks: ["d-fi-konsta","d-fi-news","d-fi-tove"], unlock: 0.010, status: "learning" },
  { lemma: "lukea",      pos: "VERB", gloss: "to read",               tokens: 312,  decks: ["d-fi-konsta","d-fi-tove"], unlock: 0.009, status: "known" },
  { lemma: "kysyä",      pos: "VERB", gloss: "to ask",                tokens: 287,  decks: ["d-fi-konsta","d-fi-tove"], unlock: 0.008, status: "new" },
  { lemma: "ajatella",   pos: "VERB", gloss: "to think",              tokens: 254,  decks: ["d-fi-konsta","d-fi-tove"], unlock: 0.007, status: "new" },
  { lemma: "kaupunki",   pos: "NOUN", gloss: "city",                  tokens: 221,  decks: ["d-fi-news","d-fi-tove"],   unlock: 0.007, status: "learning" },
  { lemma: "vuosi",      pos: "NOUN", gloss: "year",                  tokens: 198,  decks: ["d-fi-konsta","d-fi-news"], unlock: 0.006, status: "known" },
  { lemma: "ihminen",    pos: "NOUN", gloss: "human, person",         tokens: 184,  decks: ["d-fi-konsta","d-fi-news","d-fi-tove"], unlock: 0.006, status: "learning" },
  { lemma: "lapsi",      pos: "NOUN", gloss: "child",                 tokens: 171,  decks: ["d-fi-konsta","d-fi-tove"], unlock: 0.005, status: "new" },
  { lemma: "huone",      pos: "NOUN", gloss: "room",                  tokens: 158,  decks: ["d-fi-tove"],               unlock: 0.005, status: "new" },
  { lemma: "ovi",        pos: "NOUN", gloss: "door",                  tokens: 142,  decks: ["d-fi-tove"],               unlock: 0.004, status: "learning" },
];

// Daily review queue
const TODAY_QUEUE = {
  due: 87,
  new_capacity: 12,
  new_backlog: 450,
  learning_steps: 18,
  predicted_minutes: 24,
  streak_days: 41,
  retention_30d: 0.918,
};

// Sparkline data - last 30 days of known-word count
const KNOWN_SERIES = [820, 832, 845, 851, 866, 880, 891, 904, 922, 938, 945, 961, 974, 988, 1002, 1015, 1031, 1042, 1056, 1071, 1083, 1098, 1112, 1129, 1148, 1162, 1183, 1198, 1216, 1234];

window.FED_DATA = {
  FI_TEXT_1, FI_TEXT_2, ET_TEXT_1,
  SAMPLE_PARSE_FI, SAMPLE_DECKS, LEVERAGE_RANK, TODAY_QUEUE, KNOWN_SERIES
};
