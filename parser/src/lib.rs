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

/// Split text into sentences based on punctuation
fn split_sentences(text: &str) -> Vec<String> {
    let mut sentences = Vec::new();
    let mut current = String::new();
    
    for ch in text.chars() {
        current.push(ch);
        // Sentence-ending punctuation: . ! ? followed by space or end
        if matches!(ch, '.' | '!' | '?') {
            // Check if next char is space or end of string
            let next_pos = current.len();
            if next_pos >= text.len() || text.chars().nth(next_pos).map_or(true, |c| c.is_whitespace()) {
                let trimmed = current.trim().to_string();
                if !trimmed.is_empty() {
                    sentences.push(trimmed);
                }
                current.clear();
            }
        }
    }
    
    // Add remaining text as a sentence if not empty
    let trimmed = current.trim().to_string();
    if !trimmed.is_empty() {
        sentences.push(trimmed);
    }
    
    if sentences.is_empty() && !text.trim().is_empty() {
        sentences.push(text.trim().to_string());
    }
    
    sentences
}

/// Tokenize a sentence into words
fn tokenize(sentence: &str) -> Vec<String> {
    sentence
        .split_whitespace()
        .map(|s| {
            // Remove trailing punctuation but keep it as part of token for now
            s.trim_matches(|c: char| c.is_whitespace())
                .to_string()
        })
        .filter(|s| !s.is_empty())
        .collect()
}

/// Create a basic token structure (stub - no real morphological analysis)
fn create_token(form: &str) -> Token {
    // For stub, use form as lemma and guess POS based on simple heuristics
    let lemma = form.to_lowercase();
    let pos = guess_pos(&form);
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
    if lower.ends_with("aa") || lower.ends_with("ää") || 
       lower.ends_with("oi") || lower.ends_with("ui") ||
       lower.ends_with("in") || lower.ends_with("en") {
        return "VERB".to_string();
    }
    
    // Check for common noun endings
    if lower.ends_with("nen") || lower.ends_with("ssa") || 
       lower.ends_with("ssä") || lower.ends_with("lla") ||
       lower.ends_with("llä") || lower.ends_with("iin") {
        return "NOUN".to_string();
    }
    
    // Check for adjective endings
    if lower.ends_with("inen") || lower.ends_with("inen") {
        return "ADJ".to_string();
    }
    
    // Default to NOUN
    "NOUN".to_string()
}

/// Main analysis function
fn analyze_text_internal(lang: &str, text: &str) -> Result<AnalysisResult, String> {
    // Normalize text
    let normalized = normalize_text(text);
    
    // Split into sentences
    let sentence_strings = split_sentences(&normalized);
    
    // Tokenize each sentence
    let sentences: Vec<Sentence> = sentence_strings
        .iter()
        .map(|sent_str| {
            let tokens: Vec<Token> = tokenize(sent_str)
                .iter()
                .map(|form| create_token(form))
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
        Ok(result) => {
            match serde_json::to_string(&result) {
                Ok(json) => CString::new(json).unwrap().into_raw(),
                Err(_) => CString::new("").unwrap().into_raw(),
            }
        }
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

    #[test]
    fn test_normalize() {
        let result = normalize_text("test");
        assert_eq!(result, "test");
    }

    #[test]
    fn test_split_sentences() {
        let text = "Hei. Miten menee? Hyvää!";
        let sentences = split_sentences(text);
        assert_eq!(sentences.len(), 3);
    }

    #[test]
    fn test_tokenize() {
        let sentence = "Hei miten menee";
        let tokens = tokenize(sentence);
        assert_eq!(tokens.len(), 3);
    }
}

