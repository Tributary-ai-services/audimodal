package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

// MLAnalysisResult represents the result of ML-based content analysis
type MLAnalysisResult struct {
	DocumentID          string                 `json:"document_id"`
	ChunkID             string                 `json:"chunk_id,omitempty"`
	Sentiment           SentimentResult        `json:"sentiment"`
	Topics              []TopicResult          `json:"topics"`
	Entities            []EntityResult         `json:"entities"`
	Summary             SummaryResult          `json:"summary"`
	QualityMetrics      QualityMetrics         `json:"quality_metrics"`
	ContentType         ContentTypeResult      `json:"content_type"`
	KeyPhrases          []KeyPhraseResult      `json:"key_phrases"`
	ReadabilityScore    ReadabilityScore       `json:"readability"`
	LanguageDetection   LanguageResult         `json:"language"`
	EmotionalTone       EmotionalToneResult    `json:"emotional_tone"`
	ProcessingTime      int64                  `json:"processing_time_ms"`
	ModelVersions       map[string]string      `json:"model_versions"`
	Confidence          float64                `json:"overall_confidence"`
}

// SentimentResult represents sentiment analysis results
type SentimentResult struct {
	Label      string  `json:"label"`      // positive, negative, neutral
	Score      float64 `json:"score"`      // confidence score 0-1
	Magnitude  float64 `json:"magnitude"`  // strength of emotion 0-1
	Subjectivity float64 `json:"subjectivity"` // objective vs subjective 0-1
}

// TopicResult represents topic modeling results
type TopicResult struct {
	Topic       string   `json:"topic"`
	Keywords    []string `json:"keywords"`
	Probability float64  `json:"probability"`
	Coherence   float64  `json:"coherence"`
}

// EntityResult represents named entity recognition results
type EntityResult struct {
	Text       string  `json:"text"`
	Label      string  `json:"label"`      // PERSON, ORG, LOC, MISC, etc.
	StartPos   int     `json:"start_pos"`
	EndPos     int     `json:"end_pos"`
	Confidence float64 `json:"confidence"`
	Context    string  `json:"context"`    // surrounding text
}

// SummaryResult represents document summarization results
type SummaryResult struct {
	ExtractiveSummary   string   `json:"extractive_summary"`
	AbstractiveSummary  string   `json:"abstractive_summary"`
	KeySentences        []string `json:"key_sentences"`
	SummaryRatio        float64  `json:"summary_ratio"`
	CompressionScore    float64  `json:"compression_score"`
}

// QualityMetrics represents content quality assessment
type QualityMetrics struct {
	Coherence           float64  `json:"coherence"`
	Clarity             float64  `json:"clarity"`
	Completeness        float64  `json:"completeness"`
	Informativeness     float64  `json:"informativeness"`
	Redundancy          float64  `json:"redundancy"`
	StructuralQuality   float64  `json:"structural_quality"`
	Issues              []string `json:"issues"`
	OverallScore        float64  `json:"overall_score"`
}

// ContentTypeResult represents advanced content classification
type ContentTypeResult struct {
	PrimaryType    string             `json:"primary_type"`
	SecondaryTypes []string           `json:"secondary_types"`
	Confidence     float64            `json:"confidence"`
	Features       map[string]float64 `json:"features"`
	Genre          string             `json:"genre"`
	FormattingType string             `json:"formatting_type"`
}

// KeyPhraseResult represents key phrase extraction
type KeyPhraseResult struct {
	Phrase     string  `json:"phrase"`
	Score      float64 `json:"score"`
	Frequency  int     `json:"frequency"`
	Position   int     `json:"position"`
	IsNoun     bool    `json:"is_noun"`
	IsAcronym  bool    `json:"is_acronym"`
}

// ReadabilityScore represents text readability metrics
type ReadabilityScore struct {
	FleschKincaid    float64 `json:"flesch_kincaid"`
	FleschReading    float64 `json:"flesch_reading"`
	GunningFog       float64 `json:"gunning_fog"`
	SMOG             float64 `json:"smog"`
	ColemanLiau      float64 `json:"coleman_liau"`
	GradeLevel       float64 `json:"grade_level"`
	ReadingTime      int     `json:"reading_time_seconds"`
}

// LanguageResult represents language detection with dialects
type LanguageResult struct {
	Language    string             `json:"language"`
	Dialect     string             `json:"dialect"`
	Confidence  float64            `json:"confidence"`
	Alternatives []LanguageAlternative `json:"alternatives"`
	Script      string             `json:"script"`
}

// LanguageAlternative represents alternative language predictions
type LanguageAlternative struct {
	Language   string  `json:"language"`
	Confidence float64 `json:"confidence"`
}

// EmotionalToneResult represents emotional tone analysis
type EmotionalToneResult struct {
	Joy        float64 `json:"joy"`
	Anger      float64 `json:"anger"`
	Fear       float64 `json:"fear"`
	Sadness    float64 `json:"sadness"`
	Analytical float64 `json:"analytical"`
	Confident  float64 `json:"confident"`
	Tentative  float64 `json:"tentative"`
	Openness   float64 `json:"openness"`
}

// MLAnalyzer performs advanced ML-based content analysis
type MLAnalyzer struct {
	config            *MLAnalysisConfig
	sentimentModel    SentimentModel
	topicModel        TopicModel
	entityModel       EntityModel
	summaryModel      SummaryModel
	qualityModel      QualityModel
	languageModel     LanguageModel
	emotionModel      EmotionModel
}

// MLAnalysisConfig configures the ML analysis pipeline
type MLAnalysisConfig struct {
	EnableSentiment      bool   `json:"enable_sentiment"`
	EnableTopics         bool   `json:"enable_topics"`
	EnableEntities       bool   `json:"enable_entities"`
	EnableSummary        bool   `json:"enable_summary"`
	EnableQuality        bool   `json:"enable_quality"`
	EnableLanguage       bool   `json:"enable_language"`
	EnableEmotion        bool   `json:"enable_emotion"`
	MaxTopics            int    `json:"max_topics"`
	SummaryRatio         float64 `json:"summary_ratio"`
	MinConfidence        float64 `json:"min_confidence"`
	ModelTimeout         time.Duration `json:"model_timeout"`
	CacheEnabled         bool   `json:"cache_enabled"`
	CacheTTL             time.Duration `json:"cache_ttl"`
	ParallelProcessing   bool   `json:"parallel_processing"`
	BatchSize            int    `json:"batch_size"`
}

// SentimentModel interface for sentiment analysis
type SentimentModel interface {
	AnalyzeSentiment(ctx context.Context, text string) (SentimentResult, error)
	GetModelVersion() string
}

// TopicModel interface for topic modeling
type TopicModel interface {
	ExtractTopics(ctx context.Context, text string, maxTopics int) ([]TopicResult, error)
	GetModelVersion() string
}

// EntityModel interface for named entity recognition
type EntityModel interface {
	ExtractEntities(ctx context.Context, text string) ([]EntityResult, error)
	GetModelVersion() string
}

// SummaryModel interface for text summarization
type SummaryModel interface {
	GenerateSummary(ctx context.Context, text string, ratio float64) (SummaryResult, error)
	GetModelVersion() string
}

// QualityModel interface for content quality assessment
type QualityModel interface {
	AssessQuality(ctx context.Context, text string) (QualityMetrics, error)
	GetModelVersion() string
}

// LanguageModel interface for language detection
type LanguageModel interface {
	DetectLanguage(ctx context.Context, text string) (LanguageResult, error)
	GetModelVersion() string
}

// EmotionModel interface for emotional tone analysis
type EmotionModel interface {
	AnalyzeEmotion(ctx context.Context, text string) (EmotionalToneResult, error)
	GetModelVersion() string
}

// NewMLAnalyzer creates a new ML analyzer with the specified configuration
func NewMLAnalyzer(config *MLAnalysisConfig) *MLAnalyzer {
	if config == nil {
		config = DefaultMLAnalysisConfig()
	}

	analyzer := &MLAnalyzer{
		config: config,
	}

	// Initialize models based on configuration
	if config.EnableSentiment {
		analyzer.sentimentModel = NewHeuristicSentimentModel()
	}
	if config.EnableTopics {
		analyzer.topicModel = NewHeuristicTopicModel()
	}
	if config.EnableEntities {
		analyzer.entityModel = NewHeuristicEntityModel()
	}
	if config.EnableSummary {
		analyzer.summaryModel = NewHeuristicSummaryModel()
	}
	if config.EnableQuality {
		analyzer.qualityModel = NewHeuristicQualityModel()
	}
	if config.EnableLanguage {
		analyzer.languageModel = NewHeuristicLanguageModel()
	}
	if config.EnableEmotion {
		analyzer.emotionModel = NewHeuristicEmotionModel()
	}

	return analyzer
}

// DefaultMLAnalysisConfig returns default configuration
func DefaultMLAnalysisConfig() *MLAnalysisConfig {
	return &MLAnalysisConfig{
		EnableSentiment:    true,
		EnableTopics:       true,
		EnableEntities:     true,
		EnableSummary:      true,
		EnableQuality:      true,
		EnableLanguage:     true,
		EnableEmotion:      true,
		MaxTopics:          5,
		SummaryRatio:       0.3,
		MinConfidence:      0.5,
		ModelTimeout:       30 * time.Second,
		CacheEnabled:       true,
		CacheTTL:           1 * time.Hour,
		ParallelProcessing: true,
		BatchSize:          10,
	}
}

// AnalyzeContent performs comprehensive ML analysis on the given text
func (a *MLAnalyzer) AnalyzeContent(ctx context.Context, documentID, chunkID, text string) (*MLAnalysisResult, error) {
	startTime := time.Now()

	result := &MLAnalysisResult{
		DocumentID:    documentID,
		ChunkID:       chunkID,
		ModelVersions: make(map[string]string),
	}

	// Create a channel for collecting analysis results
	type analysisTask struct {
		name string
		fn   func() error
	}

	tasks := []analysisTask{}

	// Add enabled analysis tasks
	if a.config.EnableSentiment && a.sentimentModel != nil {
		tasks = append(tasks, analysisTask{
			name: "sentiment",
			fn: func() error {
				sentiment, err := a.sentimentModel.AnalyzeSentiment(ctx, text)
				if err != nil {
					return fmt.Errorf("sentiment analysis failed: %w", err)
				}
				result.Sentiment = sentiment
				result.ModelVersions["sentiment"] = a.sentimentModel.GetModelVersion()
				return nil
			},
		})
	}

	if a.config.EnableTopics && a.topicModel != nil {
		tasks = append(tasks, analysisTask{
			name: "topics",
			fn: func() error {
				topics, err := a.topicModel.ExtractTopics(ctx, text, a.config.MaxTopics)
				if err != nil {
					return fmt.Errorf("topic modeling failed: %w", err)
				}
				result.Topics = topics
				result.ModelVersions["topics"] = a.topicModel.GetModelVersion()
				return nil
			},
		})
	}

	if a.config.EnableEntities && a.entityModel != nil {
		tasks = append(tasks, analysisTask{
			name: "entities",
			fn: func() error {
				entities, err := a.entityModel.ExtractEntities(ctx, text)
				if err != nil {
					return fmt.Errorf("entity extraction failed: %w", err)
				}
				result.Entities = entities
				result.ModelVersions["entities"] = a.entityModel.GetModelVersion()
				return nil
			},
		})
	}

	if a.config.EnableSummary && a.summaryModel != nil {
		tasks = append(tasks, analysisTask{
			name: "summary",
			fn: func() error {
				summary, err := a.summaryModel.GenerateSummary(ctx, text, a.config.SummaryRatio)
				if err != nil {
					return fmt.Errorf("summarization failed: %w", err)
				}
				result.Summary = summary
				result.ModelVersions["summary"] = a.summaryModel.GetModelVersion()
				return nil
			},
		})
	}

	if a.config.EnableQuality && a.qualityModel != nil {
		tasks = append(tasks, analysisTask{
			name: "quality",
			fn: func() error {
				quality, err := a.qualityModel.AssessQuality(ctx, text)
				if err != nil {
					return fmt.Errorf("quality assessment failed: %w", err)
				}
				result.QualityMetrics = quality
				result.ModelVersions["quality"] = a.qualityModel.GetModelVersion()
				return nil
			},
		})
	}

	if a.config.EnableLanguage && a.languageModel != nil {
		tasks = append(tasks, analysisTask{
			name: "language",
			fn: func() error {
				language, err := a.languageModel.DetectLanguage(ctx, text)
				if err != nil {
					return fmt.Errorf("language detection failed: %w", err)
				}
				result.LanguageDetection = language
				result.ModelVersions["language"] = a.languageModel.GetModelVersion()
				return nil
			},
		})
	}

	if a.config.EnableEmotion && a.emotionModel != nil {
		tasks = append(tasks, analysisTask{
			name: "emotion",
			fn: func() error {
				emotion, err := a.emotionModel.AnalyzeEmotion(ctx, text)
				if err != nil {
					return fmt.Errorf("emotion analysis failed: %w", err)
				}
				result.EmotionalTone = emotion
				result.ModelVersions["emotion"] = a.emotionModel.GetModelVersion()
				return nil
			},
		})
	}

	// Execute tasks based on configuration
	if a.config.ParallelProcessing {
		// Execute tasks in parallel
		errorChan := make(chan error, len(tasks))
		for _, task := range tasks {
			go func(t analysisTask) {
				errorChan <- t.fn()
			}(task)
		}

		// Collect results
		var errors []error
		for range tasks {
			if err := <-errorChan; err != nil {
				errors = append(errors, err)
			}
		}

		if len(errors) > 0 {
			return nil, fmt.Errorf("analysis failed with %d errors: %v", len(errors), errors[0])
		}
	} else {
		// Execute tasks sequentially
		for _, task := range tasks {
			if err := task.fn(); err != nil {
				return nil, fmt.Errorf("task %s failed: %w", task.name, err)
			}
		}
	}

	// Always compute basic metrics
	result.KeyPhrases = a.extractKeyPhrases(text)
	result.ReadabilityScore = a.computeReadabilityScore(text)
	result.ContentType = a.classifyContentType(text)

	// Compute overall confidence
	result.Confidence = a.computeOverallConfidence(result)

	// Record processing time
	result.ProcessingTime = time.Since(startTime).Milliseconds()

	return result, nil
}

// extractKeyPhrases extracts key phrases using statistical methods
func (a *MLAnalyzer) extractKeyPhrases(text string) []KeyPhraseResult {
	// Simple implementation using TF-IDF-like scoring
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return []KeyPhraseResult{}
	}

	// Remove common stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
		"this": true, "that": true, "these": true, "those": true, "i": true, "you": true,
		"he": true, "she": true, "it": true, "we": true, "they": true, "them": true,
	}

	// Count word frequencies
	wordFreq := make(map[string]int)
	for _, word := range words {
		cleaned := regexp.MustCompile(`[^\w]`).ReplaceAllString(word, "")
		if len(cleaned) > 2 && !stopWords[cleaned] {
			wordFreq[cleaned]++
		}
	}

	// Extract n-grams (bigrams and trigrams)
	phrases := make(map[string]int)
	for phrase := range wordFreq {
		phrases[phrase] = wordFreq[phrase]
	}

	// Add bigrams
	for i := 0; i < len(words)-1; i++ {
		word1 := regexp.MustCompile(`[^\w]`).ReplaceAllString(words[i], "")
		word2 := regexp.MustCompile(`[^\w]`).ReplaceAllString(words[i+1], "")
		if len(word1) > 2 && len(word2) > 2 && !stopWords[word1] && !stopWords[word2] {
			bigram := word1 + " " + word2
			phrases[bigram]++
		}
	}

	// Convert to results and sort by score
	var results []KeyPhraseResult
	for phrase, freq := range phrases {
		if freq < 2 {
			continue // Filter low-frequency phrases
		}

		score := float64(freq) * math.Log(float64(len(words))/float64(freq))
		results = append(results, KeyPhraseResult{
			Phrase:    phrase,
			Score:     score,
			Frequency: freq,
			Position:  strings.Index(text, phrase),
			IsNoun:    a.isLikelyNoun(phrase),
			IsAcronym: a.isAcronym(phrase),
		})
	}

	// Sort by score and return top results
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > 20 {
		results = results[:20]
	}

	return results
}

// computeReadabilityScore computes various readability metrics
func (a *MLAnalyzer) computeReadabilityScore(text string) ReadabilityScore {
	sentences := a.countSentences(text)
	words := a.countWords(text)
	syllables := a.countSyllables(text)
	
	if sentences == 0 || words == 0 {
		return ReadabilityScore{}
	}

	avgWordsPerSentence := float64(words) / float64(sentences)
	avgSyllablesPerWord := float64(syllables) / float64(words)

	// Flesch-Kincaid Grade Level
	fleschKincaid := 0.39*avgWordsPerSentence + 11.8*avgSyllablesPerWord - 15.59

	// Flesch Reading Ease
	fleschReading := 206.835 - 1.015*avgWordsPerSentence - 84.6*avgSyllablesPerWord

	// Gunning Fog Index
	complexWords := a.countComplexWords(text)
	gunningFog := 0.4 * (avgWordsPerSentence + 100*float64(complexWords)/float64(words))

	// SMOG Index
	smog := 1.043*math.Sqrt(float64(complexWords)*30/float64(sentences)) + 3.1291

	// Coleman-Liau Index
	avgLettersPerWord := float64(a.countLetters(text)) / float64(words)
	colemanLiau := 0.0588*avgLettersPerWord*100/avgWordsPerSentence - 0.296*float64(sentences)*100/float64(words) - 15.8

	// Estimated reading time (words per minute)
	wordsPerMinute := 200.0
	readingTime := int(float64(words) / wordsPerMinute * 60)

	return ReadabilityScore{
		FleschKincaid: math.Round(fleschKincaid*100) / 100,
		FleschReading: math.Round(fleschReading*100) / 100,
		GunningFog:    math.Round(gunningFog*100) / 100,
		SMOG:          math.Round(smog*100) / 100,
		ColemanLiau:   math.Round(colemanLiau*100) / 100,
		GradeLevel:    math.Round(fleschKincaid*100) / 100,
		ReadingTime:   readingTime,
	}
}

// classifyContentType performs advanced content type classification
func (a *MLAnalyzer) classifyContentType(text string) ContentTypeResult {
	features := make(map[string]float64)
	
	// Extract features
	features["avg_sentence_length"] = float64(a.countWords(text)) / float64(a.countSentences(text))
	features["question_ratio"] = float64(strings.Count(text, "?")) / float64(len(text))
	features["exclamation_ratio"] = float64(strings.Count(text, "!")) / float64(len(text))
	features["number_ratio"] = float64(len(regexp.MustCompile(`\d+`).FindAllString(text, -1))) / float64(a.countWords(text))
	features["capital_ratio"] = float64(len(regexp.MustCompile(`[A-Z]`).FindAllString(text, -1))) / float64(len(text))
	
	// Simple rule-based classification
	primaryType := "general"
	confidence := 0.7
	
	if features["question_ratio"] > 0.05 {
		primaryType = "faq"
		confidence = 0.8
	} else if features["number_ratio"] > 0.1 {
		primaryType = "technical"
		confidence = 0.75
	} else if features["exclamation_ratio"] > 0.02 {
		primaryType = "marketing"
		confidence = 0.7
	}

	return ContentTypeResult{
		PrimaryType:    primaryType,
		SecondaryTypes: []string{},
		Confidence:     confidence,
		Features:       features,
		Genre:          "informational",
		FormattingType: "plain_text",
	}
}

// computeOverallConfidence computes an overall confidence score
func (a *MLAnalyzer) computeOverallConfidence(result *MLAnalysisResult) float64 {
	confidences := []float64{}
	
	if result.Sentiment.Score > 0 {
		confidences = append(confidences, result.Sentiment.Score)
	}
	if result.ContentType.Confidence > 0 {
		confidences = append(confidences, result.ContentType.Confidence)
	}
	if result.LanguageDetection.Confidence > 0 {
		confidences = append(confidences, result.LanguageDetection.Confidence)
	}
	
	if len(confidences) == 0 {
		return 0.5 // Default confidence
	}
	
	sum := 0.0
	for _, conf := range confidences {
		sum += conf
	}
	
	return sum / float64(len(confidences))
}

// Helper functions for text analysis
func (a *MLAnalyzer) countSentences(text string) int {
	count := len(regexp.MustCompile(`[.!?]+`).FindAllString(text, -1))
	if count == 0 {
		return 1
	}
	return count
}

func (a *MLAnalyzer) countWords(text string) int {
	return len(strings.Fields(text))
}

func (a *MLAnalyzer) countLetters(text string) int {
	return len(regexp.MustCompile(`[a-zA-Z]`).FindAllString(text, -1))
}

func (a *MLAnalyzer) countSyllables(text string) int {
	words := strings.Fields(strings.ToLower(text))
	syllables := 0
	
	for _, word := range words {
		syllables += a.countSyllablesInWord(word)
	}
	
	return syllables
}

func (a *MLAnalyzer) countSyllablesInWord(word string) int {
	word = regexp.MustCompile(`[^a-z]`).ReplaceAllString(word, "")
	if len(word) == 0 {
		return 0
	}
	
	vowels := regexp.MustCompile(`[aeiouy]`)
	matches := vowels.FindAllString(word, -1)
	syllables := len(matches)
	
	// Adjust for silent e
	if strings.HasSuffix(word, "e") {
		syllables--
	}
	
	if syllables == 0 {
		syllables = 1
	}
	
	return syllables
}

func (a *MLAnalyzer) countComplexWords(text string) int {
	words := strings.Fields(strings.ToLower(text))
	complexCount := 0
	
	for _, word := range words {
		if a.countSyllablesInWord(word) >= 3 {
			complexCount++
		}
	}
	
	return complexCount
}

func (a *MLAnalyzer) isLikelyNoun(phrase string) bool {
	// Simple heuristic: check if phrase starts with capital letter or contains common noun patterns
	return regexp.MustCompile(`^[A-Z]`).MatchString(phrase) || 
		   regexp.MustCompile(`(tion|sion|ness|ment|ity)$`).MatchString(phrase)
}

func (a *MLAnalyzer) isAcronym(phrase string) bool {
	return regexp.MustCompile(`^[A-Z]{2,}$`).MatchString(phrase)
}

// ToJSON converts the analysis result to JSON
func (r *MLAnalysisResult) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// FromJSON populates the result from JSON
func (r *MLAnalysisResult) FromJSON(data []byte) error {
	return json.Unmarshal(data, r)
}