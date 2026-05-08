#!/usr/bin/env python3
"""Persistent estnltk batch adapter.

Reads surfaces (one per line) from stdin, writes one TSV line per surface
to stdout: surface\tlemma\tPOS\tfeats\tanalysis_count.

Loads estnltk once at startup so per-surface latency is just morphology
lookup.

Invoked by Go via cmd/enrichcorpus when -lang et. Skips surfaces that
estnltk can't analyze (just writes empty fields).
"""
import sys
import json

try:
    from estnltk import Text
except Exception as e:
    print(f"FAIL: estnltk import error: {e}", file=sys.stderr)
    sys.exit(1)

# estnltk POS code → UD UPOS translation table.
# Source: https://github.com/EstNLTK/estnltk + UD treebanks for ET.
POS_MAP = {
    'A': 'ADJ',     'C': 'ADJ',    'G': 'ADJ',    # adjectival forms
    'D': 'ADV',     'X': 'ADV',
    'H': 'PROPN',
    'I': 'INTJ',
    'J': 'CCONJ',
    'K': 'ADP',
    'N': 'NUM',     'O': 'NUM',
    'P': 'PRON',
    'S': 'NOUN',
    'V': 'VERB',
    'Y': 'SYM',
    'Z': 'PUNCT',
}

def to_upos(pos: str) -> str:
    return POS_MAP.get(pos, pos)

print("READY", file=sys.stderr, flush=True)

for line in sys.stdin:
    surface = line.strip()
    if not surface:
        continue
    try:
        text = Text(surface).tag_layer(['morph_analysis'])
        analyses = text.morph_analysis
        if not analyses or len(analyses) == 0:
            sys.stdout.write(f"{surface}\t\t\t\t0\n")
            sys.stdout.flush()
            continue
        ann = analyses[0]
        if hasattr(ann, 'annotations') and len(ann.annotations) > 0:
            first = ann.annotations[0]
            lemma = getattr(first, 'lemma', '') or ''
            pos = getattr(first, 'partofspeech', '') or ''
            feats_dict = {}
            for attr in ('form', 'case', 'number', 'tense', 'voice', 'mood'):
                v = getattr(first, attr, None)
                if v:
                    feats_dict[attr] = v
            feats = '|'.join(f'{k}={v}' for k, v in sorted(feats_dict.items()))
            count = len(ann.annotations)
            sys.stdout.write(f"{surface}\t{lemma}\t{to_upos(pos)}\t{feats}\t{count}\n")
        else:
            sys.stdout.write(f"{surface}\t\t\t\t0\n")
    except Exception:
        sys.stdout.write(f"{surface}\t\t\t\t0\n")
    sys.stdout.flush()
