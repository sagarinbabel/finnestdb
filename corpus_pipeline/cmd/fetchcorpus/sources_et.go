package main

// ET source registry - v1 minimal: 4 verified OPUS sources for pilot.
var sourcesET = []Source{
	{
		Slug:     "opus-tatoeba",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-Tatoeba/v2023-04-12/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "CC BY 2.0 FR (Tatoeba)",
		Notes:    "Conversational sentences. ~8 MB.",
	},
	{
		Slug:     "opus-bible",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-bible-uedin/v1/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "Public Domain / various (bible-uedin)",
		Notes:    "Religious text. ~12 MB.",
	},
	{
		Slug:     "opus-emea",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-EMEA/v3/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "Public (EMEA)",
		Notes:    "European Medicines Agency leaflets. ~18 MB.",
	},
	{
		Slug:     "opus-jrc-acquis",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-JRC-Acquis/v3.0/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "Public (JRC-Acquis EU legal)",
		Notes:    "EU legal corpus. ~70 MB.",
	},
	{
		Slug:     "opus-europarl",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-Europarl/v8/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "Public (Europarl)",
		Notes:    "European Parliament proceedings. ~80 MB.",
	},
	{
		Slug:     "opus-eubookshop",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-EUbookshop/v2/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "Public (EU Bookshop)",
		Notes:    "EU publications.",
	},
	{
		Slug:     "opus-wikimatrix",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-WikiMatrix/v1/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "CC BY-SA (Wikipedia derivative)",
		Notes:    "Wikipedia parallel sentences.",
	},
	{
		Slug:     "opus-opensubtitles",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-OpenSubtitles/v2024/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "OpenSubtitles ToS (local-only)",
		Notes:    "Movie/TV subtitles. ~391 MB.",
	},
	{
		Slug:     "opus-ted2020",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-TED2020/v1/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "CC BY-NC-ND (TED talks)",
		Notes:    "TED talks transcripts.",
	},
	{
		Slug:     "opus-paracrawl",
		Lang:     "et",
		Kind:     "prose",
		Format:   "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-ParaCrawl/v9/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "CC0 (ParaCrawl)",
		Notes:    "Web crawl. ~360 MB.",
	},
	{
		Slug:     "hf-err-newsroom",
		Lang:     "et",
		Kind:     "prose",
		Format:   "huggingface",
		URL:      "https://huggingface.co/datasets/TalTechNLP/err-newsroom/resolve/main/train.json.gz",
		Filename: "train.json.gz",
		License:  "ERR (Estonian Public Broadcasting); local-only research use",
		Notes:    "ERR news articles 2016-2022, 187K rows.\ntext_fields=heading,lead-in,text",
	},
	// === Heavy OPUS (~9 GB combined) ===
	{
		Slug: "opus-dochplt", Lang: "et", Kind: "prose", Format: "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-DocHPLT/v3/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "CC0 (HPLT)",
		Notes:    "DocHPLT v3 - biggest single ET corpus, ~5.07 GB compressed",
	},
	{
		Slug: "opus-ccmatrix", Lang: "et", Kind: "prose", Format: "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-CCMatrix/v1/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "Open (CCMatrix)",
		Notes:    "CCMatrix mined parallel sentences, ~887 MB",
	},
	{
		Slug: "opus-nllb", Lang: "et", Kind: "prose", Format: "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-NLLB/v1/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "CC BY-NC (NLLB)",
		Notes:    "Meta NLLB mined sentences, ~882 MB",
	},
	{
		Slug: "opus-hplt", Lang: "et", Kind: "prose", Format: "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-HPLT/v3/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "CC0 (HPLT)",
		Notes:    "HPLT v3, ~800 MB",
	},
	{
		Slug: "opus-multihplt", Lang: "et", Kind: "prose", Format: "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-MultiHPLT/v3/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "CC0 (HPLT)",
		Notes:    "MultiHPLT v3, ~683 MB",
	},
	{
		Slug: "opus-multiparacrawl", Lang: "et", Kind: "prose", Format: "gz",
		URL:      "https://object.pouta.csc.fi/OPUS-MultiParaCrawl/v9b/mono/et.txt.gz",
		Filename: "et.txt.gz",
		License:  "CC0 (ParaCrawl)",
		Notes:    "MultiParaCrawl v9b, ~295 MB",
	},
	// === Wikipedia ET XML dump ===
	{
		Slug: "wikipedia-et", Lang: "et", Kind: "prose", Format: "wikipedia_dump",
		URL:      "https://dumps.wikimedia.org/etwiki/latest/etwiki-latest-pages-articles.xml.bz2",
		Filename: "etwiki-latest-pages-articles.xml.bz2",
		License:  "CC BY-SA 4.0 (Wikipedia)",
		Notes:    "Estonian Wikipedia full dump, ~300 MB bz2",
	},
	// === Leipzig Corpora ET ===
	{
		Slug: "leipzig-et-news-2020", Lang: "et", Kind: "prose", Format: "leipzig",
		URL:      "https://downloads.wortschatz-leipzig.de/corpora/est_news_2020_300K.tar.gz",
		Filename: "est_news_2020_300K.tar.gz",
		License:  "CC BY (Leipzig Corpora)",
		Notes:    "Leipzig ET news 2020, 300K sentences, ~60 MB",
	},
	{
		Slug: "leipzig-et-newscrawl-2017", Lang: "et", Kind: "prose", Format: "leipzig",
		URL:      "https://downloads.wortschatz-leipzig.de/corpora/est_newscrawl_2017_1M.tar.gz",
		Filename: "est_newscrawl_2017_1M.tar.gz",
		License:  "CC BY (Leipzig Corpora)",
		Notes:    "Leipzig ET newscrawl 2017, 1M sentences, ~204 MB",
	},
	{
		Slug: "leipzig-et-wikipedia-2021", Lang: "et", Kind: "prose", Format: "leipzig",
		URL:      "https://downloads.wortschatz-leipzig.de/corpora/est_wikipedia_2021_300K.tar.gz",
		Filename: "est_wikipedia_2021_300K.tar.gz",
		License:  "CC BY-SA (Wikipedia derivative)",
		Notes:    "Leipzig ET Wikipedia 2021, 300K sentences, ~60 MB",
	},
	// === FrequencyWords ===
	{
		Slug: "frequency-words-et", Lang: "et", Kind: "prose", Format: "text",
		URL:      "https://raw.githubusercontent.com/hermitdave/FrequencyWords/master/content/2018/et/et_50k.txt",
		Filename: "et_50k.txt",
		License:  "CC BY-SA 4.0 (Hermit Dave OpenSubtitles 2018)",
		Notes:    "Top-50k ET surface forms",
	},
}

func registryFor(lang string) []Source {
	switch lang {
	case "fi":
		return sourcesFI
	case "et":
		return sourcesET
	default:
		return nil
	}
}
