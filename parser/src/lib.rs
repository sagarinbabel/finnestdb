use serde::{Deserialize, Serialize};
use std::ffi::{CStr, CString};
use std::os::raw::c_char;
use unicode_normalization::UnicodeNormalization;

#[derive(Debug, Serialize, Deserialize)]
pub struct Token {
    pub form: String,
    pub lemma: String,
    pub pos: String,
    pub feats: serde_json::Value,
    pub grammar_label: String,
    pub mwe_id: Option<u32>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Sentence {
    pub tokens: Vec<Token>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AnalysisResult {
    pub sentences: Vec<Sentence>,
}

/// Normalize text to NFC form
fn normalize_text(text: &str) -> String {
    text.nfc().collect::<String>()
}

/// Check if a character is punctuation that should be separated from words.
///
/// Covers sentence-ending marks, paired brackets/quotes, and common typographic
/// punctuation used in Finnish and Estonian text.
fn is_punct(c: char) -> bool {
    matches!(
        c,
        '„' | '.'
            | ','
            | ';'
            | ':'
            | '!'
            | '?'
            | '('
            | ')'
            | '['
            | ']'
            | '{'
            | '}'
            | '"'
            | '\''
            | '*'
            | '«'
            | '»'
            | '—'
            | '–'
            | '…'
            | '\u{201C}'
            | '\u{201D}'
            | '\u{2018}'
            | '\u{2019}'
    )
}

/// Check if a character is opening punctuation (space suppressed after it).
fn is_opening_punct(c: char) -> bool {
    matches!(c, '(' | '[' | '{' | '«' | '„' | '\u{201C}' | '\u{2018}')
}

/// Split text into sentences using improved heuristics.
///
/// Splits on `. ! ?` only when followed by whitespace and an uppercase character
/// (or end-of-string). This avoids false splits on abbreviations (esim., ns.)
/// and decimal numbers (3.14).
fn split_sentences(text: &str) -> Vec<String> {
    let chars: Vec<char> = text.chars().collect();
    let len = chars.len();
    let mut sentences = Vec::new();
    let mut start = 0;

    let mut i = 0;
    while i < len {
        if matches!(chars[i], '.' | '!' | '?') {
            // Look ahead past any consecutive sentence-ending punctuation (e.g. "?!")
            let mut end_punct = i + 1;
            while end_punct < len && matches!(chars[end_punct], '.' | '!' | '?') {
                end_punct += 1;
            }
            // Also consume any closing punctuation after the sentence-ending marks
            // so that `"word)."` keeps the `)` and `.` together in the sentence.
            while end_punct < len
                && is_punct(chars[end_punct])
                && !matches!(chars[end_punct], '.' | '!' | '?')
            {
                // Only consume closing-style punct, not opening
                if is_opening_punct(chars[end_punct]) {
                    break;
                }
                end_punct += 1;
            }

            // Check if this looks like a real sentence boundary:
            // either end-of-string or whitespace followed by an uppercase letter.
            let is_boundary = if end_punct >= len {
                true
            } else if chars[end_punct].is_whitespace() {
                // Find next non-whitespace character
                let mut next_nonws = end_punct + 1;
                while next_nonws < len && chars[next_nonws].is_whitespace() {
                    next_nonws += 1;
                }
                // Sentence boundary if next non-ws char is uppercase or end-of-string
                next_nonws >= len || chars[next_nonws].is_uppercase()
            } else {
                false
            };

            if is_boundary {
                let sentence: String = chars[start..end_punct].iter().collect();
                let trimmed = sentence.trim().to_string();
                if !trimmed.is_empty() {
                    sentences.push(trimmed);
                }
                // Skip whitespace after the sentence boundary
                start = end_punct;
                while start < len && chars[start].is_whitespace() {
                    start += 1;
                }
                i = start;
                continue;
            } else {
                i = end_punct;
                continue;
            }
        }
        i += 1;
    }

    // Add remaining text as a sentence if not empty
    if start < len {
        let remaining: String = chars[start..].iter().collect();
        let trimmed = remaining.trim().to_string();
        if !trimmed.is_empty() {
            sentences.push(trimmed);
        }
    }

    if sentences.is_empty() && !text.trim().is_empty() {
        sentences.push(text.trim().to_string());
    }

    sentences
}

/// True iff `s` is non-empty and all characters are ASCII digits.
fn is_all_digits(s: &str) -> bool {
    !s.is_empty() && s.chars().all(|c| c.is_ascii_digit())
}

/// Try to split a chunk at a numeric hyphen boundary (rules R1 and R4).
///
/// R1 — digit/letter boundary: split at the first hyphen where one side is
/// pure digits and the other starts with a letter. Examples that split:
/// `65-aastane`, `1990-luvulla`, `aastane-65`. Examples that don't:
/// `B1-tase` (mixed prefix), `well-known` (no digits).
///
/// R4 — single-hyphen digit/digit range: if there is exactly one hyphen and
/// both sides are pure digits, split there. Examples that split: `1990-2020`,
/// `12-15`. Examples that don't: `2026-05-06` (two hyphens — date pattern
/// stays whole).
///
/// Returns `Some([left, "-", right])` on a split, `None` otherwise.
fn try_split_numeric_hyphen(word: &str) -> Option<[String; 3]> {
    let chars: Vec<char> = word.chars().collect();

    // R1: scan for the first digit/letter (or letter/digit) hyphen boundary.
    for i in 0..chars.len() {
        if chars[i] != '-' {
            continue;
        }
        let left: String = chars[..i].iter().collect();
        let right: String = chars[i + 1..].iter().collect();
        if left.is_empty() || right.is_empty() {
            continue;
        }
        let left_digits = is_all_digits(&left);
        let right_digits = is_all_digits(&right);
        let right_starts_letter = right
            .chars()
            .next()
            .map(|c| c.is_alphabetic())
            .unwrap_or(false);
        let left_ends_letter = left
            .chars()
            .last()
            .map(|c| c.is_alphabetic())
            .unwrap_or(false);
        if (left_digits && right_starts_letter) || (left_ends_letter && right_digits) {
            return Some([left, "-".to_string(), right]);
        }
    }

    // R4: exactly one hyphen and both sides pure digits.
    if word.chars().filter(|c| *c == '-').count() == 1 {
        if let Some(pos) = word.find('-') {
            let (left, rest) = word.split_at(pos);
            let right = &rest[1..];
            if is_all_digits(left) && is_all_digits(right) {
                return Some([left.to_string(), "-".to_string(), right.to_string()]);
            }
        }
    }

    None
}

/// Merge adjacent NUM-shaped tokens that look like an SI thousand-separated
/// number (rule R3).
///
/// Pattern: a non-punct token of 1–3 ASCII digits followed by one or more
/// non-punct tokens of exactly 3 ASCII digits. The whole run collapses into
/// one token whose form preserves the spaces (`"250 000"`); `create_token`
/// strips the spaces when forming the lemma so spaced and unspaced numbers
/// group together in the words list.
///
/// ```text
///   ["Maja","maksis","250","000","eurot","."]
///       → ["Maja","maksis","250 000","eurot","."]
/// ```
fn merge_thousand_separator(tokens: Vec<(String, bool)>) -> Vec<(String, bool)> {
    let mut out: Vec<(String, bool)> = Vec::with_capacity(tokens.len());
    let mut i = 0;
    while i < tokens.len() {
        let (form, is_pun) = &tokens[i];
        if !*is_pun && is_all_digits(form) {
            let first_len = form.chars().count();
            if (1..=3).contains(&first_len) {
                let mut end = i + 1;
                while end < tokens.len() {
                    let (next_form, next_is_pun) = &tokens[end];
                    if *next_is_pun {
                        break;
                    }
                    if !is_all_digits(next_form) {
                        break;
                    }
                    if next_form.chars().count() != 3 {
                        break;
                    }
                    end += 1;
                }
                if end > i + 1 {
                    let parts: Vec<&str> = tokens[i..end].iter().map(|(f, _)| f.as_str()).collect();
                    out.push((parts.join(" "), false));
                    i = end;
                    continue;
                }
            }
        }
        out.push((form.clone(), *is_pun));
        i += 1;
    }
    out
}

/// Tokenize a sentence into (form, is_punct) pairs.
///
/// For each whitespace-delimited chunk, peels leading and trailing punctuation
/// into separate tokens. Hyphens inside words are preserved ("well-known"),
/// and decimal numbers are kept intact ("3.14"). Numeric-hyphen compounds
/// (`65-aastane`, `1990-luvulla`, `1990-2020`) are split at the hyphen via
/// `try_split_numeric_hyphen`. Adjacent digit groups like `250 000` are
/// merged into a single NUM-shaped token by `merge_thousand_separator`.
///
/// ```text
///   "(kauppaan)." → [("(", true), ("kauppaan", false), (")", true), (".", true)]
///   "3.14"        → [("3.14", false)]
///   "well-known"  → [("well-known", false)]
///   "65-aastane"  → [("65", false), ("-", true), ("aastane", false)]
///   "250 000"     → [("250 000", false)]
/// ```
fn tokenize(sentence: &str) -> Vec<(String, bool)> {
    let mut tokens = Vec::new();

    for chunk in sentence.split_whitespace() {
        let chars: Vec<char> = chunk.chars().collect();
        let len = chars.len();

        // If the entire chunk is punctuation, emit each char as a PUNCT token.
        if chars.iter().all(|c| is_punct(*c)) {
            for c in &chars {
                tokens.push((c.to_string(), true));
            }
            continue;
        }

        // Peel leading punctuation.
        let mut left = 0;
        while left < len && is_punct(chars[left]) {
            tokens.push((chars[left].to_string(), true));
            left += 1;
        }

        // Peel trailing punctuation.
        let mut right = len;
        while right > left && is_punct(chars[right - 1]) {
            right -= 1;
        }

        // The word core (between leading and trailing punct).
        if left < right {
            let word: String = chars[left..right].iter().collect();

            // Check if this looks like a decimal number (digits.digits).
            // If so, re-attach the trailing '.' that we peeled.
            if right < len && chars[right] == '.' {
                let has_digits_before_dot = word.chars().all(|c| c.is_ascii_digit());
                let has_digits_after_dot = (right + 1 < len)
                    && chars[right + 1..]
                        .iter()
                        .take_while(|c| !is_punct(**c) || **c == '.')
                        .all(|c| c.is_ascii_digit());
                if has_digits_before_dot && has_digits_after_dot {
                    // Re-collect the full decimal: word + "." + trailing digits
                    let decimal: String = chars[left..len].iter().collect();
                    // Check if it's actually digits.digits (no trailing punct beyond that)
                    let parts: Vec<&str> = decimal.splitn(2, '.').collect();
                    if parts.len() == 2
                        && !parts[0].is_empty()
                        && parts[0].chars().all(|c| c.is_ascii_digit())
                        && !parts[1].is_empty()
                        && parts[1].chars().all(|c| c.is_ascii_digit())
                    {
                        tokens.push((decimal, false));
                        // No trailing punct to emit — the digits consumed everything.
                        continue;
                    }
                }
            }

            // R1/R4: split numeric-hyphen compounds before emitting.
            if let Some(parts) = try_split_numeric_hyphen(&word) {
                let [head, hyphen, tail] = parts;
                tokens.push((head, false));
                tokens.push((hyphen, true));
                tokens.push((tail, false));
            } else {
                tokens.push((word, false));
            }
        }

        // Emit trailing punctuation (each char as its own PUNCT token).
        for i in right..len {
            tokens.push((chars[i].to_string(), true));
        }
    }

    // R3: merge adjacent digit groups that look like SI thousand notation.
    merge_thousand_separator(tokens)
}

/// Create a token from a form, marking punctuation with POS "PUNCT".
fn create_token(form: &str, is_punct_token: bool) -> Token {
    if is_punct_token {
        // Determine grammar label for punctuation direction (opening vs closing).
        let first_char = form.chars().next().unwrap_or('.');
        let grammar_label = if is_opening_punct(first_char) {
            "PUNCT_OPEN".to_string()
        } else {
            "PUNCT_CLOSE".to_string()
        };
        return Token {
            form: form.to_string(),
            lemma: form.to_string(),
            pos: "PUNCT".to_string(),
            feats: serde_json::json!({}),
            grammar_label,
            mwe_id: None,
        };
    }

    // Non-punctuation: guess POS via heuristic and derive a lemma.
    //
    // For NUM tokens, the lemma drops whitespace so SI-spaced and unspaced
    // numbers (`250 000` and `250000`) group as the same entry in the words
    // list. Other POS keep the lowercased form.
    let pos = guess_pos(form);
    let lemma = if pos == "NUM" {
        form.chars().filter(|c| !c.is_whitespace()).collect()
    } else {
        form.to_lowercase()
    };
    let grammar_label = format!("{} (stub)", pos);

    Token {
        form: form.to_string(),
        lemma,
        pos,
        feats: serde_json::json!({}),
        grammar_label,
        mwe_id: None,
    }
}

/// Simple POS guessing based on form (stub implementation).
///
/// R2: forms made up entirely of ASCII digits — optionally with a single
/// `.` or `,` decimal separator, optionally with internal whitespace acting
/// as a thousand separator — are tagged `NUM` before any other heuristic
/// runs. This catches `65`, `1990`, `3.14`, `12,50`, and `250 000`.
fn guess_pos(form: &str) -> String {
    let stripped: String = form.chars().filter(|c| !c.is_whitespace()).collect();
    if !stripped.is_empty() {
        let only_digits = stripped.chars().all(|c| c.is_ascii_digit());
        let is_decimal_with = |sep: char| -> bool {
            let mut iter = stripped.split(sep);
            match (iter.next(), iter.next(), iter.next()) {
                (Some(a), Some(b), None) => is_all_digits(a) && is_all_digits(b),
                _ => false,
            }
        };
        if only_digits || is_decimal_with('.') || is_decimal_with(',') {
            return "NUM".to_string();
        }
    }

    let lower = form.to_lowercase();

    // Check for common Finnish/Estonian verb endings
    if lower.ends_with("aa")
        || lower.ends_with("ää")
        || lower.ends_with("oi")
        || lower.ends_with("ui")
        || lower.ends_with("in")
        || lower.ends_with("en")
    {
        return "VERB".to_string();
    }

    // Check for common noun endings
    if lower.ends_with("nen")
        || lower.ends_with("ssa")
        || lower.ends_with("ssä")
        || lower.ends_with("lla")
        || lower.ends_with("llä")
        || lower.ends_with("iin")
    {
        return "NOUN".to_string();
    }

    // Check for adjective endings
    if lower.ends_with("inen") {
        return "ADJ".to_string();
    }

    // Default to NOUN
    "NOUN".to_string()
}

/// Main analysis function
fn analyze_text_internal(_lang: &str, text: &str) -> Result<AnalysisResult, String> {
    let normalized = normalize_text(text);
    let sentence_strings = split_sentences(&normalized);

    let sentences: Vec<Sentence> = sentence_strings
        .iter()
        .map(|sent_str| {
            let tokens: Vec<Token> = tokenize(sent_str)
                .iter()
                .map(|(form, is_punct)| create_token(form, *is_punct))
                .collect();
            Sentence { tokens }
        })
        .collect();

    Ok(AnalysisResult { sentences })
}

/// FFI export: Analyze text and return JSON string
///
/// # Safety
/// This function is unsafe because it deals with raw C pointers.
/// Caller must ensure `lang` and `text` are valid null-terminated C strings.
#[no_mangle]
pub extern "C" fn analyze_text(lang: *const c_char, text: *const c_char) -> *mut c_char {
    let lang_str = unsafe {
        match CStr::from_ptr(lang).to_str() {
            Ok(s) => s,
            Err(_) => return CString::new("").unwrap().into_raw(),
        }
    };

    let text_str = unsafe {
        match CStr::from_ptr(text).to_str() {
            Ok(s) => s,
            Err(_) => return CString::new("").unwrap().into_raw(),
        }
    };

    match analyze_text_internal(lang_str, text_str) {
        Ok(result) => match serde_json::to_string(&result) {
            Ok(json) => CString::new(json).unwrap().into_raw(),
            Err(_) => CString::new("").unwrap().into_raw(),
        },
        Err(_) => CString::new("").unwrap().into_raw(),
    }
}

/// FFI export: Free a C string allocated by analyze_text
///
/// # Safety
/// Caller must ensure ptr is a valid pointer returned by analyze_text
#[no_mangle]
pub extern "C" fn free_string(ptr: *mut c_char) {
    if !ptr.is_null() {
        unsafe {
            let _ = CString::from_raw(ptr);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // ─── Helper: extract forms from tokenize output ─────────────────────────
    fn forms(input: &str) -> Vec<String> {
        tokenize(input).into_iter().map(|(f, _)| f).collect()
    }

    // ─── Normalization ──────────────────────────────────────────────────────
    #[test]
    fn test_normalize() {
        let result = normalize_text("test");
        assert_eq!(result, "test");
    }

    // ─── Sentence splitting ─────────────────────────────────────────────────
    #[test]
    fn test_split_sentences() {
        let text = "Hei. Miten menee? Hyvää!";
        let sentences = split_sentences(text);
        assert_eq!(sentences.len(), 3);
    }

    #[test]
    fn test_sentence_split_abbreviation() {
        // "esim." followed by lowercase should NOT split.
        let text = "Esim. tämä on lause.";
        let sentences = split_sentences(text);
        assert_eq!(
            sentences.len(),
            1,
            "abbreviation should not cause split: {:?}",
            sentences
        );
    }

    #[test]
    fn test_sentence_split_decimal() {
        let text = "Hinta on 3.14 euroa.";
        let sentences = split_sentences(text);
        assert_eq!(
            sentences.len(),
            1,
            "decimal should not cause split: {:?}",
            sentences
        );
    }

    // ─── Tokenization: punctuation separation ───────────────────────────────
    #[test]
    fn test_tokenize_basic() {
        let tokens = forms("Hei miten menee");
        assert_eq!(tokens, vec!["Hei", "miten", "menee"]);
    }

    #[test]
    fn test_tokenize_trailing_punct() {
        // "kauppaan." → ["kauppaan", "."]
        let tokens = forms("kauppaan.");
        assert_eq!(tokens, vec!["kauppaan", "."]);
    }

    #[test]
    fn test_tokenize_brackets() {
        // "(kirja)" → ["(", "kirja", ")"]
        let tokens = forms("(kirja)");
        assert_eq!(tokens, vec!["(", "kirja", ")"]);
    }

    #[test]
    fn test_tokenize_mixed_punct() {
        // "word)." → ["word", ")", "."]  — the user's explicit pain point
        let tokens = forms("word).");
        assert_eq!(tokens, vec!["word", ")", "."]);
    }

    #[test]
    fn test_tokenize_decimal_preserved() {
        // "3.14" should stay as one token, not split on the period.
        let tokens = forms("3.14");
        assert_eq!(tokens, vec!["3.14"]);
    }

    #[test]
    fn test_tokenize_hyphen_preserved() {
        // Hyphens inside words are NOT punctuation to separate.
        let tokens = forms("well-known");
        assert_eq!(tokens, vec!["well-known"]);
    }

    #[test]
    fn test_tokenize_trailing_asterisk() {
        let tokens = forms("b1-tase*");
        assert_eq!(tokens, vec!["b1-tase", "*"]);
    }

    #[test]
    fn test_tokenize_guillemets() {
        let tokens = forms("«tervetuloa»");
        assert_eq!(tokens, vec!["«", "tervetuloa", "»"]);
    }

    #[test]
    fn test_tokenize_low_double_quote() {
        let tokens = forms("„tervetuloa”");
        assert_eq!(tokens, vec!["„", "tervetuloa", "”"]);
    }

    #[test]
    fn test_tokenize_all_punct() {
        // A chunk that is entirely punctuation: each char becomes a token.
        let tokens = forms("...");
        assert_eq!(tokens, vec![".", ".", "."]);
    }

    // ─── Token creation: PUNCT vs word ──────────────────────────────────────
    #[test]
    fn test_create_token_punct() {
        let tok = create_token(".", true);
        assert_eq!(tok.pos, "PUNCT");
        assert_eq!(tok.grammar_label, "PUNCT_CLOSE");
    }

    #[test]
    fn test_create_token_open_punct() {
        let tok = create_token("(", true);
        assert_eq!(tok.pos, "PUNCT");
        assert_eq!(tok.grammar_label, "PUNCT_OPEN");
    }

    #[test]
    fn test_create_token_word() {
        let tok = create_token("kirja", false);
        assert_ne!(tok.pos, "PUNCT");
        assert_eq!(tok.lemma, "kirja");
    }

    // ─── Numeric-hyphen splitting (R1, R4) ──────────────────────────────────
    #[test]
    fn test_tokenize_digit_letter_hyphen_split_et() {
        // ET: "65-aastane" → ["65", "-", "aastane"]
        let tokens = forms("65-aastane");
        assert_eq!(tokens, vec!["65", "-", "aastane"]);
    }

    #[test]
    fn test_tokenize_digit_letter_hyphen_split_fi() {
        // FI: "65-vuotias" → ["65", "-", "vuotias"]
        let tokens = forms("65-vuotias");
        assert_eq!(tokens, vec!["65", "-", "vuotias"]);
    }

    #[test]
    fn test_tokenize_digit_letter_hyphen_split_decade() {
        // FI: "1990-luvulla" → ["1990", "-", "luvulla"]
        let tokens = forms("1990-luvulla");
        assert_eq!(tokens, vec!["1990", "-", "luvulla"]);
    }

    #[test]
    fn test_tokenize_letter_digit_hyphen_split() {
        // Reverse order also splits.
        let tokens = forms("aastane-65");
        assert_eq!(tokens, vec!["aastane", "-", "65"]);
    }

    #[test]
    fn test_tokenize_mixed_prefix_does_not_split() {
        // "B1" is alphanumeric (mixed), not pure digits → don't split.
        let tokens = forms("B1-tase");
        assert_eq!(tokens, vec!["B1-tase"]);
    }

    #[test]
    fn test_tokenize_letters_letters_does_not_split() {
        // Already covered by test_tokenize_hyphen_preserved, but reaffirm under
        // the new rule set.
        let tokens = forms("well-known");
        assert_eq!(tokens, vec!["well-known"]);
    }

    #[test]
    fn test_tokenize_digit_digit_range_splits() {
        // R4: single hyphen, both sides pure digits → split.
        let tokens = forms("1990-2020");
        assert_eq!(tokens, vec!["1990", "-", "2020"]);
    }

    #[test]
    fn test_tokenize_iso_date_does_not_split() {
        // R4 only fires on a single hyphen — ISO dates have two and stay whole.
        let tokens = forms("2026-05-06");
        assert_eq!(tokens, vec!["2026-05-06"]);
    }

    #[test]
    fn test_tokenize_split_with_trailing_punct() {
        // Trailing "." peels first, then the inner chunk splits.
        let tokens = forms("65-aastane.");
        assert_eq!(tokens, vec!["65", "-", "aastane", "."]);
    }

    #[test]
    fn test_tokenize_split_inside_brackets() {
        let tokens = forms("(65-aastane)");
        assert_eq!(tokens, vec!["(", "65", "-", "aastane", ")"]);
    }

    // ─── Thousand-separator merge (R3) ──────────────────────────────────────
    #[test]
    fn test_tokenize_thousand_space_merge_two_groups() {
        let tokens = forms("250 000 eurot");
        assert_eq!(tokens, vec!["250 000", "eurot"]);
    }

    #[test]
    fn test_tokenize_thousand_space_merge_three_groups() {
        let tokens = forms("1 234 567 syntyi");
        assert_eq!(tokens, vec!["1 234 567", "syntyi"]);
    }

    #[test]
    fn test_tokenize_thousand_merge_stops_at_non_three_digit() {
        // Second chunk is 2 digits, not 3 → no merge.
        let tokens = forms("12 34 sai");
        assert_eq!(tokens, vec!["12", "34", "sai"]);
    }

    #[test]
    fn test_tokenize_thousand_merge_skips_non_digit_first() {
        // First non-punct token isn't all-digits → nothing to merge.
        let tokens = forms("Maja 000 kallis");
        assert_eq!(tokens, vec!["Maja", "000", "kallis"]);
    }

    // ─── NUM POS (R2) ───────────────────────────────────────────────────────
    #[test]
    fn test_guess_pos_pure_integer() {
        assert_eq!(guess_pos("65"), "NUM");
        assert_eq!(guess_pos("1990"), "NUM");
    }

    #[test]
    fn test_guess_pos_decimal_period() {
        assert_eq!(guess_pos("3.14"), "NUM");
    }

    #[test]
    fn test_guess_pos_decimal_comma() {
        assert_eq!(guess_pos("12,50"), "NUM");
    }

    #[test]
    fn test_guess_pos_thousand_spaced() {
        assert_eq!(guess_pos("250 000"), "NUM");
        assert_eq!(guess_pos("1 234 567"), "NUM");
    }

    #[test]
    fn test_create_token_num_strips_space_in_lemma() {
        // Form keeps the space, lemma strips it so "250 000" and "250000"
        // group as one entry in the words list.
        let tok = create_token("250 000", false);
        assert_eq!(tok.pos, "NUM");
        assert_eq!(tok.form, "250 000");
        assert_eq!(tok.lemma, "250000");
    }

    #[test]
    fn test_create_token_num_decimal_lemma() {
        let tok = create_token("3.14", false);
        assert_eq!(tok.pos, "NUM");
        assert_eq!(tok.lemma, "3.14");
    }

    // ─── End-to-end: analyze_text_internal ──────────────────────────────────
    #[test]
    fn test_analyze_punct_separation() {
        let result = analyze_text_internal("FI", "Menin kauppaan).").unwrap();
        let tokens = &result.sentences[0].tokens;
        let token_forms: Vec<&str> = tokens.iter().map(|t| t.form.as_str()).collect();
        // "kauppaan)." should become "kauppaan", ")", "."
        assert!(
            token_forms.contains(&"kauppaan"),
            "missing 'kauppaan': {:?}",
            token_forms
        );
        assert!(token_forms.contains(&")"), "missing ')': {:?}", token_forms);
        assert!(token_forms.contains(&"."), "missing '.': {:?}", token_forms);
        // "kauppaan)." should NOT be a single token
        assert!(
            !token_forms.contains(&"kauppaan)."),
            "should not contain 'kauppaan).': {:?}",
            token_forms
        );
    }

    #[test]
    fn test_analyze_numeric_hyphen_split_et() {
        // Full pipeline: "Ta on 65-aastane." → "65" tagged NUM, "aastane"
        // emitted as a separate word token so the dictionary lookup can
        // resolve it on the Go side.
        let result = analyze_text_internal("ET", "Ta on 65-aastane.").unwrap();
        let tokens = &result.sentences[0].tokens;
        let pairs: Vec<(&str, &str)> = tokens
            .iter()
            .map(|t| (t.form.as_str(), t.pos.as_str()))
            .collect();
        assert!(
            pairs.contains(&("65", "NUM")),
            "expected ('65','NUM'): {:?}",
            pairs
        );
        assert!(
            pairs.iter().any(|(f, _)| *f == "aastane"),
            "expected token 'aastane': {:?}",
            pairs
        );
        assert!(
            !pairs.iter().any(|(f, _)| *f == "65-aastane"),
            "should not keep '65-aastane' as one token: {:?}",
            pairs
        );
    }

    #[test]
    fn test_analyze_thousand_spaced_number() {
        let result = analyze_text_internal("ET", "Maja maksis 250 000 eurot.").unwrap();
        let tokens = &result.sentences[0].tokens;
        let merged = tokens
            .iter()
            .find(|t| t.form == "250 000")
            .expect("expected merged '250 000' token");
        assert_eq!(merged.pos, "NUM");
        assert_eq!(merged.lemma, "250000");
    }

    #[test]
    fn test_analyze_year_range_split() {
        let result = analyze_text_internal("ET", "Tegevus toimus aastatel 1990-2020.").unwrap();
        let tokens = &result.sentences[0].tokens;
        let pairs: Vec<(&str, &str)> = tokens
            .iter()
            .map(|t| (t.form.as_str(), t.pos.as_str()))
            .collect();
        assert!(pairs.contains(&("1990", "NUM")), "{:?}", pairs);
        assert!(pairs.contains(&("2020", "NUM")), "{:?}", pairs);
        assert!(
            pairs.iter().any(|(f, p)| *f == "-" && *p == "PUNCT"),
            "expected hyphen as PUNCT: {:?}",
            pairs
        );
    }
}
