#!/usr/bin/env python3
"""
EstNLTK/Vabamorf adapter for FinEstDB.

Requires:
- `estnltk` installed in the interpreter running this script

The adapter intentionally emits the same JSON shape as the Rust FFI parser and
the Omorfi adapter, so the existing parser eval loop can compare
basic/custom/estnltk without special cases.
"""

import argparse
import json
import os
import re
import sys
from pathlib import Path
from typing import Any


CASE_LABELS = {
    "n": "nominative",
    "g": "genitive",
    "p": "partitive",
    "ill": "illative",
    "in": "inessive",
    "el": "elative",
    "all": "allative",
    "ad": "adessive",
    "abl": "ablative",
    "tr": "translative",
    "term": "terminative",
    "es": "essive",
    "ab": "abessive",
    "kom": "comitative",
}

POS_LABELS = {
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


def configure_local_caches() -> None:
    repo_root = Path(__file__).resolve().parents[1]
    nltk_data = repo_root / ".cache" / "nltk_data"
    mpl_config = repo_root / ".cache" / "matplotlib"
    if nltk_data.is_dir():
        os.environ.setdefault("NLTK_DATA", str(nltk_data))
    if mpl_config.is_dir():
        os.environ.setdefault("MPLCONFIGDIR", str(mpl_config))


def raw_value(annotation: Any, key: str, default: str = "") -> str:
    if isinstance(annotation, dict):
        value = annotation.get(key, default)
    else:
        value = getattr(annotation, key, default)
    if value is None:
        return default
    if isinstance(value, list):
        return str(value[0]) if value else default
    return str(value)


def best_annotation(span: Any) -> Any:
    annotations = getattr(span, "annotations", None)
    if annotations:
        return annotations[0]
    return span


def grammar_label(form: str) -> str:
    tags = form.split()
    if not tags:
        return ""
    return CASE_LABELS.get(tags[-1], "")


def token_for_span(span: Any) -> dict | None:
    annotation = best_annotation(span)
    form = getattr(span, "text", "")
    if not form:
        return None
    raw_pos = raw_value(annotation, "partofspeech")
    pos = POS_LABELS.get(raw_pos, "X")
    if pos == "PUNCT" or (not raw_pos and re.fullmatch(r"\W+", form, flags=re.UNICODE)):
        return {
            "form": form,
            "lemma": form,
            "pos": "PUNCT",
            "feats": {},
            "grammar_label": "PUNCT_CLOSE" if form in ".,;:!?)" else "",
            "mwe_id": None,
        }
    lemma = raw_value(annotation, "lemma") or raw_value(annotation, "root") or form.lower()
    vm_form = raw_value(annotation, "form")
    return {
        "form": form,
        "lemma": lemma.replace("_", ""),
        "pos": pos,
        "feats": {"vabamorf_form": vm_form} if vm_form else {},
        "grammar_label": grammar_label(vm_form),
        "mwe_id": None,
    }


def analyse_text(raw_text: str) -> list[dict]:
    from estnltk import Text

    text = Text(raw_text)
    text.tag_layer(["sentences", "words", "morph_analysis"])

    sentences: list[dict] = []
    for sentence in text["sentences"]:
        sent_start = sentence.start
        sent_end = sentence.end
        out_tokens = []
        for span in text["morph_analysis"]:
            if span.start < sent_start or span.end > sent_end:
                continue
            token = token_for_span(span)
            if token is not None:
                out_tokens.append(token)
        if out_tokens:
            sentences.append({"tokens": out_tokens})
    return sentences


def main() -> int:
    configure_local_caches()

    parser = argparse.ArgumentParser()
    parser.add_argument("--lang", required=True)
    args = parser.parse_args()

    if args.lang != "ET":
        print("adapter only supports ET", file=sys.stderr)
        return 2

    text = sys.stdin.read()
    if not text:
        print(json.dumps({"sentences": []}))
        return 0

    try:
        sentences = analyse_text(text)
    except ImportError:
        print("estnltk is not installed. Run `make setup-estnltk`.", file=sys.stderr)
        return 2

    print(json.dumps({"sentences": sentences}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
