#!/usr/bin/env python3
"""Persistent omorfi batch adapter (Python).

Reads surfaces (one per line) from stdin, writes one TSV line per surface
to stdout: surface\tlemma\tPOS\tfeats\tanalysis_count.

Loads omorfi analyser once at startup so per-surface latency is just
morphology lookup.

Invoked by Go via cmd/enrichcorpus when -lang fi.
"""
import sys
import os

ANALYSER_PATH = os.environ.get(
    'OMORFI_ANALYSER',
    os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'omorfi-data', 'omorfi.analyse.hfst'),
)

try:
    from omorfi import Omorfi
except Exception as e:
    print(f'FAIL: omorfi import error: {e}', file=sys.stderr)
    sys.exit(1)

o = Omorfi()
try:
    o.load_analyser(ANALYSER_PATH)
except Exception as e:
    print(f'FAIL: omorfi load_analyser({ANALYSER_PATH}): {e}', file=sys.stderr)
    sys.exit(1)

print('READY', file=sys.stderr, flush=True)


def parse_raw(raw):
    """Extract lemma, UPOS, and basic feats from omorfi's [TAG=VALUE]... raw output."""
    lemma = ''
    upos = ''
    feats_dict = {}
    parts = raw.replace('][', '|').strip('[]').split('|')
    for p in parts:
        if '=' not in p:
            continue
        k, v = p.split('=', 1)
        k = k.strip()
        v = v.strip()
        if k == 'WORD_ID':
            lemma = v
        elif k == 'UPOS':
            upos = v
        elif k == 'CASE':
            feats_dict['Case'] = v.title()
        elif k == 'NUM':
            feats_dict['Number'] = {'SG': 'Sing', 'PL': 'Plur'}.get(v, v)
        elif k == 'TENSE':
            feats_dict['Tense'] = {'PRESENT': 'Pres', 'PAST': 'Past'}.get(v, v.title())
        elif k == 'MOOD':
            feats_dict['Mood'] = {'INDV': 'Ind', 'COND': 'Cnd', 'IMPV': 'Imp', 'POTN': 'Pot'}.get(v, v.title())
        elif k == 'PERS':
            # PERS=SG1 / PL2 etc.
            if len(v) >= 3:
                num = {'SG': 'Sing', 'PL': 'Plur'}.get(v[:2], '')
                if num:
                    feats_dict['Number'] = num
                feats_dict['Person'] = v[2:]
        elif k == 'VOICE':
            feats_dict['Voice'] = {'ACT': 'Act', 'PSS': 'Pass'}.get(v, v.title())
    feats = '|'.join(f'{k}={v}' for k, v in sorted(feats_dict.items()))
    return lemma, upos, feats


for line in sys.stdin:
    surface = line.strip()
    if not surface:
        continue
    try:
        analyses = list(o.analyse(surface))
        if not analyses:
            sys.stdout.write(f'{surface}\t\t\t\t0\n')
        else:
            # Take the first analysis as the "primary"; record total count.
            raw = analyses[0].raw if hasattr(analyses[0], 'raw') else str(analyses[0])
            lemma, upos, feats = parse_raw(raw)
            sys.stdout.write(f'{surface}\t{lemma}\t{upos}\t{feats}\t{len(analyses)}\n')
    except Exception:
        sys.stdout.write(f'{surface}\t\t\t\t0\n')
    sys.stdout.flush()
