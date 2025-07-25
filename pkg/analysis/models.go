package analysis

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Heuristic implementations of ML models for content analysis
// These provide baseline functionality that can be enhanced with actual ML models

// HeuristicSentimentModel implements basic sentiment analysis using lexicon-based approach
type HeuristicSentimentModel struct {
	positiveWords map[string]float64
	negativeWords map[string]float64
	version       string
}

// NewHeuristicSentimentModel creates a new heuristic sentiment model
func NewHeuristicSentimentModel() *HeuristicSentimentModel {
	return &HeuristicSentimentModel{
		positiveWords: getPositiveWords(),
		negativeWords: getNegativeWords(),
		version:       "heuristic-v1.0",
	}
}

// AnalyzeSentiment performs sentiment analysis using word polarity
func (m *HeuristicSentimentModel) AnalyzeSentiment(ctx context.Context, text string) (SentimentResult, error) {
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return SentimentResult{Label: "neutral", Score: 0.5}, nil
	}

	positiveScore := 0.0
	negativeScore := 0.0
	totalWords := 0.0

	for _, word := range words {
		// Clean word
		cleaned := regexp.MustCompile(`[^\w]`).ReplaceAllString(word, "")
		if len(cleaned) < 2 {
			continue
		}

		totalWords++
		if score, exists := m.positiveWords[cleaned]; exists {
			positiveScore += score
		}
		if score, exists := m.negativeWords[cleaned]; exists {
			negativeScore += score
		}
	}

	if totalWords == 0 {
		return SentimentResult{Label: "neutral", Score: 0.5}, nil
	}

	// Normalize scores
	positiveScore /= totalWords
	negativeScore /= totalWords

	// Determine sentiment
	netScore := positiveScore - negativeScore
	magnitude := math.Abs(netScore)
	
	var label string
	var confidence float64

	if netScore > 0.1 {
		label = "positive"
		confidence = math.Min(0.5+magnitude, 1.0)
	} else if netScore < -0.1 {
		label = "negative"
		confidence = math.Min(0.5+magnitude, 1.0)
	} else {
		label = "neutral"
		confidence = 1.0 - magnitude
	}

	// Calculate subjectivity based on emotional words
	subjectivity := math.Min((positiveScore+negativeScore)*2, 1.0)

	return SentimentResult{
		Label:        label,
		Score:        confidence,
		Magnitude:    magnitude,
		Subjectivity: subjectivity,
	}, nil
}

func (m *HeuristicSentimentModel) GetModelVersion() string {
	return m.version
}

// HeuristicTopicModel implements basic topic modeling using keyword clustering
type HeuristicTopicModel struct {
	topicKeywords map[string][]string
	version       string
}

// NewHeuristicTopicModel creates a new heuristic topic model
func NewHeuristicTopicModel() *HeuristicTopicModel {
	return &HeuristicTopicModel{
		topicKeywords: getTopicKeywords(),
		version:       "heuristic-v1.0",
	}
}

// ExtractTopics identifies topics based on keyword matching
func (m *HeuristicTopicModel) ExtractTopics(ctx context.Context, text string, maxTopics int) ([]TopicResult, error) {
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return []TopicResult{}, nil
	}

	topicScores := make(map[string]float64)
	wordSet := make(map[string]bool)
	
	// Create word set for faster lookup
	for _, word := range words {
		cleaned := regexp.MustCompile(`[^\w]`).ReplaceAllString(word, "")
		if len(cleaned) > 2 {
			wordSet[cleaned] = true
		}
	}

	// Score topics based on keyword matches
	for topic, keywords := range m.topicKeywords {
		score := 0.0
		matchedKeywords := []string{}
		
		for _, keyword := range keywords {
			if wordSet[keyword] {
				score += 1.0
				matchedKeywords = append(matchedKeywords, keyword)
			}
		}
		
		if score > 0 {
			// Normalize by topic keyword count and text length
			probability := score / float64(len(keywords))
			coherence := score / math.Sqrt(float64(len(words)))
			
			topicScores[topic] = probability * coherence
		}
	}

	// Convert to results and sort
	var results []TopicResult
	for topic, score := range topicScores {
		if score > 0.1 { // Minimum threshold
			results = append(results, TopicResult{
				Topic:       topic,
				Keywords:    m.topicKeywords[topic][:min(5, len(m.topicKeywords[topic]))],
				Probability: math.Min(score, 1.0),
				Coherence:   score,
			})
		}
	}

	// Sort by probability
	sort.Slice(results, func(i, j int) bool {
		return results[i].Probability > results[j].Probability
	})

	// Return top results
	if len(results) > maxTopics {
		results = results[:maxTopics]
	}

	return results, nil
}

func (m *HeuristicTopicModel) GetModelVersion() string {
	return m.version
}

// HeuristicEntityModel implements basic named entity recognition using pattern matching
type HeuristicEntityModel struct {
	patterns map[string]*regexp.Regexp
	version  string
}

// NewHeuristicEntityModel creates a new heuristic entity model
func NewHeuristicEntityModel() *HeuristicEntityModel {
	return &HeuristicEntityModel{
		patterns: getEntityPatterns(),
		version:  "heuristic-v1.0",
	}
}

// ExtractEntities finds named entities using regex patterns
func (m *HeuristicEntityModel) ExtractEntities(ctx context.Context, text string) ([]EntityResult, error) {
	var entities []EntityResult

	for entityType, pattern := range m.patterns {
		matches := pattern.FindAllStringIndex(text, -1)
		for _, match := range matches {
			entityText := text[match[0]:match[1]]
			
			// Skip very short entities
			if len(entityText) < 2 {
				continue
			}

			// Extract context (surrounding text)
			contextStart := max(0, match[0]-20)
			contextEnd := min(len(text), match[1]+20)
			context := text[contextStart:contextEnd]

			// Calculate confidence based on pattern specificity
			confidence := m.calculateEntityConfidence(entityType, entityText)

			entities = append(entities, EntityResult{
				Text:       entityText,
				Label:      entityType,
				StartPos:   match[0],
				EndPos:     match[1],
				Confidence: confidence,
				Context:    context,
			})
		}
	}

	// Sort by position
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].StartPos < entities[j].StartPos
	})

	return entities, nil
}

func (m *HeuristicEntityModel) calculateEntityConfidence(entityType, text string) float64 {
	confidence := 0.7 // Base confidence

	switch entityType {
	case "EMAIL":
		confidence = 0.95
	case "PHONE":
		confidence = 0.9
	case "URL":
		confidence = 0.9
	case "PERSON":
		// Higher confidence for common name patterns
		if regexp.MustCompile(`^[A-Z][a-z]+ [A-Z][a-z]+$`).MatchString(text) {
			confidence = 0.8
		} else {
			confidence = 0.6
		}
	case "ORG":
		if strings.Contains(text, "Inc") || strings.Contains(text, "Corp") || strings.Contains(text, "LLC") {
			confidence = 0.85
		} else {
			confidence = 0.6
		}
	}

	return confidence
}

func (m *HeuristicEntityModel) GetModelVersion() string {
	return m.version
}

// HeuristicSummaryModel implements extractive summarization using sentence scoring
type HeuristicSummaryModel struct {
	version string
}

// NewHeuristicSummaryModel creates a new heuristic summary model
func NewHeuristicSummaryModel() *HeuristicSummaryModel {
	return &HeuristicSummaryModel{
		version: "heuristic-v1.0",
	}
}

// GenerateSummary creates extractive and basic abstractive summaries
func (m *HeuristicSummaryModel) GenerateSummary(ctx context.Context, text string, ratio float64) (SummaryResult, error) {
	sentences := m.splitIntoSentences(text)
	if len(sentences) == 0 {
		return SummaryResult{}, nil
	}

	if len(sentences) == 1 {
		return SummaryResult{
			ExtractiveSummary: sentences[0],
			AbstractiveSummary: sentences[0],
			KeySentences:      sentences,
			SummaryRatio:      1.0,
			CompressionScore:  0.0,
		}, nil
	}

	// Score sentences
	sentenceScores := m.scoreSentences(sentences, text)

	// Select top sentences for summary
	numSentences := max(1, int(float64(len(sentences))*ratio))
	topSentences := m.selectTopSentences(sentences, sentenceScores, numSentences)

	// Create extractive summary
	extractiveSummary := strings.Join(topSentences, " ")

	// Create basic abstractive summary (simplified version)
	abstractiveSummary := m.createAbstractiveSummary(topSentences)

	compressionScore := 1.0 - float64(len(extractiveSummary))/float64(len(text))

	return SummaryResult{
		ExtractiveSummary: extractiveSummary,
		AbstractiveSummary: abstractiveSummary,
		KeySentences:      topSentences,
		SummaryRatio:      float64(len(topSentences)) / float64(len(sentences)),
		CompressionScore:  compressionScore,
	}, nil
}

func (m *HeuristicSummaryModel) splitIntoSentences(text string) []string {
	// Simple sentence splitting
	sentences := regexp.MustCompile(`[.!?]+\s+`).Split(text, -1)
	var result []string
	
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) > 10 { // Minimum sentence length
			result = append(result, sentence)
		}
	}
	
	return result
}

func (m *HeuristicSummaryModel) scoreSentences(sentences []string, fullText string) []float64 {
	scores := make([]float64, len(sentences))
	
	// Calculate word frequencies in the full text
	wordFreq := make(map[string]int)
	words := strings.Fields(strings.ToLower(fullText))
	for _, word := range words {
		cleaned := regexp.MustCompile(`[^\w]`).ReplaceAllString(word, "")
		if len(cleaned) > 2 {
			wordFreq[cleaned]++
		}
	}

	// Score each sentence
	for i, sentence := range sentences {
		sentenceWords := strings.Fields(strings.ToLower(sentence))
		score := 0.0
		
		for _, word := range sentenceWords {
			cleaned := regexp.MustCompile(`[^\w]`).ReplaceAllString(word, "")
			if freq, exists := wordFreq[cleaned]; exists && len(cleaned) > 2 {
				score += float64(freq)
			}
		}
		
		// Normalize by sentence length
		if len(sentenceWords) > 0 {
			scores[i] = score / float64(len(sentenceWords))
		}
		
		// Boost score for sentences at the beginning or end
		if i == 0 || i == len(sentences)-1 {
			scores[i] *= 1.2
		}
	}
	
	return scores
}

func (m *HeuristicSummaryModel) selectTopSentences(sentences []string, scores []float64, numSentences int) []string {
	type sentenceScore struct {
		sentence string
		score    float64
		index    int
	}
	
	var scored []sentenceScore
	for i, sentence := range sentences {
		scored = append(scored, sentenceScore{
			sentence: sentence,
			score:    scores[i],
			index:    i,
		})
	}
	
	// Sort by score
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	
	// Take top sentences
	selected := scored[:min(numSentences, len(scored))]
	
	// Sort by original order
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].index < selected[j].index
	})
	
	var result []string
	for _, s := range selected {
		result = append(result, s.sentence)
	}
	
	return result
}

func (m *HeuristicSummaryModel) createAbstractiveSummary(sentences []string) string {
	// Very basic abstractive summary: just combine and simplify key sentences
	if len(sentences) == 0 {
		return ""
	}
	
	summary := strings.Join(sentences, " ")
	
	// Basic simplification rules
	summary = regexp.MustCompile(`\s+`).ReplaceAllString(summary, " ")
	summary = strings.TrimSpace(summary)
	
	// Ensure it doesn't exceed a reasonable length
	if len(summary) > 500 {
		words := strings.Fields(summary)
		if len(words) > 80 {
			summary = strings.Join(words[:80], " ") + "..."
		}
	}
	
	return summary
}

func (m *HeuristicSummaryModel) GetModelVersion() string {
	return m.version
}

// HeuristicQualityModel implements content quality assessment
type HeuristicQualityModel struct {
	version string
}

// NewHeuristicQualityModel creates a new heuristic quality model
func NewHeuristicQualityModel() *HeuristicQualityModel {
	return &HeuristicQualityModel{
		version: "heuristic-v1.0",
	}
}

// AssessQuality evaluates content quality across multiple dimensions
func (m *HeuristicQualityModel) AssessQuality(ctx context.Context, text string) (QualityMetrics, error) {
	if len(text) == 0 {
		return QualityMetrics{}, nil
	}

	sentences := regexp.MustCompile(`[.!?]+`).Split(text, -1)
	words := strings.Fields(text)
	
	// Calculate various quality metrics
	coherence := m.assessCoherence(sentences)
	clarity := m.assessClarity(text, words)
	completeness := m.assessCompleteness(text)
	informativeness := m.assessInformativeness(words)
	redundancy := m.assessRedundancy(sentences)
	structuralQuality := m.assessStructuralQuality(text)
	
	issues := m.identifyIssues(text, words, sentences)
	
	// Calculate overall score
	scores := []float64{coherence, clarity, completeness, informativeness, 1.0-redundancy, structuralQuality}
	overallScore := 0.0
	for _, score := range scores {
		overallScore += score
	}
	overallScore /= float64(len(scores))

	return QualityMetrics{
		Coherence:         coherence,
		Clarity:           clarity,
		Completeness:      completeness,
		Informativeness:   informativeness,
		Redundancy:        redundancy,
		StructuralQuality: structuralQuality,
		Issues:            issues,
		OverallScore:      overallScore,
	}, nil
}

func (m *HeuristicQualityModel) assessCoherence(sentences []string) float64 {
	if len(sentences) < 2 {
		return 1.0
	}
	
	// Simple coherence based on sentence length variation
	lengths := make([]int, 0, len(sentences))
	for _, sentence := range sentences {
		if len(strings.TrimSpace(sentence)) > 0 {
			lengths = append(lengths, len(strings.Fields(sentence)))
		}
	}
	
	if len(lengths) == 0 {
		return 0.0
	}
	
	// Calculate coefficient of variation
	mean := 0.0
	for _, length := range lengths {
		mean += float64(length)
	}
	mean /= float64(len(lengths))
	
	variance := 0.0
	for _, length := range lengths {
		variance += math.Pow(float64(length)-mean, 2)
	}
	variance /= float64(len(lengths))
	
	stdDev := math.Sqrt(variance)
	cv := stdDev / mean
	
	// Lower coefficient of variation indicates better coherence
	coherence := 1.0 - math.Min(cv/2.0, 1.0)
	return coherence
}

func (m *HeuristicQualityModel) assessClarity(text string, words []string) float64 {
	if len(words) == 0 {
		return 0.0
	}
	
	// Assess clarity based on word complexity and sentence structure
	complexWords := 0
	for _, word := range words {
		if len(word) > 12 || m.countSyllables(word) > 3 {
			complexWords++
		}
	}
	
	complexityRatio := float64(complexWords) / float64(len(words))
	clarity := 1.0 - math.Min(complexityRatio*2, 1.0)
	
	return clarity
}

func (m *HeuristicQualityModel) assessCompleteness(text string) float64 {
	// Simple completeness based on text length and structure indicators
	score := 0.0
	
	// Length indicator
	if len(text) > 100 {
		score += 0.3
	}
	if len(text) > 500 {
		score += 0.2
	}
	
	// Structure indicators
	if strings.Contains(text, ".") {
		score += 0.2
	}
	if regexp.MustCompile(`\d+`).MatchString(text) {
		score += 0.1
	}
	if regexp.MustCompile(`[A-Z][a-z]+`).MatchString(text) {
		score += 0.2
	}
	
	return math.Min(score, 1.0)
}

func (m *HeuristicQualityModel) assessInformativeness(words []string) float64 {
	if len(words) == 0 {
		return 0.0
	}
	
	// Assess based on vocabulary diversity
	uniqueWords := make(map[string]bool)
	for _, word := range words {
		cleaned := strings.ToLower(regexp.MustCompile(`[^\w]`).ReplaceAllString(word, ""))
		if len(cleaned) > 2 {
			uniqueWords[cleaned] = true
		}
	}
	
	diversity := float64(len(uniqueWords)) / float64(len(words))
	return math.Min(diversity*2, 1.0)
}

func (m *HeuristicQualityModel) assessRedundancy(sentences []string) float64 {
	if len(sentences) < 2 {
		return 0.0
	}
	
	// Simple redundancy detection based on sentence similarity
	redundantPairs := 0
	totalPairs := 0
	
	for i := 0; i < len(sentences); i++ {
		for j := i + 1; j < len(sentences); j++ {
			if len(sentences[i]) > 10 && len(sentences[j]) > 10 {
				similarity := m.calculateSimilarity(sentences[i], sentences[j])
				if similarity > 0.8 {
					redundantPairs++
				}
				totalPairs++
			}
		}
	}
	
	if totalPairs == 0 {
		return 0.0
	}
	
	return float64(redundantPairs) / float64(totalPairs)
}

func (m *HeuristicQualityModel) assessStructuralQuality(text string) float64 {
	score := 0.0
	
	// Check for proper capitalization
	if regexp.MustCompile(`^[A-Z]`).MatchString(text) {
		score += 0.2
	}
	
	// Check for proper punctuation
	if regexp.MustCompile(`[.!?]$`).MatchString(strings.TrimSpace(text)) {
		score += 0.2
	}
	
	// Check for paragraph structure
	if strings.Contains(text, "\n\n") {
		score += 0.3
	}
	
	// Check for reasonable sentence length distribution
	sentences := regexp.MustCompile(`[.!?]+`).Split(text, -1)
	reasonableLengths := 0
	for _, sentence := range sentences {
		words := len(strings.Fields(sentence))
		if words >= 5 && words <= 30 {
			reasonableLengths++
		}
	}
	
	if len(sentences) > 0 {
		score += 0.3 * float64(reasonableLengths) / float64(len(sentences))
	}
	
	return math.Min(score, 1.0)
}

func (m *HeuristicQualityModel) identifyIssues(text string, words []string, sentences []string) []string {
	var issues []string
	
	// Check for common issues
	if len(text) < 50 {
		issues = append(issues, "Text too short")
	}
	
	if len(sentences) == 1 && len(words) > 50 {
		issues = append(issues, "Single long sentence")
	}
	
	// Check for excessive repetition
	wordCount := make(map[string]int)
	for _, word := range words {
		cleaned := strings.ToLower(regexp.MustCompile(`[^\w]`).ReplaceAllString(word, ""))
		if len(cleaned) > 3 {
			wordCount[cleaned]++
		}
	}
	
	for word, count := range wordCount {
		if count > len(words)/10 && count > 3 {
			issues = append(issues, fmt.Sprintf("Excessive repetition of '%s'", word))
		}
	}
	
	// Check for missing punctuation
	if !regexp.MustCompile(`[.!?]`).MatchString(text) {
		issues = append(issues, "Missing punctuation")
	}
	
	return issues
}

func (m *HeuristicQualityModel) calculateSimilarity(s1, s2 string) float64 {
	words1 := strings.Fields(strings.ToLower(s1))
	words2 := strings.Fields(strings.ToLower(s2))
	
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	
	for _, word := range words1 {
		set1[word] = true
	}
	for _, word := range words2 {
		set2[word] = true
	}
	
	intersection := 0
	for word := range set1 {
		if set2[word] {
			intersection++
		}
	}
	
	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 0.0
	}
	
	return float64(intersection) / float64(union)
}

func (m *HeuristicQualityModel) countSyllables(word string) int {
	word = strings.ToLower(word)
	vowels := "aeiouy"
	syllables := 0
	previousWasVowel := false
	
	for _, char := range word {
		isVowel := strings.ContainsRune(vowels, char)
		if isVowel && !previousWasVowel {
			syllables++
		}
		previousWasVowel = isVowel
	}
	
	// Adjust for silent e
	if strings.HasSuffix(word, "e") && syllables > 1 {
		syllables--
	}
	
	if syllables == 0 {
		syllables = 1
	}
	
	return syllables
}

func (m *HeuristicQualityModel) GetModelVersion() string {
	return m.version
}

// HeuristicLanguageModel implements language detection using character frequency analysis
type HeuristicLanguageModel struct {
	version string
}

// NewHeuristicLanguageModel creates a new heuristic language model
func NewHeuristicLanguageModel() *HeuristicLanguageModel {
	return &HeuristicLanguageModel{
		version: "heuristic-v1.0",
	}
}

// DetectLanguage identifies the language of the text
func (m *HeuristicLanguageModel) DetectLanguage(ctx context.Context, text string) (LanguageResult, error) {
	if len(text) == 0 {
		return LanguageResult{}, nil
	}

	// Simple language detection based on character patterns and common words
	language := "en" // Default to English
	confidence := 0.8
	script := "Latin"

	// Check for non-Latin scripts
	if m.hasChineseCharacters(text) {
		language = "zh"
		script = "Chinese"
		confidence = 0.9
	} else if m.hasArabicCharacters(text) {
		language = "ar"
		script = "Arabic"
		confidence = 0.9
	} else if m.hasCyrillicCharacters(text) {
		language = "ru"
		script = "Cyrillic"
		confidence = 0.8
	} else {
		// Detect European languages based on common words
		language, confidence = m.detectLatinLanguage(text)
	}

	alternatives := []LanguageAlternative{
		{Language: "en", Confidence: 0.3},
		{Language: "es", Confidence: 0.2},
	}

	return LanguageResult{
		Language:     language,
		Dialect:      "",
		Confidence:   confidence,
		Alternatives: alternatives,
		Script:       script,
	}, nil
}

func (m *HeuristicLanguageModel) hasChineseCharacters(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func (m *HeuristicLanguageModel) hasArabicCharacters(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Arabic, r) {
			return true
		}
	}
	return false
}

func (m *HeuristicLanguageModel) hasCyrillicCharacters(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func (m *HeuristicLanguageModel) detectLatinLanguage(text string) (string, float64) {
	text = strings.ToLower(text)
	
	// Common words for different languages
	englishWords := []string{"the", "and", "is", "in", "to", "of", "a", "that", "it", "with"}
	spanishWords := []string{"el", "la", "de", "que", "y", "a", "en", "un", "es", "se"}
	frenchWords := []string{"le", "de", "et", "à", "un", "il", "être", "et", "en", "avoir"}
	germanWords := []string{"der", "die", "und", "in", "den", "von", "zu", "das", "mit", "sich"}

	englishScore := m.countCommonWords(text, englishWords)
	spanishScore := m.countCommonWords(text, spanishWords)
	frenchScore := m.countCommonWords(text, frenchWords)
	germanScore := m.countCommonWords(text, germanWords)

	maxScore := math.Max(math.Max(englishScore, spanishScore), math.Max(frenchScore, germanScore))
	
	if maxScore == 0 {
		return "en", 0.5 // Default to English with low confidence
	}

	confidence := math.Min(maxScore/10.0, 1.0) // Normalize confidence

	if englishScore == maxScore {
		return "en", confidence
	} else if spanishScore == maxScore {
		return "es", confidence
	} else if frenchScore == maxScore {
		return "fr", confidence
	} else if germanScore == maxScore {
		return "de", confidence
	}

	return "en", 0.5
}

func (m *HeuristicLanguageModel) countCommonWords(text string, commonWords []string) float64 {
	words := strings.Fields(text)
	matches := 0.0
	
	for _, word := range words {
		for _, common := range commonWords {
			if word == common {
				matches++
				break
			}
		}
	}
	
	return matches
}

func (m *HeuristicLanguageModel) GetModelVersion() string {
	return m.version
}

// HeuristicEmotionModel implements emotional tone analysis
type HeuristicEmotionModel struct {
	emotionWords map[string]map[string]float64
	version      string
}

// NewHeuristicEmotionModel creates a new heuristic emotion model
func NewHeuristicEmotionModel() *HeuristicEmotionModel {
	return &HeuristicEmotionModel{
		emotionWords: getEmotionWords(),
		version:      "heuristic-v1.0",
	}
}

// AnalyzeEmotion performs emotional tone analysis
func (m *HeuristicEmotionModel) AnalyzeEmotion(ctx context.Context, text string) (EmotionalToneResult, error) {
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return EmotionalToneResult{}, nil
	}

	scores := make(map[string]float64)
	totalWords := float64(len(words))

	// Initialize emotion scores
	emotions := []string{"joy", "anger", "fear", "sadness", "analytical", "confident", "tentative", "openness"}
	for _, emotion := range emotions {
		scores[emotion] = 0.0
	}

	// Score each word
	for _, word := range words {
		cleaned := regexp.MustCompile(`[^\w]`).ReplaceAllString(word, "")
		if len(cleaned) < 2 {
			continue
		}

		for emotion, emotionWords := range m.emotionWords {
			if score, exists := emotionWords[cleaned]; exists {
				scores[emotion] += score
			}
		}
	}

	// Normalize scores
	for emotion := range scores {
		scores[emotion] = math.Min(scores[emotion]/totalWords*10, 1.0) // Scale and cap at 1.0
	}

	return EmotionalToneResult{
		Joy:        scores["joy"],
		Anger:      scores["anger"],
		Fear:       scores["fear"],
		Sadness:    scores["sadness"],
		Analytical: scores["analytical"],
		Confident:  scores["confident"],
		Tentative:  scores["tentative"],
		Openness:   scores["openness"],
	}, nil
}

func (m *HeuristicEmotionModel) GetModelVersion() string {
	return m.version
}

// Helper functions and data

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Lexicon data for sentiment analysis
func getPositiveWords() map[string]float64 {
	return map[string]float64{
		"good":        1.0,
		"great":       1.2,
		"excellent":   1.5,
		"amazing":     1.4,
		"wonderful":   1.3,
		"fantastic":   1.4,
		"awesome":     1.3,
		"outstanding": 1.5,
		"perfect":     1.4,
		"brilliant":   1.3,
		"positive":    1.0,
		"happy":       1.1,
		"love":        1.2,
		"best":        1.3,
		"better":      0.8,
		"success":     1.1,
		"successful":  1.1,
		"achieve":     0.9,
		"achievement": 1.0,
		"win":         1.0,
		"winner":      1.1,
	}
}

func getNegativeWords() map[string]float64 {
	return map[string]float64{
		"bad":        1.0,
		"terrible":   1.4,
		"awful":      1.3,
		"horrible":   1.3,
		"worst":      1.4,
		"hate":       1.2,
		"negative":   1.0,
		"problem":    0.8,
		"issue":      0.7,
		"error":      0.9,
		"fail":       1.1,
		"failure":    1.2,
		"wrong":      0.8,
		"difficult":  0.7,
		"hard":       0.6,
		"impossible": 1.1,
		"sad":        1.0,
		"angry":      1.1,
		"disappointed": 1.0,
		"frustrating":  0.9,
	}
}

// Topic keywords for topic modeling
func getTopicKeywords() map[string][]string {
	return map[string][]string{
		"technology": {"software", "computer", "digital", "internet", "web", "application", "system", "data", "programming", "code"},
		"business":   {"company", "market", "sales", "revenue", "profit", "customer", "product", "service", "strategy", "management"},
		"finance":    {"money", "bank", "investment", "stock", "financial", "budget", "cost", "price", "economic", "currency"},
		"health":     {"medical", "doctor", "patient", "hospital", "treatment", "disease", "health", "medicine", "care", "therapy"},
		"education":  {"school", "student", "teacher", "university", "learning", "education", "study", "knowledge", "academic", "research"},
		"legal":      {"law", "legal", "court", "judge", "attorney", "contract", "regulation", "compliance", "policy", "rights"},
		"science":    {"research", "study", "experiment", "analysis", "scientific", "theory", "hypothesis", "data", "method", "result"},
		"politics":   {"government", "political", "policy", "election", "vote", "democracy", "parliament", "senator", "congress", "legislation"},
	}
}

// Entity recognition patterns
func getEntityPatterns() map[string]*regexp.Regexp {
	return map[string]*regexp.Regexp{
		"EMAIL":  regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		"PHONE":  regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b|\(\d{3}\)\s?\d{3}[-.]?\d{4}`),
		"URL":    regexp.MustCompile(`https?://[^\s]+`),
		"PERSON": regexp.MustCompile(`\b[A-Z][a-z]+\s+[A-Z][a-z]+\b`),
		"ORG":    regexp.MustCompile(`\b[A-Z][a-zA-Z]*\s+(Inc|Corp|LLC|Ltd|Company|Corporation)\b`),
		"MONEY":  regexp.MustCompile(`\$\d+(?:,\d{3})*(?:\.\d{2})?`),
		"DATE":   regexp.MustCompile(`\b\d{1,2}[/-]\d{1,2}[/-]\d{2,4}\b|\b\d{4}-\d{2}-\d{2}\b`),
	}
}

// Emotion word lexicons
func getEmotionWords() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"joy": {
			"happy":     1.0,
			"joy":       1.0,
			"excited":   0.9,
			"pleased":   0.8,
			"delighted": 0.9,
			"cheerful":  0.8,
			"thrilled":  1.0,
			"elated":    0.9,
		},
		"anger": {
			"angry":     1.0,
			"mad":       0.9,
			"furious":   1.0,
			"irritated": 0.7,
			"annoyed":   0.6,
			"outraged":  1.0,
			"livid":     0.9,
			"irate":     0.8,
		},
		"fear": {
			"afraid":    1.0,
			"scared":    0.9,
			"terrified": 1.0,
			"nervous":   0.7,
			"anxious":   0.8,
			"worried":   0.6,
			"fearful":   0.9,
			"panic":     1.0,
		},
		"sadness": {
			"sad":        1.0,
			"depressed":  0.9,
			"miserable":  1.0,
			"unhappy":    0.8,
			"sorrowful":  0.9,
			"melancholy": 0.8,
			"grief":      1.0,
			"despair":    1.0,
		},
		"analytical": {
			"analyze":    1.0,
			"research":   0.9,
			"study":      0.8,
			"examine":    0.9,
			"investigate": 0.9,
			"logical":    0.8,
			"systematic": 0.9,
			"methodical": 0.8,
		},
		"confident": {
			"confident":  1.0,
			"certain":    0.9,
			"sure":       0.8,
			"convinced":  0.9,
			"determined": 0.8,
			"assertive":  0.9,
			"bold":       0.8,
			"decisive":   0.9,
		},
		"tentative": {
			"maybe":      0.8,
			"perhaps":    0.7,
			"possibly":   0.7,
			"uncertain":  0.9,
			"doubtful":   0.8,
			"hesitant":   0.9,
			"unsure":     0.8,
			"tentative":  1.0,
		},
		"openness": {
			"open":       0.8,
			"curious":    0.9,
			"interested": 0.8,
			"explore":    0.9,
			"discover":   0.8,
			"learn":      0.7,
			"innovative": 0.9,
			"creative":   0.8,
		},
	}
}