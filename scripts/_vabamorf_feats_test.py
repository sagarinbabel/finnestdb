"""Smoke tests for _vabamorf_feats.vabamorf_form_to_feats.

Run with: python3 scripts/_vabamorf_feats_test.py
Returns 0 on pass, 1 on first failure (one-line report each).
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _vabamorf_feats import vabamorf_form_to_feats


CASES = [
    # Nominal: number + case
    ("sg n", "S", {"Number": "Sing"}),                   # Nom implicit
    ("sg p", "S", {"Number": "Sing", "Case": "Par"}),
    ("pl ill", "S", {"Number": "Plur", "Case": "Ill"}),
    ("sg adt", "S", {"Number": "Sing", "Case": "Ill"}),  # aditive folds to Ill
    ("pl ad", "S", {"Number": "Plur", "Case": "Ade"}),
    ("sg term", "S", {"Number": "Sing", "Case": "Ter"}),
    ("sg kom", "S", {"Number": "Sing", "Case": "Com"}),

    # Verbal: present indicative active, person × number
    ("n",   "V", {"Person": "1", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"}),
    ("d",   "V", {"Person": "2", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"}),
    ("b",   "V", {"Person": "3", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"}),
    ("vad", "V", {"Person": "3", "Number": "Plur", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"}),

    # Verbal: past indicative active
    ("sin",  "V", {"Person": "1", "Number": "Sing", "Tense": "Past", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"}),
    ("sime", "V", {"Person": "1", "Number": "Plur", "Tense": "Past", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"}),

    # Conditional
    ("ks",    "V", {"Tense": "Pres", "Mood": "Cnd", "Voice": "Act", "VerbForm": "Fin"}),
    ("ksin",  "V", {"Person": "1", "Number": "Sing", "Tense": "Pres", "Mood": "Cnd", "Voice": "Act", "VerbForm": "Fin"}),

    # Imperative
    ("o",   "V", {"Person": "2", "Number": "Sing", "Tense": "Pres", "Mood": "Imp", "Voice": "Act", "VerbForm": "Fin"}),
    ("ge",  "V", {"Person": "2", "Number": "Plur", "Tense": "Pres", "Mood": "Imp", "Voice": "Act", "VerbForm": "Fin"}),

    # Non-finite
    ("da",  "V", {"VerbForm": "Inf"}),
    ("ma",  "V", {"VerbForm": "Sup", "Case": "Ill"}),
    ("mas", "V", {"VerbForm": "Sup", "Case": "Ine"}),
    ("des", "V", {"VerbForm": "Conv"}),
    ("nud", "V", {"VerbForm": "Part", "Tense": "Past", "Voice": "Act"}),
    ("tud", "V", {"VerbForm": "Part", "Tense": "Past", "Voice": "Pass"}),

    # Passive finite
    ("takse", "V", {"Tense": "Pres", "Mood": "Ind", "Voice": "Pass", "VerbForm": "Fin"}),
    ("ti",    "V", {"Tense": "Past", "Mood": "Ind", "Voice": "Pass", "VerbForm": "Fin"}),

    # Polarity (negative connegative form)
    ("neg b", "V", {"Person": "3", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin", "Polarity": "Neg"}),

    # Empty / unknown
    ("", "S", {}),
    ("", "V", {}),
    ("nonsense_code", "V", {}),

    # Edge: Vabamorf never emits "sg n" on a verb — that's a noun shape.
    # If the tagger mislabels POS as V, we still try to interpret each
    # token: "sg" is unknown in the verb table, "n" matches 1sg-pres-ind.
    # Documented for transparency; not a realistic input from Vabamorf.
    ("sg n", "V", {"Person": "1", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin"}),
]


def main() -> int:
    failures = 0
    for form, pos, want in CASES:
        got = vabamorf_form_to_feats(form, pos)
        if got != want:
            failures += 1
            print(f"FAIL form={form!r} pos={pos!r}")
            print(f"   got:  {got}")
            print(f"   want: {want}")
    if failures:
        print(f"\n{failures} failures")
        return 1
    print(f"OK: {len(CASES)} cases")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
