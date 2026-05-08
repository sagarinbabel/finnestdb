# Corpus pipeline Makefile.
#
# Operate from this directory:
#   cd corpus_pipeline && make <target>
#
# Every target has -fi, -et, and no-suffix (= both) variants per the plan's
# language conventions. Smoke profile is what's wired in v1; pilot and full
# need fetcher driving (deferred to next session).

.PHONY: help build smoke pilot full \
        extract-corpus extract-corpus-fi extract-corpus-et \
        aggregate-corpus aggregate-corpus-fi aggregate-corpus-et \
        corpus-verify corpus-verify-fi corpus-verify-et \
        corpus-promote corpus-promote-fi corpus-promote-et \
        corpus-cache-clear

# Default profile passed to corpus-verify / corpus-promote.
PROFILE ?= smoke

help:
	@echo "Corpus pipeline targets (run from corpus_pipeline/):"
	@echo "  make build                     - go build ./..."
	@echo "  make extract-corpus[-fi|-et]   - run extractcorpus"
	@echo "  make aggregate-corpus[-fi|-et] - run aggregatecorpus"
	@echo "  make corpus-verify[-fi|-et]    - run corpusverify (PROFILE=smoke|pilot|full)"
	@echo "  make corpus-promote[-fi|-et]   - run corpuspromote (PROFILE=smoke for now)"
	@echo "  make corpus-cache-clear        - wipe _derived/cache/"

build:
	go build ./...

# ─── extract ─────────────────────────────────────────────────────────────

extract-corpus: extract-corpus-fi extract-corpus-et

extract-corpus-fi:
	go run ./cmd/extractcorpus -lang fi

extract-corpus-et:
	go run ./cmd/extractcorpus -lang et

# ─── aggregate ───────────────────────────────────────────────────────────

aggregate-corpus: aggregate-corpus-fi aggregate-corpus-et

aggregate-corpus-fi:
	go run ./cmd/aggregatecorpus -lang fi

aggregate-corpus-et:
	go run ./cmd/aggregatecorpus -lang et

# ─── verify ──────────────────────────────────────────────────────────────

corpus-verify: corpus-verify-fi corpus-verify-et

corpus-verify-fi:
	go run ./cmd/corpusverify -lang fi -profile $(PROFILE)

corpus-verify-et:
	go run ./cmd/corpusverify -lang et -profile $(PROFILE)

# ─── promote (smoke → pilot → full ladder) ────────────────────────────────

corpus-promote: corpus-promote-fi corpus-promote-et

corpus-promote-fi:
	go run ./cmd/corpuspromote -lang fi -profile $(PROFILE)

corpus-promote-et:
	go run ./cmd/corpuspromote -lang et -profile $(PROFILE)

# ─── cache management ────────────────────────────────────────────────────

corpus-cache-clear:
	rm -rf ../localdata/fi-corpus/_derived/cache ../localdata/et-corpus/_derived/cache
	@echo "Caches cleared."

# ─── fetch ──────────────────────────────────────────────────────────────

.PHONY: fetch-corpus fetch-corpus-fi fetch-corpus-et fetch-corpus-dry-run

fetch-corpus: fetch-corpus-fi fetch-corpus-et

fetch-corpus-fi:
	go run ./cmd/fetchcorpus -lang fi

fetch-corpus-et:
	go run ./cmd/fetchcorpus -lang et

fetch-corpus-dry-run:
	go run ./cmd/fetchcorpus -lang fi -dry-run
	go run ./cmd/fetchcorpus -lang et -dry-run

# ─── bootstrap tarball (3-way split) ────────────────────────────────────
#
# Produces 3 tarballs, since the total ~50-80 GB compressed is impractical
# as a single file:
#   finnestdb-bootstrap-code.tgz  — finnestdb.db + small derived files (~2 GB)
#   finnestdb-bootstrap-fi.tgz    — localdata/fi-corpus/   (~25-40 GB)
#   finnestdb-bootstrap-et.tgz    — localdata/et-corpus/   (~25-40 GB)
#
# Output land in <repoRoot> next to finnestdb.db.

.PHONY: bootstrap-tarball bootstrap-tarball-code bootstrap-tarball-fi bootstrap-tarball-et

REPO_ROOT := $(realpath ..)
BOOTSTRAP_OUT := $(REPO_ROOT)/localdata/bootstraps

bootstrap-tarball: bootstrap-tarball-code bootstrap-tarball-fi bootstrap-tarball-et

bootstrap-tarball-code:
	mkdir -p $(BOOTSTRAP_OUT)
	cd $(REPO_ROOT) && \
	  EXISTING="finnestdb.db"; \
	  for d in localdata/lemmatizer-fi-et localdata/ekilex localdata/kaikki localdata/parser-eval corpus_pipeline; do \
	    if [ -e "$$d" ]; then EXISTING="$$EXISTING $$d"; fi; \
	  done; \
	  echo "Including: $$EXISTING"; \
	  tar czf $(BOOTSTRAP_OUT)/finnestdb-bootstrap-code.tgz $$EXISTING
	@echo "→ $(BOOTSTRAP_OUT)/finnestdb-bootstrap-code.tgz"

bootstrap-tarball-fi:
	mkdir -p $(BOOTSTRAP_OUT)
	cd $(REPO_ROOT) && tar czf $(BOOTSTRAP_OUT)/finnestdb-bootstrap-fi.tgz localdata/fi-corpus
	@echo "→ $(BOOTSTRAP_OUT)/finnestdb-bootstrap-fi.tgz"

bootstrap-tarball-et:
	mkdir -p $(BOOTSTRAP_OUT)
	cd $(REPO_ROOT) && tar czf $(BOOTSTRAP_OUT)/finnestdb-bootstrap-et.tgz localdata/et-corpus
	@echo "→ $(BOOTSTRAP_OUT)/finnestdb-bootstrap-et.tgz"

# ─── Gutenberg scraping (calls main repo's cmd/scrapegutenberg) ─────────
# We use the main repo's existing scraper (which already supports -lang fi/et)
# rather than maintaining a duplicate. Outputs to fi-corpus/gutenberg/raw/ etc.

.PHONY: scrape-gutenberg scrape-gutenberg-fi scrape-gutenberg-et

GUTENBERG_TARGET_TOKENS ?= 500000

scrape-gutenberg: scrape-gutenberg-fi scrape-gutenberg-et

scrape-gutenberg-fi:
	mkdir -p $(REPO_ROOT)/localdata/fi-corpus/gutenberg/raw
	cd $(REPO_ROOT) && go run ./cmd/scrapegutenberg \
	  -lang fi \
	  -target-tokens $(GUTENBERG_TARGET_TOKENS) \
	  -out localdata/fi-corpus/gutenberg/raw \
	  -manifest localdata/fi-corpus/gutenberg/manifest.jsonl

scrape-gutenberg-et:
	mkdir -p $(REPO_ROOT)/localdata/et-corpus/gutenberg/raw
	cd $(REPO_ROOT) && go run ./cmd/scrapegutenberg \
	  -lang et \
	  -target-tokens $(GUTENBERG_TARGET_TOKENS) \
	  -out localdata/et-corpus/gutenberg/raw \
	  -manifest localdata/et-corpus/gutenberg/manifest.jsonl

# ─── Riigikogu (ET parliament) scraper ─────────────────────────────────

.PHONY: scrape-riigikogu
RIIGIKOGU_LIMIT ?= 100

scrape-riigikogu:
	go run ./cmd/scraperiigikogu -limit $(RIIGIKOGU_LIMIT)
	go run ./cmd/extractcorpus -lang et -source riigikogu

# ─── EPUB workflow ─────────────────────────────────────────────────────

.PHONY: epub-to-text epub-to-text-fi epub-to-text-et \
        add-epub add-epub-fi add-epub-et \
        epub-deck epub-deck-fi epub-deck-et

# epub-to-text: just extract EPUBs to per-book/<slug>.txt + text.txt
epub-to-text: epub-to-text-fi epub-to-text-et

epub-to-text-fi:
	go run ./cmd/extractcorpus -lang fi -source epub

epub-to-text-et:
	go run ./cmd/extractcorpus -lang et -source epub

# add-epub: extract + fold EPUBs into the giant aggregated wordlist
add-epub: add-epub-fi add-epub-et

add-epub-fi: epub-to-text-fi
	go run ./cmd/aggregatecorpus -lang fi
	go run ./cmd/corpusverify -lang fi -profile pilot

add-epub-et: epub-to-text-et
	go run ./cmd/aggregatecorpus -lang et
	go run ./cmd/corpusverify -lang et -profile pilot

# epub-deck: per-book wordlist (standalone, doesn't pollute main wordlist)
epub-deck: epub-deck-fi epub-deck-et

epub-deck-fi:
	go run ./cmd/epubdeck -lang fi $(if $(EPUB),-epub "$(EPUB)",)

epub-deck-et:
	go run ./cmd/epubdeck -lang et $(if $(EPUB),-epub "$(EPUB)",)

# ─── enrichcorpus (omorfi/estnltk batch — runs on demand, slow) ─────────

.PHONY: enrich-corpus enrich-corpus-fi enrich-corpus-et

enrich-corpus: enrich-corpus-fi enrich-corpus-et

enrich-corpus-fi:
	go run ./cmd/enrichcorpus -lang fi

enrich-corpus-et:
	go run ./cmd/enrichcorpus -lang et

# ─── glosscoverage (lexical-source audit) ─────────────────────────────
#
# Joins the corpus wordlist against the dictionary DB and emits a
# pair/token coverage breakdown. Used before/after meaning-source imports
# to track gloss coverage. Output is a JSON file under reports/ stamped
# with the date.

.PHONY: gloss-coverage gloss-coverage-fi gloss-coverage-et

REPORT_DATE ?= $(shell date +%Y-%m-%d)

gloss-coverage: gloss-coverage-fi gloss-coverage-et

gloss-coverage-fi:
	mkdir -p reports
	go run ./cmd/glosscoverage -lang fi -out reports/$(REPORT_DATE)-coverage-fi.json

gloss-coverage-et:
	mkdir -p reports
	go run ./cmd/glosscoverage -lang et -out reports/$(REPORT_DATE)-coverage-et.json
