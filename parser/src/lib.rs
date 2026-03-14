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
    matches!(c, '„' | '.' | ',' | ';' | ':' | '!' | '?' | '(' | ')' | '[' | ']'
              | '{' | '}' | '"' | '\'' | '«' | '»' | '—' | '–' | '…'
              | '\u{201C}' | '\u{201D}' | '\u{2018}' | '\u{2019}')
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
            while end_punct < len && is_punct(chars[end_punct]) && !matches!(chars[end_punct], '.' | '!' | '?') {
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

/// Tokenize a sentence into (form, is_punct) pairs.
///
/// For each whitespace-delimited chunk, peels leading and trailing punctuation
/// into separate tokens. Hyphens inside words are preserved ("well-known"),
/// and decimal numbers are kept intact ("3.14").
///
/// ```text
///   "(kauppaan)." → [("(", true), ("kauppaan", false), (")", true), (".", true)]
///   "3.14"        → [("3.14", false)]
///   "well-known"  → [("well-known", false)]
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
                    && chars[right + 1..].iter().take_while(|c| !is_punct(**c) || **c == '.').all(|c| c.is_ascii_digit());
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

            tokens.push((word, false));
        }

        // Emit trailing punctuation (each char as its own PUNCT token).
        for i in right..len {
            tokens.push((chars[i].to_string(), true));
        }
    }

    tokens
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

    // Non-punctuation: use form as lemma and guess POS via heuristic.
    let lemma = form.to_lowercase();
    let pos = guess_pos(form);
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

/// Simple POS guessing based on form (stub implementation)
fn guess_pos(form: &str) -> String {
    let lower = form.to_lowercase();

    // Check for common Finnish/Estonian verb endings
    if lower.ends_with("aa") || lower.ends_with("ää")
        || lower.ends_with("oi") || lower.ends_with("ui")
        || lower.ends_with("in") || lower.ends_with("en")
    {
        return "VERB".to_string();
    }

    // Check for common noun endings
    if lower.ends_with("nen") || lower.ends_with("ssa")
        || lower.ends_with("ssä") || lower.ends_with("lla")
        || lower.ends_with("llä") || lower.ends_with("iin")
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
        assert_eq!(sentences.len(), 1, "abbreviation should not cause split: {:?}", sentences);
    }

    #[test]
    fn test_sentence_split_decimal() {
        let text = "Hinta on 3.14 euroa.";
        let sentences = split_sentences(text);
        assert_eq!(sentences.len(), 1, "decimal should not cause split: {:?}", sentences);
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
    fn test_tokenize_guillemets() {
        let tokens = forms("«tervetuloa»");
        assert_eq!(tokens, vec!["«", "tervetuloa", "»"]);
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

    // ─── End-to-end: analyze_text_internal ──────────────────────────────────
    #[test]
    fn test_analyze_punct_separation() {
        let result = analyze_text_internal("FI", "Menin kauppaan).").unwrap();
        let tokens = &result.sentences[0].tokens;
        let token_forms: Vec<&str> = tokens.iter().map(|t| t.form.as_str()).collect();
        // "kauppaan)." should become "kauppaan", ")", "."
        assert!(token_forms.contains(&"kauppaan"), "missing 'kauppaan': {:?}", token_forms);
        assert!(token_forms.contains(&")"), "missing ')': {:?}", token_forms);
        assert!(token_forms.contains(&"."), "missing '.': {:?}", token_forms);
        // "kauppaan)." should NOT be a single token
        assert!(!token_forms.contains(&"kauppaan)."), "should not contain 'kauppaan).': {:?}", token_forms);
    }
}

