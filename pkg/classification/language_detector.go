package classification

import (
	"context"
	"regexp"
	"strings"
	"unicode"
)

// BasicLanguageDetector provides basic language detection using linguistic patterns
type BasicLanguageDetector struct {
	name               string
	version            string
	supportedLanguages []Language
	patterns           map[Language]*LanguagePatterns
}

// LanguagePatterns contains patterns for detecting a specific language
type LanguagePatterns struct {
	CommonWords     []string           // Most common words in the language
	StopWords       []string           // Stop words
	CharFrequency   map[rune]float64   // Character frequency patterns
	BigramPatterns  []string           // Common bigram patterns
	TrigramPatterns []string           // Common trigram patterns
	Regex           []*regexp.Regexp   // Language-specific regex patterns
}

// NewBasicLanguageDetector creates a new basic language detector
func NewBasicLanguageDetector() *BasicLanguageDetector {
	detector := &BasicLanguageDetector{
		name:               "basic-language-detector",
		version:            "1.0.0",
		supportedLanguages: []Language{
			LanguageEnglish, LanguageSpanish, LanguageFrench, LanguageGerman,
			LanguageItalian, LanguagePortuguese, LanguageDutch, LanguageRussian,
		},
		patterns: make(map[Language]*LanguagePatterns),
	}
	
	detector.initializePatterns()
	return detector
}

// DetectLanguage detects the language of the given text
func (d *BasicLanguageDetector) DetectLanguage(ctx context.Context, text string) (*LanguageResult, error) {
	if len(text) == 0 {
		return &LanguageResult{
			Language:   LanguageUnknown,
			Confidence: 0.0,
		}, nil
	}
	
	// Preprocess text
	cleanText := d.preprocessText(text)
	if len(cleanText) < 10 {
		return &LanguageResult{
			Language:   LanguageUnknown,
			Confidence: 0.0,
		}, nil
	}
	
	// Calculate scores for each language
	scores := make(map[Language]float64)
	candidates := make([]LanguageCandidate, 0)
	
	for _, lang := range d.supportedLanguages {
		score := d.calculateLanguageScore(cleanText, lang)
		scores[lang] = score
		
		if score > 0.1 { // Only include candidates with reasonable scores
			candidates = append(candidates, LanguageCandidate{
				Language:   lang,
				Confidence: score,
			})
		}
	}
	
	// Find the best match
	bestLang := LanguageUnknown
	bestScore := 0.0
	
	for lang, score := range scores {
		if score > bestScore {
			bestScore = score
			bestLang = lang
		}
	}
	
	// If confidence is too low, return unknown
	if bestScore < 0.3 {
		bestLang = LanguageUnknown
		bestScore = 0.0
	}
	
	// Sort candidates by confidence
	for i := range candidates {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[i].Confidence < candidates[j].Confidence {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	
	return &LanguageResult{
		Language:   bestLang,
		Confidence: bestScore,
		Candidates: candidates,
	}, nil
}

// GetSupportedLanguages returns the list of supported languages
func (d *BasicLanguageDetector) GetSupportedLanguages() []Language {
	return d.supportedLanguages
}

// preprocessText cleans and normalizes text for language detection
func (d *BasicLanguageDetector) preprocessText(text string) string {
	// Convert to lowercase
	text = strings.ToLower(text)
	
	// Remove non-letter characters except spaces
	var result strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsSpace(r) {
			result.WriteRune(r)
		} else {
			result.WriteRune(' ')
		}
	}
	
	// Normalize whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(result.String(), " ")
	text = strings.TrimSpace(text)
	
	return text
}

// calculateLanguageScore calculates a confidence score for a specific language
func (d *BasicLanguageDetector) calculateLanguageScore(text string, lang Language) float64 {
	patterns, exists := d.patterns[lang]
	if !exists {
		return 0.0
	}
	
	score := 0.0
	weights := map[string]float64{
		"common_words":    0.4,
		"stop_words":      0.3,
		"char_frequency":  0.2,
		"bigrams":         0.1,
	}
	
	// Check common words
	commonWordScore := d.checkCommonWords(text, patterns.CommonWords)
	score += commonWordScore * weights["common_words"]
	
	// Check stop words
	stopWordScore := d.checkStopWords(text, patterns.StopWords)
	score += stopWordScore * weights["stop_words"]
	
	// Check character frequency patterns
	charFreqScore := d.checkCharacterFrequency(text, patterns.CharFrequency)
	score += charFreqScore * weights["char_frequency"]
	
	// Check bigram patterns
	bigramScore := d.checkBigramPatterns(text, patterns.BigramPatterns)
	score += bigramScore * weights["bigrams"]
	
	return score
}

// checkCommonWords checks for common words in the language
func (d *BasicLanguageDetector) checkCommonWords(text string, commonWords []string) float64 {
	if len(commonWords) == 0 {
		return 0.0
	}
	
	words := strings.Fields(text)
	if len(words) == 0 {
		return 0.0
	}
	
	matches := 0
	for _, word := range words {
		for _, commonWord := range commonWords {
			if word == commonWord {
				matches++
				break
			}
		}
	}
	
	return float64(matches) / float64(len(words))
}

// checkStopWords checks for stop words in the language
func (d *BasicLanguageDetector) checkStopWords(text string, stopWords []string) float64 {
	if len(stopWords) == 0 {
		return 0.0
	}
	
	words := strings.Fields(text)
	if len(words) == 0 {
		return 0.0
	}
	
	matches := 0
	for _, word := range words {
		for _, stopWord := range stopWords {
			if word == stopWord {
				matches++
				break
			}
		}
	}
	
	return float64(matches) / float64(len(words))
}

// checkCharacterFrequency checks character frequency patterns
func (d *BasicLanguageDetector) checkCharacterFrequency(text string, expectedFreq map[rune]float64) float64 {
	if len(expectedFreq) == 0 {
		return 0.0
	}
	
	// Calculate actual character frequencies
	charCount := make(map[rune]int)
	totalChars := 0
	
	for _, r := range text {
		if unicode.IsLetter(r) {
			charCount[r]++
			totalChars++
		}
	}
	
	if totalChars == 0 {
		return 0.0
	}
	
	// Calculate similarity to expected frequencies
	similarity := 0.0
	checkedChars := 0
	
	for char, expectedFreq := range expectedFreq {
		actualCount := charCount[char]
		actualFreq := float64(actualCount) / float64(totalChars)
		
		// Calculate similarity (1 - absolute difference)
		diff := actualFreq - expectedFreq
		if diff < 0 {
			diff = -diff
		}
		
		charSimilarity := 1.0 - diff
		if charSimilarity < 0 {
			charSimilarity = 0
		}
		
		similarity += charSimilarity
		checkedChars++
	}
	
	if checkedChars == 0 {
		return 0.0
	}
	
	return similarity / float64(checkedChars)
}

// checkBigramPatterns checks for common bigram patterns
func (d *BasicLanguageDetector) checkBigramPatterns(text string, patterns []string) float64 {
	if len(patterns) == 0 {
		return 0.0
	}
	
	// Generate bigrams from text
	words := strings.Fields(text)
	if len(words) < 2 {
		return 0.0
	}
	
	textBigrams := make(map[string]int)
	totalBigrams := 0
	
	for i := 0; i < len(words)-1; i++ {
		bigram := words[i] + " " + words[i+1]
		textBigrams[bigram]++
		totalBigrams++
	}
	
	if totalBigrams == 0 {
		return 0.0
	}
	
	// Count pattern matches
	matches := 0
	for _, pattern := range patterns {
		if count, exists := textBigrams[pattern]; exists {
			matches += count
		}
	}
	
	return float64(matches) / float64(totalBigrams)
}

// initializePatterns initializes language detection patterns
func (d *BasicLanguageDetector) initializePatterns() {
	// English patterns
	d.patterns[LanguageEnglish] = &LanguagePatterns{
		CommonWords: []string{
			"the", "and", "to", "of", "a", "in", "is", "it", "you", "that",
			"he", "was", "for", "on", "are", "as", "with", "his", "they", "at",
		},
		StopWords: []string{
			"the", "and", "to", "of", "a", "in", "is", "it", "you", "that",
			"he", "was", "for", "on", "are", "as", "with", "his", "they", "at",
			"be", "this", "have", "from", "or", "one", "had", "by", "words",
		},
		CharFrequency: map[rune]float64{
			'e': 0.127, 't': 0.091, 'a': 0.082, 'o': 0.075, 'i': 0.070,
			'n': 0.067, 's': 0.063, 'h': 0.061, 'r': 0.060, 'd': 0.043,
		},
		BigramPatterns: []string{
			"of the", "in the", "to the", "on the", "for the",
			"and the", "to be", "in a", "that the", "with the",
		},
	}
	
	// Spanish patterns
	d.patterns[LanguageSpanish] = &LanguagePatterns{
		CommonWords: []string{
			"el", "la", "de", "que", "y", "a", "en", "un", "es", "se",
			"no", "te", "lo", "le", "da", "su", "por", "son", "con", "para",
		},
		StopWords: []string{
			"el", "la", "de", "que", "y", "a", "en", "un", "es", "se",
			"no", "te", "lo", "le", "da", "su", "por", "son", "con", "para",
			"una", "del", "las", "los", "al", "como", "pero", "sus", "le",
		},
		CharFrequency: map[rune]float64{
			'e': 0.138, 'a': 0.125, 'o': 0.086, 's': 0.080, 'r': 0.069,
			'n': 0.067, 'i': 0.063, 'd': 0.058, 'l': 0.050, 'c': 0.047,
		},
		BigramPatterns: []string{
			"de la", "en el", "de los", "en la", "que el",
			"por la", "con la", "de un", "en un", "que la",
		},
	}
	
	// French patterns
	d.patterns[LanguageFrench] = &LanguagePatterns{
		CommonWords: []string{
			"le", "de", "et", "à", "un", "il", "être", "et", "en", "avoir",
			"que", "pour", "dans", "ce", "son", "une", "sur", "avec", "ne", "se",
		},
		StopWords: []string{
			"le", "de", "et", "à", "un", "il", "être", "et", "en", "avoir",
			"que", "pour", "dans", "ce", "son", "une", "sur", "avec", "ne", "se",
			"pas", "tout", "plus", "par", "grand", "ou", "mais", "du", "au",
		},
		CharFrequency: map[rune]float64{
			'e': 0.121, 's': 0.081, 'a': 0.076, 'r': 0.066, 'n': 0.061,
			't': 0.059, 'i': 0.059, 'u': 0.056, 'l': 0.054, 'o': 0.054,
		},
		BigramPatterns: []string{
			"de la", "de le", "à la", "dans le", "pour la",
			"sur le", "avec le", "que le", "par la", "de ce",
		},
	}
	
	// German patterns
	d.patterns[LanguageGerman] = &LanguagePatterns{
		CommonWords: []string{
			"der", "die", "und", "in", "den", "von", "zu", "das", "mit", "sich",
			"des", "auf", "für", "ist", "im", "dem", "nicht", "ein", "eine", "als",
		},
		StopWords: []string{
			"der", "die", "und", "in", "den", "von", "zu", "das", "mit", "sich",
			"des", "auf", "für", "ist", "im", "dem", "nicht", "ein", "eine", "als",
			"auch", "es", "an", "werden", "aus", "er", "hat", "dass", "sie",
		},
		CharFrequency: map[rune]float64{
			'e': 0.174, 'n': 0.098, 'i': 0.075, 's': 0.073, 'r': 0.070,
			'a': 0.065, 't': 0.061, 'd': 0.051, 'h': 0.048, 'u': 0.043,
		},
		BigramPatterns: []string{
			"der die", "in der", "von der", "zu der", "mit der",
			"auf der", "für die", "in den", "von den", "zu den",
		},
	}
	
	// Add patterns for other languages...
	// Italian
	d.patterns[LanguageItalian] = &LanguagePatterns{
		CommonWords: []string{
			"il", "di", "che", "e", "la", "per", "un", "in", "con", "del",
			"da", "a", "al", "le", "si", "dei", "come", "io", "lo", "tutto",
		},
		StopWords: []string{
			"il", "di", "che", "e", "la", "per", "un", "in", "con", "del",
			"da", "a", "al", "le", "si", "dei", "come", "io", "lo", "tutto",
			"ma", "se", "era", "lei", "mio", "quello", "o", "ci", "anche",
		},
		CharFrequency: map[rune]float64{
			'e': 0.119, 'a': 0.117, 'i': 0.113, 'o': 0.098, 'n': 0.069,
			't': 0.063, 'r': 0.064, 'l': 0.065, 's': 0.050, 'c': 0.045,
		},
	}
	
	// Portuguese
	d.patterns[LanguagePortuguese] = &LanguagePatterns{
		CommonWords: []string{
			"de", "a", "o", "e", "da", "em", "um", "para", "é", "com",
			"não", "uma", "os", "no", "se", "na", "por", "mais", "as", "dos",
		},
		StopWords: []string{
			"de", "a", "o", "e", "da", "em", "um", "para", "é", "com",
			"não", "uma", "os", "no", "se", "na", "por", "mais", "as", "dos",
			"como", "mas", "foi", "ao", "ele", "das", "tem", "à", "seu",
		},
		CharFrequency: map[rune]float64{
			'a': 0.146, 'e': 0.126, 'o': 0.103, 's': 0.078, 'r': 0.065,
			'i': 0.062, 'n': 0.050, 'd': 0.050, 'm': 0.047, 'u': 0.046,
		},
	}
}