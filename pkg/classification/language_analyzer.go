package classification

import (
	"context"
	"math"
	"regexp"
	"strings"
	"unicode"
)

// LanguageAnalyzer provides comprehensive language analysis
type LanguageAnalyzer struct {
	name                string
	version             string
	languageDetector    LanguageDetector
	linguisticAnalyzers map[Language]*LinguisticAnalyzer
}

// LinguisticAnalysis represents comprehensive language analysis results
type LinguisticAnalysis struct {
	Language           Language             `json:"language"`
	Confidence         float64              `json:"confidence"`
	Dialect            string               `json:"dialect,omitempty"`
	TextComplexity     TextComplexity       `json:"text_complexity"`
	ReadabilityScores  ReadabilityScores    `json:"readability_scores"`
	LinguisticFeatures LinguisticFeatures   `json:"linguistic_features"`
	VocabularyStats    VocabularyStatistics `json:"vocabulary_stats"`
	SyntaxAnalysis     SyntaxAnalysis       `json:"syntax_analysis"`
	SemanticFeatures   SemanticFeatures     `json:"semantic_features"`
	Candidates         []LanguageCandidate  `json:"candidates,omitempty"`
}

// TextComplexity represents text complexity metrics
type TextComplexity struct {
	Level                string  `json:"level"` // elementary, intermediate, advanced, expert
	Score                float64 `json:"score"` // 0-100
	FleschKincaidGrade   float64 `json:"flesch_kincaid_grade"`
	FleschReadingEase    float64 `json:"flesch_reading_ease"`
	GunningFogIndex      float64 `json:"gunning_fog_index"`
	ColemanLiauIndex     float64 `json:"coleman_liau_index"`
	AutomatedReadability float64 `json:"automated_readability"`
}

// ReadabilityScores contains various readability measurements
type ReadabilityScores struct {
	FleschKincaid float64 `json:"flesch_kincaid"`
	FleschReading float64 `json:"flesch_reading"`
	GunningFog    float64 `json:"gunning_fog"`
	SMOG          float64 `json:"smog"`
	ColemanLiau   float64 `json:"coleman_liau"`
	ARI           float64 `json:"ari"`
	ReadingLevel  string  `json:"reading_level"`
}

// LinguisticFeatures represents linguistic characteristics
type LinguisticFeatures struct {
	AverageSentenceLength   float64 `json:"avg_sentence_length"`
	AverageWordsPerSentence float64 `json:"avg_words_per_sentence"`
	AverageSyllablesPerWord float64 `json:"avg_syllables_per_word"`
	LexicalDiversity        float64 `json:"lexical_diversity"`
	TypeTokenRatio          float64 `json:"type_token_ratio"`
	HapaxLegomena           int     `json:"hapax_legomena"` // Words that occur only once
	ComplexWordPercentage   float64 `json:"complex_word_percentage"`
	FormalityScore          float64 `json:"formality_score"`
	AbstractnessScore       float64 `json:"abstractness_score"`
}

// VocabularyStatistics represents vocabulary analysis
type VocabularyStatistics struct {
	TotalWords        int                `json:"total_words"`
	UniqueWords       int                `json:"unique_words"`
	VocabularySize    int                `json:"vocabulary_size"`
	WordFrequency     map[string]int     `json:"word_frequency,omitempty"`
	CommonWords       []string           `json:"common_words"`
	RareWords         []string           `json:"rare_words"`
	LongWords         []string           `json:"long_words"`
	WordLengthDistrib map[int]int        `json:"word_length_distribution"`
	PartOfSpeechDist  map[string]float64 `json:"part_of_speech_distribution,omitempty"`
}

// SyntaxAnalysis represents syntactic analysis
type SyntaxAnalysis struct {
	SentenceTypes         map[string]int `json:"sentence_types"` // declarative, interrogative, exclamatory
	PunctuationDensity    float64        `json:"punctuation_density"`
	CapitalizationPattern string         `json:"capitalization_pattern"`
	QuestionCount         int            `json:"question_count"`
	ExclamationCount      int            `json:"exclamation_count"`
	PassiveVoiceCount     int            `json:"passive_voice_count"`
	ConjunctionDensity    float64        `json:"conjunction_density"`
}

// SemanticFeatures represents semantic characteristics
type SemanticFeatures struct {
	TopicCoherence       float64  `json:"topic_coherence"`
	SemanticDensity      float64  `json:"semantic_density"`
	ConceptualComplexity float64  `json:"conceptual_complexity"`
	AbstractConcepts     []string `json:"abstract_concepts,omitempty"`
	TechnicalTerms       []string `json:"technical_terms,omitempty"`
	EmotionalTone        string   `json:"emotional_tone"`
	Subjectivity         float64  `json:"subjectivity"`
}

// LinguisticAnalyzer analyzes linguistic features for a specific language
type LinguisticAnalyzer struct {
	language         Language
	stopWords        map[string]bool
	commonWords      map[string]bool
	syllablePatterns []*regexp.Regexp
	complexityRules  map[string]float64
}

// NewLanguageAnalyzer creates a new language analyzer
func NewLanguageAnalyzer() *LanguageAnalyzer {
	analyzer := &LanguageAnalyzer{
		name:                "comprehensive-language-analyzer",
		version:             "1.0.0",
		languageDetector:    NewBasicLanguageDetector(),
		linguisticAnalyzers: make(map[Language]*LinguisticAnalyzer),
	}

	analyzer.initializeLinguisticAnalyzers()
	return analyzer
}

// AnalyzeLanguage performs comprehensive language analysis
func (l *LanguageAnalyzer) AnalyzeLanguage(ctx context.Context, text string) (*LinguisticAnalysis, error) {
	if len(text) == 0 {
		return &LinguisticAnalysis{
			Language:   LanguageUnknown,
			Confidence: 0.0,
		}, nil
	}

	// First detect the language
	langResult, err := l.languageDetector.DetectLanguage(ctx, text)
	if err != nil {
		return nil, err
	}

	analysis := &LinguisticAnalysis{
		Language:   langResult.Language,
		Confidence: langResult.Confidence,
		Candidates: langResult.Candidates,
	}

	// Get linguistic analyzer for the detected language
	linguisticAnalyzer, exists := l.linguisticAnalyzers[langResult.Language]
	if !exists {
		// Fallback to English analyzer
		linguisticAnalyzer = l.linguisticAnalyzers[LanguageEnglish]
	}

	if linguisticAnalyzer != nil {
		// Perform comprehensive analysis
		analysis.TextComplexity = l.analyzeTextComplexity(text, linguisticAnalyzer)
		analysis.ReadabilityScores = l.calculateReadabilityScores(text, linguisticAnalyzer)
		analysis.LinguisticFeatures = l.analyzeLinguisticFeatures(text, linguisticAnalyzer)
		analysis.VocabularyStats = l.analyzeVocabulary(text, linguisticAnalyzer)
		analysis.SyntaxAnalysis = l.analyzeSyntax(text, linguisticAnalyzer)
		analysis.SemanticFeatures = l.analyzeSemanticFeatures(text, linguisticAnalyzer)
	}

	return analysis, nil
}

// analyzeTextComplexity analyzes overall text complexity
func (l *LanguageAnalyzer) analyzeTextComplexity(text string, analyzer *LinguisticAnalyzer) TextComplexity {
	sentences := l.splitIntoSentences(text)
	words := strings.Fields(text)

	if len(sentences) == 0 || len(words) == 0 {
		return TextComplexity{}
	}

	// Calculate various readability metrics
	fleschKincaid := l.calculateFleschKincaid(text, analyzer)
	fleschReading := l.calculateFleschReadingEase(text, analyzer)
	gunningFog := l.calculateGunningFog(text, analyzer)
	colemanLiau := l.calculateColemanLiau(text)
	ari := l.calculateARI(text)

	// Determine complexity level
	complexityScore := (fleschKincaid + gunningFog + colemanLiau + ari) / 4.0

	var level string
	switch {
	case complexityScore <= 6:
		level = "elementary"
	case complexityScore <= 9:
		level = "intermediate"
	case complexityScore <= 13:
		level = "advanced"
	default:
		level = "expert"
	}

	return TextComplexity{
		Level:                level,
		Score:                math.Min(complexityScore*10, 100),
		FleschKincaidGrade:   fleschKincaid,
		FleschReadingEase:    fleschReading,
		GunningFogIndex:      gunningFog,
		ColemanLiauIndex:     colemanLiau,
		AutomatedReadability: ari,
	}
}

// calculateReadabilityScores calculates various readability scores
func (l *LanguageAnalyzer) calculateReadabilityScores(text string, analyzer *LinguisticAnalyzer) ReadabilityScores {
	fleschKincaid := l.calculateFleschKincaid(text, analyzer)
	fleschReading := l.calculateFleschReadingEase(text, analyzer)
	gunningFog := l.calculateGunningFog(text, analyzer)
	smog := l.calculateSMOG(text, analyzer)
	colemanLiau := l.calculateColemanLiau(text)
	ari := l.calculateARI(text)

	// Determine reading level based on average of scores
	avgScore := (fleschKincaid + gunningFog + colemanLiau + ari) / 4.0

	var readingLevel string
	switch {
	case avgScore <= 6:
		readingLevel = "Elementary"
	case avgScore <= 8:
		readingLevel = "Middle School"
	case avgScore <= 12:
		readingLevel = "High School"
	case avgScore <= 16:
		readingLevel = "College"
	default:
		readingLevel = "Graduate"
	}

	return ReadabilityScores{
		FleschKincaid: fleschKincaid,
		FleschReading: fleschReading,
		GunningFog:    gunningFog,
		SMOG:          smog,
		ColemanLiau:   colemanLiau,
		ARI:           ari,
		ReadingLevel:  readingLevel,
	}
}

// analyzeLinguisticFeatures analyzes linguistic characteristics
func (l *LanguageAnalyzer) analyzeLinguisticFeatures(text string, analyzer *LinguisticAnalyzer) LinguisticFeatures {
	sentences := l.splitIntoSentences(text)
	words := strings.Fields(text)

	if len(sentences) == 0 || len(words) == 0 {
		return LinguisticFeatures{}
	}

	// Calculate averages
	avgSentenceLength := float64(len(text)) / float64(len(sentences))
	avgWordsPerSentence := float64(len(words)) / float64(len(sentences))

	// Calculate syllables
	totalSyllables := 0
	complexWords := 0
	for _, word := range words {
		syllables := l.countSyllables(word, analyzer)
		totalSyllables += syllables
		if syllables >= 3 {
			complexWords++
		}
	}
	avgSyllablesPerWord := float64(totalSyllables) / float64(len(words))

	// Calculate lexical diversity
	uniqueWords := make(map[string]bool)
	wordFreq := make(map[string]int)
	for _, word := range words {
		cleanWord := strings.ToLower(strings.Trim(word, ".,!?;:"))
		uniqueWords[cleanWord] = true
		wordFreq[cleanWord]++
	}

	typeTokenRatio := float64(len(uniqueWords)) / float64(len(words))

	// Count hapax legomena (words that appear only once)
	hapaxCount := 0
	for _, freq := range wordFreq {
		if freq == 1 {
			hapaxCount++
		}
	}

	// Calculate complexity percentage
	complexWordPercentage := float64(complexWords) / float64(len(words)) * 100

	// Calculate formality and abstractness (simplified)
	formalityScore := l.calculateFormality(text, analyzer)
	abstractnessScore := l.calculateAbstractness(text, analyzer)

	return LinguisticFeatures{
		AverageSentenceLength:   avgSentenceLength,
		AverageWordsPerSentence: avgWordsPerSentence,
		AverageSyllablesPerWord: avgSyllablesPerWord,
		LexicalDiversity:        typeTokenRatio,
		TypeTokenRatio:          typeTokenRatio,
		HapaxLegomena:           hapaxCount,
		ComplexWordPercentage:   complexWordPercentage,
		FormalityScore:          formalityScore,
		AbstractnessScore:       abstractnessScore,
	}
}

// analyzeVocabulary analyzes vocabulary characteristics
func (l *LanguageAnalyzer) analyzeVocabulary(text string, analyzer *LinguisticAnalyzer) VocabularyStatistics {
	words := strings.Fields(text)

	wordFreq := make(map[string]int)
	wordLengthDist := make(map[int]int)

	for _, word := range words {
		cleanWord := strings.ToLower(strings.Trim(word, ".,!?;:\"'()[]{}"))
		if len(cleanWord) > 0 {
			wordFreq[cleanWord]++
			wordLengthDist[len(cleanWord)]++
		}
	}

	// Find common and rare words
	var commonWords, rareWords, longWords []string

	for word, freq := range wordFreq {
		if freq > 3 && !analyzer.stopWords[word] {
			commonWords = append(commonWords, word)
		}
		if freq == 1 && len(word) > 6 {
			rareWords = append(rareWords, word)
		}
		if len(word) > 8 {
			longWords = append(longWords, word)
		}
	}

	// Limit arrays to reasonable sizes
	if len(commonWords) > 20 {
		commonWords = commonWords[:20]
	}
	if len(rareWords) > 20 {
		rareWords = rareWords[:20]
	}
	if len(longWords) > 15 {
		longWords = longWords[:15]
	}

	return VocabularyStatistics{
		TotalWords:        len(words),
		UniqueWords:       len(wordFreq),
		VocabularySize:    len(wordFreq),
		CommonWords:       commonWords,
		RareWords:         rareWords,
		LongWords:         longWords,
		WordLengthDistrib: wordLengthDist,
	}
}

// analyzeSyntax analyzes syntactic features
func (l *LanguageAnalyzer) analyzeSyntax(text string, analyzer *LinguisticAnalyzer) SyntaxAnalysis {
	sentences := l.splitIntoSentences(text)

	sentenceTypes := map[string]int{
		"declarative":   0,
		"interrogative": 0,
		"exclamatory":   0,
	}

	questionCount := 0
	exclamationCount := 0
	passiveVoiceCount := 0

	for _, sentence := range sentences {
		trimmed := strings.TrimSpace(sentence)
		if len(trimmed) == 0 {
			continue
		}

		if strings.HasSuffix(trimmed, "?") {
			sentenceTypes["interrogative"]++
			questionCount++
		} else if strings.HasSuffix(trimmed, "!") {
			sentenceTypes["exclamatory"]++
			exclamationCount++
		} else {
			sentenceTypes["declarative"]++
		}

		// Simple passive voice detection
		if l.containsPassiveVoice(trimmed) {
			passiveVoiceCount++
		}
	}

	// Calculate punctuation density
	punctuationCount := 0
	for _, r := range text {
		if unicode.IsPunct(r) {
			punctuationCount++
		}
	}
	punctuationDensity := float64(punctuationCount) / float64(len(text))

	// Analyze capitalization pattern
	capitalizationPattern := l.analyzeCapitalization(text)

	// Calculate conjunction density
	conjunctionDensity := l.calculateConjunctionDensity(text, analyzer)

	return SyntaxAnalysis{
		SentenceTypes:         sentenceTypes,
		PunctuationDensity:    punctuationDensity,
		CapitalizationPattern: capitalizationPattern,
		QuestionCount:         questionCount,
		ExclamationCount:      exclamationCount,
		PassiveVoiceCount:     passiveVoiceCount,
		ConjunctionDensity:    conjunctionDensity,
	}
}

// analyzeSemanticFeatures analyzes semantic characteristics
func (l *LanguageAnalyzer) analyzeSemanticFeatures(text string, analyzer *LinguisticAnalyzer) SemanticFeatures {
	// Simplified semantic analysis
	words := strings.Fields(text)

	// Topic coherence (simplified measure based on word repetition)
	wordFreq := make(map[string]int)
	for _, word := range words {
		cleanWord := strings.ToLower(strings.Trim(word, ".,!?;:"))
		if !analyzer.stopWords[cleanWord] && len(cleanWord) > 3 {
			wordFreq[cleanWord]++
		}
	}

	repeatedWords := 0
	for _, freq := range wordFreq {
		if freq > 1 {
			repeatedWords++
		}
	}
	topicCoherence := float64(repeatedWords) / float64(len(wordFreq))

	// Semantic density (ratio of content words to total words)
	contentWords := 0
	for _, word := range words {
		cleanWord := strings.ToLower(strings.Trim(word, ".,!?;:"))
		if !analyzer.stopWords[cleanWord] && len(cleanWord) > 2 {
			contentWords++
		}
	}
	semanticDensity := float64(contentWords) / float64(len(words))

	// Conceptual complexity (based on abstract word usage)
	abstractWords := l.identifyAbstractConcepts(text, analyzer)
	technicalTerms := l.identifyTechnicalTerms(text, analyzer)
	conceptualComplexity := (float64(len(abstractWords)) + float64(len(technicalTerms))) / float64(len(words)) * 100

	// Emotional tone (simplified)
	emotionalTone := l.determineEmotionalTone(text, analyzer)

	// Subjectivity (simplified measure)
	subjectivity := l.calculateSubjectivity(text, analyzer)

	return SemanticFeatures{
		TopicCoherence:       topicCoherence,
		SemanticDensity:      semanticDensity,
		ConceptualComplexity: conceptualComplexity,
		AbstractConcepts:     abstractWords,
		TechnicalTerms:       technicalTerms,
		EmotionalTone:        emotionalTone,
		Subjectivity:         subjectivity,
	}
}

// Helper methods for calculations

func (l *LanguageAnalyzer) splitIntoSentences(text string) []string {
	sentences := regexp.MustCompile(`[.!?]+\s+`).Split(text, -1)
	var result []string
	for _, s := range sentences {
		if trimmed := strings.TrimSpace(s); len(trimmed) > 0 {
			result = append(result, trimmed)
		}
	}
	return result
}

func (l *LanguageAnalyzer) countSyllables(word string, analyzer *LinguisticAnalyzer) int {
	word = strings.ToLower(word)

	// Simple syllable counting for English
	if analyzer.language == LanguageEnglish {
		vowelGroups := regexp.MustCompile(`[aeiouy]+`).FindAllString(word, -1)
		syllables := len(vowelGroups)

		// Adjust for silent e
		if strings.HasSuffix(word, "e") && syllables > 1 {
			syllables--
		}

		if syllables == 0 {
			syllables = 1
		}

		return syllables
	}

	// Default simple counting
	return max(1, len(regexp.MustCompile(`[aeiouAEIOU]`).FindAllString(word, -1)))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (l *LanguageAnalyzer) calculateFleschKincaid(text string, analyzer *LinguisticAnalyzer) float64 {
	sentences := l.splitIntoSentences(text)
	words := strings.Fields(text)

	if len(sentences) == 0 || len(words) == 0 {
		return 0
	}

	totalSyllables := 0
	for _, word := range words {
		totalSyllables += l.countSyllables(word, analyzer)
	}

	avgSentenceLength := float64(len(words)) / float64(len(sentences))
	avgSyllablesPerWord := float64(totalSyllables) / float64(len(words))

	return 0.39*avgSentenceLength + 11.8*avgSyllablesPerWord - 15.59
}

func (l *LanguageAnalyzer) calculateFleschReadingEase(text string, analyzer *LinguisticAnalyzer) float64 {
	sentences := l.splitIntoSentences(text)
	words := strings.Fields(text)

	if len(sentences) == 0 || len(words) == 0 {
		return 0
	}

	totalSyllables := 0
	for _, word := range words {
		totalSyllables += l.countSyllables(word, analyzer)
	}

	avgSentenceLength := float64(len(words)) / float64(len(sentences))
	avgSyllablesPerWord := float64(totalSyllables) / float64(len(words))

	return 206.835 - 1.015*avgSentenceLength - 84.6*avgSyllablesPerWord
}

func (l *LanguageAnalyzer) calculateGunningFog(text string, analyzer *LinguisticAnalyzer) float64 {
	sentences := l.splitIntoSentences(text)
	words := strings.Fields(text)

	if len(sentences) == 0 || len(words) == 0 {
		return 0
	}

	complexWords := 0
	for _, word := range words {
		if l.countSyllables(word, analyzer) >= 3 {
			complexWords++
		}
	}

	avgSentenceLength := float64(len(words)) / float64(len(sentences))
	complexWordRatio := float64(complexWords) / float64(len(words)) * 100

	return 0.4 * (avgSentenceLength + complexWordRatio)
}

func (l *LanguageAnalyzer) calculateSMOG(text string, analyzer *LinguisticAnalyzer) float64 {
	sentences := l.splitIntoSentences(text)
	words := strings.Fields(text)

	if len(sentences) < 30 || len(words) == 0 {
		return 0
	}

	complexWords := 0
	for _, word := range words {
		if l.countSyllables(word, analyzer) >= 3 {
			complexWords++
		}
	}

	return 1.043*math.Sqrt(float64(complexWords)*30.0/float64(len(sentences))) + 3.1291
}

func (l *LanguageAnalyzer) calculateColemanLiau(text string) float64 {
	sentences := l.splitIntoSentences(text)
	words := strings.Fields(text)

	if len(sentences) == 0 || len(words) == 0 {
		return 0
	}

	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}

	L := float64(letters) / float64(len(words)) * 100
	S := float64(len(sentences)) / float64(len(words)) * 100

	return 0.0588*L - 0.296*S - 15.8
}

func (l *LanguageAnalyzer) calculateARI(text string) float64 {
	sentences := l.splitIntoSentences(text)
	words := strings.Fields(text)

	if len(sentences) == 0 || len(words) == 0 {
		return 0
	}

	characters := len(strings.ReplaceAll(text, " ", ""))

	return 4.71*float64(characters)/float64(len(words)) + 0.5*float64(len(words))/float64(len(sentences)) - 21.43
}

func (l *LanguageAnalyzer) calculateFormality(text string, analyzer *LinguisticAnalyzer) float64 {
	// Simplified formality calculation based on word choice
	words := strings.Fields(text)
	if len(words) == 0 {
		return 0.5
	}

	formalWords := 0
	informalWords := 0

	formalIndicators := []string{"however", "therefore", "furthermore", "consequently", "nevertheless"}
	informalIndicators := []string{"yeah", "ok", "cool", "awesome", "stuff"}

	lowerText := strings.ToLower(text)
	for _, formal := range formalIndicators {
		if strings.Contains(lowerText, formal) {
			formalWords++
		}
	}

	for _, informal := range informalIndicators {
		if strings.Contains(lowerText, informal) {
			informalWords++
		}
	}

	if formalWords+informalWords == 0 {
		return 0.5
	}

	return float64(formalWords) / float64(formalWords+informalWords)
}

func (l *LanguageAnalyzer) calculateAbstractness(text string, analyzer *LinguisticAnalyzer) float64 {
	words := strings.Fields(text)
	if len(words) == 0 {
		return 0
	}

	abstractWords := l.identifyAbstractConcepts(text, analyzer)
	return float64(len(abstractWords)) / float64(len(words))
}

func (l *LanguageAnalyzer) containsPassiveVoice(sentence string) bool {
	// Simple passive voice detection
	passivePatterns := []string{
		"was.*ed", "were.*ed", "is.*ed", "are.*ed", "been.*ed", "being.*ed",
	}

	lowerSentence := strings.ToLower(sentence)
	for _, pattern := range passivePatterns {
		if matched, _ := regexp.MatchString(pattern, lowerSentence); matched {
			return true
		}
	}

	return false
}

func (l *LanguageAnalyzer) analyzeCapitalization(text string) string {
	hasUpper := false
	hasLower := false

	for _, r := range text {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
	}

	if hasUpper && hasLower {
		return "mixed_case"
	} else if hasUpper {
		return "uppercase"
	} else if hasLower {
		return "lowercase"
	}
	return "no_letters"
}

func (l *LanguageAnalyzer) calculateConjunctionDensity(text string, analyzer *LinguisticAnalyzer) float64 {
	words := strings.Fields(text)
	if len(words) == 0 {
		return 0
	}

	conjunctions := []string{"and", "or", "but", "so", "yet", "for", "nor", "because", "since", "although"}
	conjunctionCount := 0

	lowerText := strings.ToLower(text)
	for _, conj := range conjunctions {
		conjunctionCount += strings.Count(lowerText, " "+conj+" ")
	}

	return float64(conjunctionCount) / float64(len(words))
}

func (l *LanguageAnalyzer) identifyAbstractConcepts(text string, analyzer *LinguisticAnalyzer) []string {
	// Simplified abstract concept identification
	abstractWords := []string{}
	abstractPatterns := []string{
		"freedom", "justice", "beauty", "truth", "love", "hate", "fear",
		"happiness", "sadness", "anger", "joy", "hope", "despair",
		"concept", "idea", "theory", "principle", "belief", "value",
	}

	lowerText := strings.ToLower(text)
	for _, word := range abstractPatterns {
		if strings.Contains(lowerText, word) {
			abstractWords = append(abstractWords, word)
		}
	}

	return abstractWords
}

func (l *LanguageAnalyzer) identifyTechnicalTerms(text string, analyzer *LinguisticAnalyzer) []string {
	// Simplified technical term identification
	technicalTerms := []string{}
	words := strings.Fields(text)

	for _, word := range words {
		cleanWord := strings.ToLower(strings.Trim(word, ".,!?;:"))
		// Consider words with specific patterns as technical
		if len(cleanWord) > 8 || strings.Contains(cleanWord, "_") ||
			regexp.MustCompile(`[A-Z]{2,}`).MatchString(word) {
			technicalTerms = append(technicalTerms, cleanWord)
		}
	}

	// Limit to reasonable size
	if len(technicalTerms) > 10 {
		technicalTerms = technicalTerms[:10]
	}

	return technicalTerms
}

func (l *LanguageAnalyzer) determineEmotionalTone(text string, analyzer *LinguisticAnalyzer) string {
	// Simplified emotional tone detection
	positiveWords := []string{"good", "great", "excellent", "amazing", "wonderful", "happy", "joy"}
	negativeWords := []string{"bad", "terrible", "awful", "sad", "angry", "hate", "fear"}

	lowerText := strings.ToLower(text)
	positiveCount := 0
	negativeCount := 0

	for _, word := range positiveWords {
		positiveCount += strings.Count(lowerText, word)
	}

	for _, word := range negativeWords {
		negativeCount += strings.Count(lowerText, word)
	}

	if positiveCount > negativeCount {
		return "positive"
	} else if negativeCount > positiveCount {
		return "negative"
	}
	return "neutral"
}

func (l *LanguageAnalyzer) calculateSubjectivity(text string, analyzer *LinguisticAnalyzer) float64 {
	// Simplified subjectivity calculation
	subjectiveWords := []string{"think", "believe", "feel", "opinion", "personally", "maybe", "probably"}
	objectiveWords := []string{"fact", "data", "evidence", "research", "study", "analysis"}

	lowerText := strings.ToLower(text)
	subjectiveCount := 0
	objectiveCount := 0

	for _, word := range subjectiveWords {
		subjectiveCount += strings.Count(lowerText, word)
	}

	for _, word := range objectiveWords {
		objectiveCount += strings.Count(lowerText, word)
	}

	total := subjectiveCount + objectiveCount
	if total == 0 {
		return 0.5
	}

	return float64(subjectiveCount) / float64(total)
}

// initializeLinguisticAnalyzers initializes language-specific analyzers
func (l *LanguageAnalyzer) initializeLinguisticAnalyzers() {
	// English analyzer
	l.linguisticAnalyzers[LanguageEnglish] = &LinguisticAnalyzer{
		language:    LanguageEnglish,
		stopWords:   l.getEnglishStopWords(),
		commonWords: l.getEnglishCommonWords(),
	}

	// Add more language analyzers as needed
	l.linguisticAnalyzers[LanguageSpanish] = &LinguisticAnalyzer{
		language:    LanguageSpanish,
		stopWords:   l.getSpanishStopWords(),
		commonWords: l.getSpanishCommonWords(),
	}

	l.linguisticAnalyzers[LanguageFrench] = &LinguisticAnalyzer{
		language:    LanguageFrench,
		stopWords:   l.getFrenchStopWords(),
		commonWords: l.getFrenchCommonWords(),
	}
}

func (l *LanguageAnalyzer) getEnglishStopWords() map[string]bool {
	stopWords := map[string]bool{}
	words := []string{
		"the", "and", "to", "of", "a", "in", "is", "it", "you", "that",
		"he", "was", "for", "on", "are", "as", "with", "his", "they", "at",
		"be", "this", "have", "from", "or", "one", "had", "by", "words", "but",
		"not", "what", "all", "were", "they", "we", "when", "your", "can", "said",
	}
	for _, word := range words {
		stopWords[word] = true
	}
	return stopWords
}

func (l *LanguageAnalyzer) getEnglishCommonWords() map[string]bool {
	commonWords := map[string]bool{}
	words := []string{
		"time", "person", "year", "way", "day", "thing", "man", "world", "life", "hand",
		"part", "child", "eye", "woman", "place", "work", "week", "case", "point", "government",
	}
	for _, word := range words {
		commonWords[word] = true
	}
	return commonWords
}

func (l *LanguageAnalyzer) getSpanishStopWords() map[string]bool {
	stopWords := map[string]bool{}
	words := []string{
		"el", "la", "de", "que", "y", "a", "en", "un", "es", "se",
		"no", "te", "lo", "le", "da", "su", "por", "son", "con", "para",
	}
	for _, word := range words {
		stopWords[word] = true
	}
	return stopWords
}

func (l *LanguageAnalyzer) getSpanishCommonWords() map[string]bool {
	commonWords := map[string]bool{}
	words := []string{
		"tiempo", "persona", "año", "forma", "día", "cosa", "hombre", "mundo", "vida", "mano",
	}
	for _, word := range words {
		commonWords[word] = true
	}
	return commonWords
}

func (l *LanguageAnalyzer) getFrenchStopWords() map[string]bool {
	stopWords := map[string]bool{}
	words := []string{
		"le", "de", "et", "à", "un", "il", "être", "et", "en", "avoir",
		"que", "pour", "dans", "ce", "son", "une", "sur", "avec", "ne", "se",
	}
	for _, word := range words {
		stopWords[word] = true
	}
	return stopWords
}

func (l *LanguageAnalyzer) getFrenchCommonWords() map[string]bool {
	commonWords := map[string]bool{}
	words := []string{
		"temps", "personne", "année", "façon", "jour", "chose", "homme", "monde", "vie", "main",
	}
	for _, word := range words {
		commonWords[word] = true
	}
	return commonWords
}
