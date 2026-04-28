#!/usr/bin/env python3
"""
Omorfi adapter for FinEstDB.

Requires:
- `omorfi` installed in the interpreter running this script
- `OMORFI_ANALYSE_HFST` pointing at a real `omorfi.analyse.hfst` model
"""

import argparse
import json
import os
import re
import sys
from pathlib import Path

from omorfi import Omorfi, Token


CASE_LABELS = {
    "Nom": "nominative",
    "Gen": "genitive",
    "Par": "partitive",
    "Ine": "inessive",
    "Ela": "elative",
    "Ill": "illative",
    "Ade": "adessive",
    "Abl": "ablative",
    "All": "allative",
    "Ess": "essive",
    "Tra": "translative",
    "Abe": "abessive",
    "Com": "comitative",
    "Ins": "instructive",
}


def split_sentences(text: str) -> list[str]:
    parts = re.split(r"(?<=[.!?])\s+", text.strip())
    return [part for part in parts if part]


def best_analysis(token: Token):
    if not token.analyses:
        return None
    return min(token.analyses, key=lambda analysis: analysis.weight)


def grammar_label_from_ufeats(ufeats: dict) -> str:
    case = ufeats.get("Case")
    if not case:
        return ""
    return CASE_LABELS.get(case, case)


def load_model() -> Omorfi:
    model_path = os.environ.get("OMORFI_ANALYSE_HFST", "").strip()
    if not model_path:
        model_path = str(Path(".cache/omorfi/omorfi.analyse.hfst").resolve())
    if not Path(model_path).is_file():
        raise FileNotFoundError(
            f"Omorfi analyser model not found at {model_path}. "
            "Set OMORFI_ANALYSE_HFST to a valid omorfi.analyse.hfst path."
        )
    omorfi = Omorfi()
    omorfi.load_analyser(model_path)
    return omorfi


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--lang", required=True)
    args = parser.parse_args()

    if args.lang != "FI":
        print("adapter only supports FI", file=sys.stderr)
        return 2

    text = sys.stdin.read()
    if not text:
        print(json.dumps({"sentences": []}))
        return 0

    omorfi = load_model()
    out_sentences = []
    for sentence in split_sentences(text):
        out_tokens = []
        for ix, raw_token in enumerate(omorfi.tokenise_sentence(sentence), start=1):
            form = raw_token.surf or ""
            if not form:
                continue
            if re.fullmatch(r"\W+", form):
                out_tokens.append(
                    {
                        "form": form,
                        "lemma": form,
                        "pos": "PUNCT",
                        "feats": {},
                        "grammar_label": "PUNCT_CLOSE" if form in ".,;:!?)" else "",
                        "mwe_id": None,
                    }
                )
                continue

            token = Token(form)
            token.pos = ix
            omorfi.analyse(token)
            analysis = best_analysis(token)
            if analysis is None:
                lemma = form.lower()
                pos = "X"
                ufeats = {}
            else:
                lemmas = analysis.get_lemmas() or [form.lower()]
                lemma = lemmas[0]
                pos = analysis.get_upos() or "X"
                ufeats = analysis.get_ufeats() or {}

            out_tokens.append(
                {
                    "form": form,
                    "lemma": lemma,
                    "pos": pos,
                    "feats": ufeats,
                    "grammar_label": grammar_label_from_ufeats(ufeats),
                    "mwe_id": None,
                }
            )
        out_sentences.append({"tokens": out_tokens})

    print(json.dumps({"sentences": out_sentences}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
