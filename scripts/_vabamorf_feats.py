"""Vabamorf morph-form code → UD FEATS string.

Used by the EstNLTK adapter (scripts/estnltk_adapter_example.py) to
emit the same UD FEATS shape that the Omorfi adapter and the dict
layer produce. Without this, the adapter only emitted a raw
"vabamorf_form" string and the FEATS-attribute eval table was empty
for the estnltk parser mode.

Vabamorf encodes morphology as a whitespace-separated string in the
`form` attribute of each morph_analysis annotation. Examples:
    "sg n"     - singular nominative noun
    "pl ill"   - plural illative
    "b"        - 3sg present indicative verb
    "sin"      - 1sg past indicative verb
    "ks"       - conditional (no person/number)
    "ge"       - 2pl imperative
    "tud"      - past participle passive
    "ma"       - supine
    "des"      - gerund

Coverage philosophy mirrors cmd/importekilexdetails/feats.go: map every
code with a clear UD FEATS equivalent; return an empty dict for unknown
codes rather than guess. The result is a dict (not the pipe string)
because the EstNLTK adapter feeds it into the existing JSON shape that
parsecore.featsFromJSON converts to UD canonical form.

Polarity=Neg is propagated from a separate "neg" tag if the analyzer
emits one in the form string (Vabamorf flags the connegative form
this way when it sees the auxiliary).
"""

from __future__ import annotations

from typing import Dict


# ── Nominal: number ─────────────────────────────────────────────────────
NUMBER_CODES = {"sg": "Sing", "pl": "Plur"}

# ── Nominal: case (matches CASE_LABELS in the adapter, projected to UD) ─
CASE_CODES = {
    "n": "Nom",
    "g": "Gen",
    "p": "Par",
    "ill": "Ill",
    "adt": "Ill",  # aditive folds to illative (UD-EDT convention)
    "in": "Ine",
    "el": "Ela",
    "all": "All",
    "ad": "Ade",
    "abl": "Abl",
    "tr": "Tra",
    "es": "Ess",
    "ab": "Abe",
    "kom": "Com",
    "term": "Ter",
    "ter": "Ter",
}

# ── Verbal: finite forms encode person × number × tense × mood × voice ─
# Each entry is (Person, Number, Tense, Mood, Voice) - empty string when
# the code doesn't constrain that attribute.
FINITE_VERB_CODES: Dict[str, Dict[str, str]] = {
    # Present indicative active
    "n":   {"Person": "1", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    "d":   {"Person": "2", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    "b":   {"Person": "3", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    "me":  {"Person": "1", "Number": "Plur", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    "te":  {"Person": "2", "Number": "Plur", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    "vad": {"Person": "3", "Number": "Plur", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},

    # Past indicative active
    "sin":  {"Person": "1", "Number": "Sing", "Tense": "Past", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    "sid":  {"Person": "2", "Number": "Sing", "Tense": "Past", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    "s":    {"Person": "3", "Number": "Sing", "Tense": "Past", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    "sime": {"Person": "1", "Number": "Plur", "Tense": "Past", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    "site": {"Person": "2", "Number": "Plur", "Tense": "Past", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"},
    # Vabamorf collapses 2sg-past and 3pl-past on "sid"; without context
    # we treat the singular reading as primary above (sid in singular row).
    # The plural reading would need disambiguation elsewhere.

    # Conditional (Knd)
    "ks":    {"Tense": "Pres", "Mood": "Cnd", "Voice": "Act", "VerbForm": "Fin"},
    "ksin":  {"Person": "1", "Number": "Sing", "Tense": "Pres", "Mood": "Cnd", "Voice": "Act", "VerbForm": "Fin"},
    "ksid":  {"Person": "2", "Number": "Sing", "Tense": "Pres", "Mood": "Cnd", "Voice": "Act", "VerbForm": "Fin"},
    "ksime": {"Person": "1", "Number": "Plur", "Tense": "Pres", "Mood": "Cnd", "Voice": "Act", "VerbForm": "Fin"},
    "ksite": {"Person": "2", "Number": "Plur", "Tense": "Pres", "Mood": "Cnd", "Voice": "Act", "VerbForm": "Fin"},

    # Imperative (Imp)
    "o":   {"Person": "2", "Number": "Sing", "Tense": "Pres", "Mood": "Imp", "Voice": "Act", "VerbForm": "Fin"},
    "gem": {"Person": "1", "Number": "Plur", "Tense": "Pres", "Mood": "Imp", "Voice": "Act", "VerbForm": "Fin"},
    "ge":  {"Person": "2", "Number": "Plur", "Tense": "Pres", "Mood": "Imp", "Voice": "Act", "VerbForm": "Fin"},
    "gu":  {"Person": "3",                   "Tense": "Pres", "Mood": "Imp", "Voice": "Act", "VerbForm": "Fin"},

    # Quotative (Qot) - Estonian's hearsay mood (Vabamorf calls this
    # "mineviku oblique"). Codes vary across EstNLTK versions; common ones:
    "vat":  {"Tense": "Pres", "Mood": "Qot", "Voice": "Act", "VerbForm": "Fin"},
    "nuvat": {"Tense": "Past", "Mood": "Qot", "Voice": "Act", "VerbForm": "Fin"},

    # Passive present/past - the personal-form codes "takse"/"ti" alone
    "takse": {"Tense": "Pres", "Mood": "Ind", "Voice": "Pass", "VerbForm": "Fin"},
    "ti":    {"Tense": "Past", "Mood": "Ind", "Voice": "Pass", "VerbForm": "Fin"},
    "tagu":  {"Tense": "Pres", "Mood": "Imp", "Voice": "Pass", "VerbForm": "Fin"},
    "taks":  {"Tense": "Pres", "Mood": "Cnd", "Voice": "Pass", "VerbForm": "Fin"},
    "tavat": {"Tense": "Pres", "Mood": "Qot", "Voice": "Pass", "VerbForm": "Fin"},
}

# ── Verbal: non-finite forms ────────────────────────────────────────────
NONFINITE_VERB_CODES: Dict[str, Dict[str, str]] = {
    "da":  {"VerbForm": "Inf"},
    "des": {"VerbForm": "Conv"},  # gerund / converb
    "ma":  {"VerbForm": "Sup", "Case": "Ill"},   # ma-form is illative supine
    "mas": {"VerbForm": "Sup", "Case": "Ine"},
    "mast": {"VerbForm": "Sup", "Case": "Ela"},
    "maks": {"VerbForm": "Sup", "Case": "Tra"},
    "mata": {"VerbForm": "Sup", "Case": "Abe"},
    "tud":  {"VerbForm": "Part", "Tense": "Past", "Voice": "Pass"},
    "nud":  {"VerbForm": "Part", "Tense": "Past", "Voice": "Act"},
    "tav":  {"VerbForm": "Part", "Tense": "Pres", "Voice": "Pass"},
    "v":    {"VerbForm": "Part", "Tense": "Pres", "Voice": "Act"},
    "dav":  {"VerbForm": "Part", "Tense": "Pres", "Voice": "Pass"},
}


def vabamorf_form_to_feats(form: str, partofspeech: str = "") -> Dict[str, str]:
    """Project a Vabamorf form code (whitespace-separated tag string) into
    a UD FEATS dict.

    Args:
        form: the raw form attribute (e.g. "sg n", "b", "ks", "tud").
        partofspeech: Vabamorf POS letter (S/V/A/...). Used to gate
            interpretation: the same code can mean different things on
            different POS (e.g. "n" is nominative on a noun, 1sg on a verb).

    Returns:
        A dict like {"Case": "Ine", "Number": "Sing"} with attributes in
        no particular order (caller normalises). Empty dict if no
        productive morphology.
    """
    if not form:
        return {}
    feats: Dict[str, str] = {}
    tokens = form.split()
    pos = (partofspeech or "").upper()
    is_verb = pos == "V"
    is_noun_like = pos in ("S", "H", "A", "C", "U", "P", "N", "O")

    # Pass 1: nominal number/case if POS suggests it.
    if is_noun_like or not is_verb:
        for tok in tokens:
            if tok in NUMBER_CODES:
                feats["Number"] = NUMBER_CODES[tok]
            elif tok in CASE_CODES:
                udcase = CASE_CODES[tok]
                if udcase != "Nom":
                    feats["Case"] = udcase
                # When Nom: leave Case unset per UD convention. Number
                # may still come through.

    # Pass 2: verbal codes. The form string is usually a single token
    # for finite verbs, or a participle/converb keyword for non-finite.
    if is_verb:
        joined = " ".join(tokens)
        # Try the whole form first, then individual tokens.
        if joined in FINITE_VERB_CODES:
            feats.update(FINITE_VERB_CODES[joined])
        elif joined in NONFINITE_VERB_CODES:
            feats.update(NONFINITE_VERB_CODES[joined])
        else:
            # Sometimes Vabamorf concatenates a polarity flag: "neg b" etc.
            for tok in tokens:
                if tok == "neg":
                    feats["Polarity"] = "Neg"
                    continue
                if tok in FINITE_VERB_CODES:
                    feats.update(FINITE_VERB_CODES[tok])
                elif tok in NONFINITE_VERB_CODES:
                    feats.update(NONFINITE_VERB_CODES[tok])

    # Polarity=Neg may appear as a bare "neg" token even on non-verbs
    # (rare) - fold it in.
    if "neg" in tokens and "Polarity" not in feats:
        feats["Polarity"] = "Neg"

    return feats


def vabamorf_pos_to_upos(partofspeech: str, _form: str = "") -> str:
    """POS letter → UPOS. Mirrors the POS_LABELS dict used in the
    EstNLTK adapter; centralised here so callers can rely on a single
    mapping."""
    return _POS_LABELS.get(partofspeech, "X")


_POS_LABELS = {
    "S": "NOUN",
    "H": "PROPN",
    "A": "ADJ",
    "C": "ADJ",
    "U": "ADJ",
    "N": "NUM",
    "O": "NUM",
    "P": "PRON",
    "V": "VERB",
    "D": "ADV",
    "J": "CCONJ",
    "K": "ADP",
    "I": "INTJ",
    "Y": "X",
    "Z": "PUNCT",
}
