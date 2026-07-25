#!/usr/bin/env python3
"""Persistent vabamorf batch adapter (Python).

Reads surfaces (one per line) from stdin, writes one TSV line per surface
to stdout: surface\tlemma\tUPOS\tfeats\tanalysis_count.

Uses vabamorf directly (the C++ Estonian morphology engine that estnltk
wraps internally). Lower-overhead than estnltk's Text-layer pipeline,
and a clean parallel to FI's omorfi adapter - both are the underlying
language-specific FST analyzers.

Invoked by Go via cmd/enrichcorpus when -lang et (preferred path).
Falls back to estnltk-batch.py if vabamorf direct import fails.
"""
import sys

try:
    from estnltk.vabamorf.morf import Vabamorf
except Exception as e:
    print(f'FAIL: vabamorf import error: {e}', file=sys.stderr)
    sys.exit(1)

# Estonian POS code → UD UPOS translation (same map as estnltk-batch.py).
POS_MAP = {
    'A': 'ADJ',     'C': 'ADJ',    'G': 'ADJ',
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

# vabamorf form strings → UD FEATS-ish translation.
# vabamorf uses short codes like "sg n" (sing nominative) or "pl g"
# (plur genitive) or "Pres", etc. UD FEATS is "Case=Nom|Number=Sing".
NUM_MAP = {'sg': 'Sing', 'pl': 'Plur'}
CASE_MAP = {
    'n': 'Nom', 'g': 'Gen', 'p': 'Par',
    'ill': 'Ill', 'in': 'Ine', 'el': 'Ela',
    'all': 'All', 'ad': 'Ade', 'abl': 'Abl',
    'tr': 'Tra', 'ter': 'Ter', 'es': 'Ess',
    'ab': 'Abe', 'kom': 'Com',
}

def parse_form(form: str) -> str:
    """Translate vabamorf 'form' field to UD FEATS-ish."""
    if not form or form == '?':
        return ''
    feats = {}
    parts = form.split()
    for p in parts:
        if p in NUM_MAP:
            feats['Number'] = NUM_MAP[p]
        elif p in CASE_MAP:
            feats['Case'] = CASE_MAP[p]
        elif p in ('Pres', 'Past'):
            feats['Tense'] = p
        elif p in ('o', 'b', 'd', 'da', 'tav', 'tud', 'mata'):
            # Various Estonian verb forms - leave as InfForm
            feats['InfForm'] = p
    return '|'.join(f'{k}={v}' for k, v in sorted(feats.items()))

morph = Vabamorf()
print('READY', file=sys.stderr, flush=True)

for line in sys.stdin:
    surface = line.strip()
    if not surface:
        continue
    try:
        result = morph.analyze([surface])
        if not result or not result[0].get('analysis'):
            sys.stdout.write(f'{surface}\t\t\t\t0\n')
        else:
            analyses = result[0]['analysis']
            # Take the first analysis for primary; record total count.
            first = analyses[0]
            lemma = first.get('lemma', '')
            pos = to_upos(first.get('partofspeech', ''))
            feats = parse_form(first.get('form', ''))
            sys.stdout.write(f'{surface}\t{lemma}\t{pos}\t{feats}\t{len(analyses)}\n')
    except Exception:
        sys.stdout.write(f'{surface}\t\t\t\t0\n')
    sys.stdout.flush()
