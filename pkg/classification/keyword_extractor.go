package classification

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// BasicKeywordExtractor provides basic keyword extraction using TF-IDF
type BasicKeywordExtractor struct {
	name           string
	version        string
	algorithm      string
	stopWords      map[string]bool
	minWordLength  int
	maxWordLength  int
	minFrequency   int
}

// NewBasicKeywordExtractor creates a new basic keyword extractor
func NewBasicKeywordExtractor() *BasicKeywordExtractor {
	extractor := &BasicKeywordExtractor{
		name:          "basic-keyword-extractor",
		version:       "1.0.0",
		algorithm:     "tf-idf",
		stopWords:     make(map[string]bool),
		minWordLength: 3,
		maxWordLength: 50,
		minFrequency:  1,
	}
	
	extractor.initializeStopWords()
	return extractor
}

// ExtractKeywords extracts keywords from the given text
func (e *BasicKeywordExtractor) ExtractKeywords(ctx context.Context, text string, maxKeywords int) ([]Keyword, error) {
	if len(text) == 0 {
		return []Keyword{}, nil
	}
	
	// Preprocess text
	cleanText := e.preprocessText(text)
	
	// Extract candidate words
	words := e.extractCandidateWords(cleanText)
	
	// Calculate word frequencies
	wordFreq := e.calculateWordFrequency(words)
	
	// Calculate TF-IDF scores
	keywords := e.calculateTFIDF(text, wordFreq, words)
	
	// Filter and sort keywords
	keywords = e.filterKeywords(keywords)
	
	// Sort by relevance (TF-IDF score)
	sort.Slice(keywords, func(i, j int) bool {
		return keywords[i].TfIdf > keywords[j].TfIdf
	})
	
	// Limit to maxKeywords
	if maxKeywords > 0 && len(keywords) > maxKeywords {
		keywords = keywords[:maxKeywords]
	}
	
	// Update relevance scores to be between 0 and 1
	e.normalizeRelevanceScores(keywords)
	
	return keywords, nil
}

// GetAlgorithm returns the extraction algorithm name
func (e *BasicKeywordExtractor) GetAlgorithm() string {
	return e.algorithm
}

// preprocessText cleans and normalizes text
func (e *BasicKeywordExtractor) preprocessText(text string) string {
	// Convert to lowercase
	text = strings.ToLower(text)
	
	// Remove punctuation and special characters, keep only letters and spaces
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

// extractCandidateWords extracts candidate words from text
func (e *BasicKeywordExtractor) extractCandidateWords(text string) []string {
	words := strings.Fields(text)
	candidates := []string{}
	
	for _, word := range words {
		// Check word length
		if len(word) < e.minWordLength || len(word) > e.maxWordLength {
			continue
		}
		
		// Check if it's a stop word
		if e.stopWords[word] {
			continue
		}
		
		// Check if it's all letters
		isValid := true
		for _, r := range word {
			if !unicode.IsLetter(r) {
				isValid = false
				break
			}
		}
		
		if isValid {
			candidates = append(candidates, word)
		}
	}
	
	return candidates
}

// calculateWordFrequency calculates word frequencies
func (e *BasicKeywordExtractor) calculateWordFrequency(words []string) map[string]int {
	freq := make(map[string]int)
	
	for _, word := range words {
		freq[word]++
	}
	
	return freq
}

// calculateTFIDF calculates TF-IDF scores for words
func (e *BasicKeywordExtractor) calculateTFIDF(originalText string, wordFreq map[string]int, words []string) []Keyword {
	keywords := []Keyword{}
	totalWords := len(words)
	
	// Calculate TF-IDF for each unique word
	for word, freq := range wordFreq {
		if freq < e.minFrequency {
			continue
		}
		
		// Calculate TF (Term Frequency)
		tf := float64(freq) / float64(totalWords)
		
		// Calculate IDF (Inverse Document Frequency)
		// For single document, we use a simple heuristic based on word commonality
		idf := e.calculateIDF(word, originalText)
		
		// Calculate TF-IDF
		tfIdf := tf * idf
		
		// Find positions of the word in the original text
		positions := e.findWordPositions(strings.ToLower(originalText), word)
		
		keyword := Keyword{
			Text:      word,
			Relevance: tfIdf,
			Frequency: freq,
			TfIdf:     tfIdf,
			Position:  positions,
		}
		
		keywords = append(keywords, keyword)
	}
	
	return keywords
}

// calculateIDF calculates IDF score for a word
func (e *BasicKeywordExtractor) calculateIDF(word string, text string) float64 {
	// Simple IDF calculation for single document
	// We estimate rarity based on word characteristics
	
	baseIDF := 1.0
	
	// Longer words are generally more specific
	if len(word) > 8 {
		baseIDF += 0.5
	} else if len(word) > 6 {
		baseIDF += 0.3
	} else if len(word) > 4 {
		baseIDF += 0.1
	}
	
	// Words that appear in titles or headings are more important
	if e.appearsInTitle(word, text) {
		baseIDF += 0.3
	}
	
	// Words that appear at the beginning or end of sentences are more important
	if e.appearsAtSentenceBoundary(word, text) {
		baseIDF += 0.2
	}
	
	// Common English words get lower IDF
	if e.isCommonWord(word) {
		baseIDF -= 0.3
	}
	
	// Ensure IDF is positive
	if baseIDF < 0.1 {
		baseIDF = 0.1
	}
	
	return math.Log(baseIDF + 1)
}

// appearsInTitle checks if a word appears in what looks like a title
func (e *BasicKeywordExtractor) appearsInTitle(word string, text string) bool {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Check if line looks like a title (short, potentially all caps, etc.)
		if len(line) < 100 && len(line) > 0 {
			words := strings.Fields(strings.ToLower(line))
			for _, w := range words {
				if w == word {
					return true
				}
			}
		}
	}
	return false
}

// appearsAtSentenceBoundary checks if a word appears at sentence boundaries
func (e *BasicKeywordExtractor) appearsAtSentenceBoundary(word string, text string) bool {
	sentences := regexp.MustCompile(`[.!?]+`).Split(text, -1)
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(strings.ToLower(sentence))
		words := strings.Fields(sentence)
		if len(words) > 0 {
			// Check first and last words of sentence
			if words[0] == word || words[len(words)-1] == word {
				return true
			}
		}
	}
	return false
}

// isCommonWord checks if a word is commonly used in English
func (e *BasicKeywordExtractor) isCommonWord(word string) bool {
	commonWords := map[string]bool{
		"said": true, "can": true, "get": true, "would": true, "make": true,
		"know": true, "will": true, "people": true, "time": true, "way": true,
		"could": true, "see": true, "first": true, "been": true, "call": true,
		"who": true, "its": true, "now": true, "find": true, "long": true,
		"down": true, "day": true, "did": true, "come": true, "made": true,
		"may": true, "part": true, "over": true, "new": true, "sound": true,
		"take": true, "only": true, "little": true, "work": true,
	}
	
	return commonWords[word]
}

// findWordPositions finds all positions of a word in text
func (e *BasicKeywordExtractor) findWordPositions(text string, word string) []int {
	positions := []int{}
	wordLen := len(word)
	
	for i := 0; i <= len(text)-wordLen; i++ {
		if text[i:i+wordLen] == word {
			// Check if it's a complete word (not part of another word)
			if (i == 0 || !unicode.IsLetter(rune(text[i-1]))) &&
				(i+wordLen == len(text) || !unicode.IsLetter(rune(text[i+wordLen]))) {
				positions = append(positions, i)
			}
		}
	}
	
	return positions
}

// filterKeywords filters keywords based on various criteria
func (e *BasicKeywordExtractor) filterKeywords(keywords []Keyword) []Keyword {
	filtered := []Keyword{}
	
	for _, keyword := range keywords {
		// Filter out very low scoring keywords
		if keyword.TfIdf < 0.01 {
			continue
		}
		
		// Filter out keywords that are too short after preprocessing
		if len(keyword.Text) < e.minWordLength {
			continue
		}
		
		// Filter out keywords that look like noise
		if e.isNoiseWord(keyword.Text) {
			continue
		}
		
		filtered = append(filtered, keyword)
	}
	
	return filtered
}

// isNoiseWord checks if a word is likely to be noise
func (e *BasicKeywordExtractor) isNoiseWord(word string) bool {
	// Check for patterns that indicate noise
	
	// Words with repeating characters (like "aaa", "xxx")
	if len(word) >= 3 {
		allSame := true
		for i := 1; i < len(word); i++ {
			if word[i] != word[0] {
				allSame = false
				break
			}
		}
		if allSame {
			return true
		}
	}
	
	// Words that are mostly numbers or special characters
	letterCount := 0
	for _, r := range word {
		if unicode.IsLetter(r) {
			letterCount++
		}
	}
	
	if float64(letterCount)/float64(len(word)) < 0.5 {
		return true
	}
	
	// Very short words that slipped through
	if len(word) < 2 {
		return true
	}
	
	return false
}

// normalizeRelevanceScores normalizes relevance scores to 0-1 range
func (e *BasicKeywordExtractor) normalizeRelevanceScores(keywords []Keyword) {
	if len(keywords) == 0 {
		return
	}
	
	// Find max TF-IDF score
	maxScore := 0.0
	for _, keyword := range keywords {
		if keyword.TfIdf > maxScore {
			maxScore = keyword.TfIdf
		}
	}
	
	// Normalize relevance scores
	if maxScore > 0 {
		for i := range keywords {
			keywords[i].Relevance = keywords[i].TfIdf / maxScore
		}
	}
}

// initializeStopWords initializes the stop words list
func (e *BasicKeywordExtractor) initializeStopWords() {
	stopWordsList := []string{
		// Common English stop words
		"a", "an", "and", "are", "as", "at", "be", "by", "for", "from",
		"has", "he", "in", "is", "it", "its", "of", "on", "that", "the",
		"to", "was", "were", "will", "with", "the", "this", "but", "they",
		"have", "had", "what", "said", "each", "which", "she", "do", "how",
		"their", "if", "up", "out", "many", "then", "them", "these", "so",
		"some", "her", "would", "make", "like", "into", "him", "has", "more",
		"go", "no", "way", "could", "my", "than", "first", "been", "call",
		"who", "oil", "sit", "now", "find", "long", "down", "day", "did",
		"get", "come", "made", "may", "part", "over", "new", "sound", "take",
		"only", "little", "work", "know", "place", "year", "live", "me",
		"back", "give", "most", "very", "after", "thing", "our", "just",
		"name", "good", "sentence", "man", "think", "say", "great", "where",
		"help", "through", "much", "before", "line", "right", "too", "mean",
		"old", "any", "same", "tell", "boy", "follow", "came", "want", "show",
		"also", "around", "form", "three", "small", "set", "put", "end",
		"why", "again", "turn", "here", "off", "went", "old", "number",
		"great", "tell", "men", "say", "small", "every", "found", "still",
		"between", "mane", "should", "home", "big", "give", "air", "line",
		"set", "own", "under", "read", "last", "never", "us", "left", "end",
		"along", "while", "might", "next", "sound", "below", "saw", "something",
		"thought", "both", "few", "those", "always", "looked", "show", "large",
		"often", "together", "asked", "house", "don't", "world", "going", "want",
		"school", "important", "until", "form", "food", "keep", "children", "feet",
		"land", "side", "without", "boy", "once", "animal", "life", "enough",
		"took", "sometimes", "four", "head", "above", "kind", "began", "almost",
		"live", "page", "got", "earth", "need", "far", "hand", "high", "year",
		"mother", "light", "country", "father", "let", "night", "picture", "being",
		"study", "second", "book", "carry", "took", "science", "eat", "room",
		"friend", "began", "idea", "fish", "mountain", "north", "once", "base",
		"hear", "horse", "cut", "sure", "watch", "color", "face", "wood", "main",
		"enough", "plain", "girl", "usual", "young", "ready", "above", "ever",
		"red", "list", "though", "feel", "talk", "bird", "soon", "body", "dog",
		"family", "direct", "leave", "song", "measure", "door", "product", "black",
		"short", "numeral", "class", "wind", "question", "happen", "complete",
		"ship", "area", "half", "rock", "order", "fire", "south", "problem",
		"piece", "told", "knew", "pass", "since", "top", "whole", "king", "space",
		"heard", "best", "hour", "better", "during", "hundred", "five", "remember",
		"step", "early", "hold", "west", "ground", "interest", "reach", "fast",
		"verb", "sing", "listen", "six", "table", "travel", "less", "morning",
		"ten", "simple", "several", "vowel", "toward", "war", "lay", "against",
		"pattern", "slow", "center", "love", "person", "money", "serve", "appear",
		"road", "map", "rain", "rule", "govern", "pull", "cold", "notice", "voice",
		"unit", "power", "town", "fine", "certain", "fly", "fall", "lead", "cry",
		"dark", "machine", "note", "wait", "plan", "figure", "star", "box", "noun",
		"field", "rest", "correct", "able", "pound", "done", "beauty", "drive",
		"stood", "contain", "front", "teach", "week", "final", "gave", "green",
		"oh", "quick", "develop", "ocean", "warm", "free", "minute", "strong",
		"special", "mind", "behind", "clear", "tail", "produce", "fact", "street",
		"inch", "multiply", "nothing", "course", "stay", "wheel", "full", "force",
		"blue", "object", "decide", "surface", "deep", "moon", "island", "foot",
		"system", "busy", "test", "record", "boat", "common", "gold", "possible",
		"plane", "stead", "dry", "wonder", "laugh", "thousands", "ago", "ran",
		"check", "game", "shape", "equate", "hot", "miss", "brought", "heat",
		"snow", "tire", "bring", "yes", "distant", "fill", "east", "paint",
		"language", "among", "grand", "ball", "yet", "wave", "drop", "heart",
		"am", "present", "heavy", "dance", "engine", "position", "arm", "wide",
		"sail", "material", "size", "vary", "settle", "speak", "weight", "general",
		"ice", "matter", "circle", "pair", "include", "divide", "syllable", "felt",
		"perhaps", "pick", "sudden", "count", "square", "reason", "length", "represent",
		"art", "subject", "region", "energy", "hunt", "probable", "bed", "brother",
		"egg", "ride", "cell", "believe", "fraction", "forest", "sit", "race",
		"window", "store", "summer", "train", "sleep", "prove", "lone", "leg",
		"exercise", "wall", "catch", "mount", "wish", "sky", "board", "joy",
		"winter", "sat", "written", "wild", "instrument", "kept", "glass", "grass",
		"cow", "job", "edge", "sign", "visit", "past", "soft", "fun", "bright",
		"gas", "weather", "month", "million", "bear", "finish", "happy", "hope",
		"flower", "clothe", "strange", "gone", "jump", "baby", "eight", "village",
		"meet", "root", "buy", "raise", "solve", "metal", "whether", "push",
		"seven", "paragraph", "third", "shall", "held", "hair", "describe", "cook",
		"floor", "either", "result", "burn", "hill", "safe", "cat", "century",
		"consider", "type", "law", "bit", "coast", "copy", "phrase", "silent",
		"tall", "sand", "soil", "roll", "temperature", "finger", "industry", "value",
		"fight", "lie", "beat", "excite", "natural", "view", "sense", "ear",
		"else", "quite", "broke", "case", "middle", "kill", "son", "lake",
		"moment", "scale", "loud", "spring", "observe", "child", "straight", "consonant",
		"nation", "dictionary", "milk", "speed", "method", "organ", "pay", "age",
		"section", "dress", "cloud", "surprise", "quiet", "stone", "tiny", "climb",
		"bad", "oil", "blood", "touch", "grew", "cent", "mix", "team", "wire",
		"cost", "lost", "brown", "wear", "garden", "equal", "sent", "choose",
		"fell", "fit", "flow", "fair", "bank", "collect", "save", "control",
		"decimal", "gentle", "woman", "captain", "practice", "separate", "difficult",
		"doctor", "please", "protect", "noon", "whose", "locate", "ring", "character",
		"insect", "caught", "period", "indicate", "radio", "spoke", "atom", "human",
		"history", "effect", "electric", "expect", "crop", "modern", "element",
		"hit", "student", "corner", "party", "supply", "bone", "rail", "imagine",
		"provide", "agree", "thus", "capital", "won't", "chair", "danger", "fruit",
		"rich", "thick", "soldier", "process", "operate", "guess", "necessary",
		"sharp", "wing", "create", "neighbor", "wash", "bat", "rather", "crowd",
		"corn", "compare", "poem", "string", "bell", "depend", "meat", "rub",
		"tube", "famous", "dollar", "stream", "fear", "sight", "thin", "triangle",
		"planet", "hurry", "chief", "colony", "clock", "mine", "tie", "enter",
		"major", "fresh", "search", "send", "yellow", "gun", "allow", "print",
		"dead", "spot", "desert", "suit", "current", "lift", "rose", "continue",
		"block", "chart", "hat", "sell", "success", "company", "subtract", "event",
		"particular", "deal", "swim", "term", "opposite", "wife", "shoe", "shoulder",
		"spread", "arrange", "camp", "invent", "cotton", "born", "determine", "quart",
		"nine", "truck", "noise", "level", "chance", "gather", "shop", "stretch",
		"throw", "shine", "property", "column", "molecule", "select", "wrong", "gray",
		"repeat", "require", "broad", "prepare", "salt", "nose", "plural", "anger",
		"claim", "continent", "oxygen", "sugar", "death", "pretty", "skill", "women",
		"season", "solution", "magnet", "silver", "thank", "branch", "match", "suffix",
		"especially", "fig", "afraid", "huge", "sister", "steel", "discuss", "forward",
		"similar", "guide", "experience", "score", "apple", "bought", "led", "pitch",
		"coat", "mass", "card", "band", "rope", "slip", "win", "dream", "evening",
		"condition", "feed", "tool", "total", "basic", "smell", "valley", "nor",
		"double", "seat", "arrive", "master", "track", "parent", "shore", "division",
		"sheet", "substance", "favor", "connect", "post", "spend", "chord", "fat",
		"glad", "original", "share", "station", "dad", "bread", "charge", "proper",
		"bar", "offer", "segment", "slave", "duck", "instant", "market", "degree",
		"populate", "chick", "dear", "enemy", "reply", "drink", "occur", "support",
		"speech", "nature", "range", "steam", "motion", "path", "liquid", "log",
		"meant", "quotient", "teeth", "shell", "neck",
	}
	
	for _, word := range stopWordsList {
		e.stopWords[word] = true
	}
}