package classification

import (
	"context"
	"math"
	"sort"
	"strings"
)

// BasicTopicModeler provides basic topic modeling using keyword clustering
type BasicTopicModeler struct {
	name         string
	version      string
	modelName    string
	minTopicSize int
	maxTopics    int
	topicSeeds   map[string][]string // Predefined topic seeds
}

// NewBasicTopicModeler creates a new basic topic modeler
func NewBasicTopicModeler() *BasicTopicModeler {
	modeler := &BasicTopicModeler{
		name:         "basic-topic-modeler",
		version:      "1.0.0",
		modelName:    "keyword-clustering",
		minTopicSize: 2,
		maxTopics:    20,
		topicSeeds:   make(map[string][]string),
	}
	
	modeler.initializeTopicSeeds()
	return modeler
}

// ExtractTopics extracts topics from the given text
func (t *BasicTopicModeler) ExtractTopics(ctx context.Context, text string, maxTopics int) ([]Topic, error) {
	if len(text) == 0 {
		return []Topic{}, nil
	}
	
	if maxTopics <= 0 {
		maxTopics = t.maxTopics
	}
	
	// Extract keywords first (reusing keyword extraction logic)
	keywordExtractor := NewBasicKeywordExtractor()
	keywords, err := keywordExtractor.ExtractKeywords(ctx, text, 50) // Get more keywords for topic modeling
	if err != nil {
		return []Topic{}, err
	}
	
	if len(keywords) < t.minTopicSize {
		return []Topic{}, nil
	}
	
	// Group keywords into topics
	topics := t.clusterKeywordsIntoTopics(keywords, text, maxTopics)
	
	// Calculate topic relevance and confidence
	for i := range topics {
		topics[i].Relevance = t.calculateTopicRelevance(topics[i], text)
		topics[i].Confidence = t.calculateTopicConfidence(topics[i], keywords)
	}
	
	// Filter and sort topics
	topics = t.filterTopics(topics)
	
	// Sort by relevance
	sort.Slice(topics, func(i, j int) bool {
		return topics[i].Relevance > topics[j].Relevance
	})
	
	// Limit to maxTopics
	if len(topics) > maxTopics {
		topics = topics[:maxTopics]
	}
	
	return topics, nil
}

// GetModelName returns the model name
func (t *BasicTopicModeler) GetModelName() string {
	return t.modelName
}

// clusterKeywordsIntoTopics clusters keywords into coherent topics
func (t *BasicTopicModeler) clusterKeywordsIntoTopics(keywords []Keyword, text string, maxTopics int) []Topic {
	topics := []Topic{}
	
	// First, try to match against predefined topic seeds
	seedTopics := t.matchTopicSeeds(keywords, text)
	topics = append(topics, seedTopics...)
	
	// Get remaining keywords not assigned to seed topics
	usedKeywords := make(map[string]bool)
	for _, topic := range topics {
		for _, keyword := range topic.Keywords {
			usedKeywords[keyword] = true
		}
	}
	
	remainingKeywords := []Keyword{}
	for _, keyword := range keywords {
		if !usedKeywords[keyword.Text] {
			remainingKeywords = append(remainingKeywords, keyword)
		}
	}
	
	// Create additional topics from remaining high-relevance keywords
	if len(remainingKeywords) >= t.minTopicSize {
		additionalTopics := t.createTopicsFromKeywords(remainingKeywords, text, maxTopics-len(topics))
		topics = append(topics, additionalTopics...)
	}
	
	return topics
}

// matchTopicSeeds matches keywords against predefined topic seeds
func (t *BasicTopicModeler) matchTopicSeeds(keywords []Keyword, text string) []Topic {
	topics := []Topic{}
	keywordMap := make(map[string]Keyword)
	
	// Create a map for quick keyword lookup
	for _, keyword := range keywords {
		keywordMap[keyword.Text] = keyword
	}
	
	for topicName, seedWords := range t.topicSeeds {
		matchedKeywords := []string{}
		totalRelevance := 0.0
		
		for _, seedWord := range seedWords {
			if keyword, exists := keywordMap[seedWord]; exists {
				matchedKeywords = append(matchedKeywords, seedWord)
				totalRelevance += keyword.Relevance
			}
		}
		
		// Only create topic if we have enough matches
		if len(matchedKeywords) >= t.minTopicSize {
			topic := Topic{
				Name:     topicName,
				Keywords: matchedKeywords,
				Metadata: map[string]interface{}{
					"type":        "seed_based",
					"seed_count":  len(seedWords),
					"match_count": len(matchedKeywords),
				},
			}
			topics = append(topics, topic)
		}
	}
	
	return topics
}

// createTopicsFromKeywords creates topics from remaining keywords using clustering
func (t *BasicTopicModeler) createTopicsFromKeywords(keywords []Keyword, text string, maxTopics int) []Topic {
	topics := []Topic{}
	
	if len(keywords) < t.minTopicSize || maxTopics <= 0 {
		return topics
	}
	
	// Simple clustering: group by semantic similarity (basic approach)
	clusters := t.simpleKeywordClustering(keywords, maxTopics)
	
	for i, cluster := range clusters {
		if len(cluster) >= t.minTopicSize {
			topicKeywords := make([]string, len(cluster))
			for j, keyword := range cluster {
				topicKeywords[j] = keyword.Text
			}
			
			topic := Topic{
				Name:     t.generateTopicName(cluster),
				Keywords: topicKeywords,
				Metadata: map[string]interface{}{
					"type":         "clustered",
					"cluster_id":   i,
					"keyword_count": len(cluster),
				},
			}
			topics = append(topics, topic)
		}
	}
	
	return topics
}

// simpleKeywordClustering performs simple keyword clustering
func (t *BasicTopicModeler) simpleKeywordClustering(keywords []Keyword, maxClusters int) [][]Keyword {
	if len(keywords) <= maxClusters {
		// Each keyword becomes its own cluster
		clusters := make([][]Keyword, len(keywords))
		for i, keyword := range keywords {
			clusters[i] = []Keyword{keyword}
		}
		return clusters
	}
	
	// Sort keywords by relevance
	sort.Slice(keywords, func(i, j int) bool {
		return keywords[i].Relevance > keywords[j].Relevance
	})
	
	// Create clusters based on string similarity and co-occurrence
	clusters := [][]Keyword{}
	used := make(map[int]bool)
	
	for i, keyword := range keywords {
		if used[i] {
			continue
		}
		
		cluster := []Keyword{keyword}
		used[i] = true
		
		// Find similar keywords to add to this cluster
		for j := i + 1; j < len(keywords); j++ {
			if used[j] {
				continue
			}
			
			if t.areKeywordsSimilar(keyword.Text, keywords[j].Text) {
				cluster = append(cluster, keywords[j])
				used[j] = true
				
				// Stop if cluster gets too large
				if len(cluster) >= 8 {
					break
				}
			}
		}
		
		clusters = append(clusters, cluster)
		
		// Stop if we have enough clusters
		if len(clusters) >= maxClusters {
			break
		}
	}
	
	return clusters
}

// areKeywordsSimilar checks if two keywords are semantically similar
func (t *BasicTopicModeler) areKeywordsSimilar(word1, word2 string) bool {
	// Simple similarity checks
	
	// Same word
	if word1 == word2 {
		return true
	}
	
	// One is substring of another
	if strings.Contains(word1, word2) || strings.Contains(word2, word1) {
		return true
	}
	
	// Similar prefixes/suffixes
	if len(word1) > 4 && len(word2) > 4 {
		if strings.HasPrefix(word1, word2[:4]) || strings.HasPrefix(word2, word1[:4]) {
			return true
		}
		if strings.HasSuffix(word1, word2[len(word2)-4:]) || strings.HasSuffix(word2, word1[len(word1)-4:]) {
			return true
		}
	}
	
	// Edit distance check (very basic)
	if t.levenshteinDistance(word1, word2) <= 2 && len(word1) > 4 && len(word2) > 4 {
		return true
	}
	
	return false
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func (t *BasicTopicModeler) levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}
	
	// Create a matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}
	
	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}
	
	// Fill the matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}
			
			deletion := matrix[i-1][j] + 1
			insertion := matrix[i][j-1] + 1
			substitution := matrix[i-1][j-1] + cost
			
			matrix[i][j] = minInt(deletion, minInt(insertion, substitution))
		}
	}
	
	return matrix[len(s1)][len(s2)]
}


// generateTopicName generates a name for a topic based on its keywords
func (t *BasicTopicModeler) generateTopicName(cluster []Keyword) string {
	if len(cluster) == 0 {
		return "Unknown Topic"
	}
	
	// Sort by relevance to get the most important keywords first
	sort.Slice(cluster, func(i, j int) bool {
		return cluster[i].Relevance > cluster[j].Relevance
	})
	
	// Use the top 2-3 keywords to form the topic name
	nameParts := []string{}
	maxParts := 3
	if len(cluster) < maxParts {
		maxParts = len(cluster)
	}
	
	for i := 0; i < maxParts; i++ {
		nameParts = append(nameParts, strings.Title(cluster[i].Text))
	}
	
	return strings.Join(nameParts, " & ")
}

// calculateTopicRelevance calculates how relevant a topic is to the text
func (t *BasicTopicModeler) calculateTopicRelevance(topic Topic, text string) float64 {
	if len(topic.Keywords) == 0 {
		return 0.0
	}
	
	lowerText := strings.ToLower(text)
	totalRelevance := 0.0
	
	for _, keyword := range topic.Keywords {
		// Count occurrences of keyword in text
		count := strings.Count(lowerText, strings.ToLower(keyword))
		
		// Factor in keyword length (longer keywords are more specific)
		lengthFactor := float64(len(keyword)) / 10.0
		if lengthFactor > 1.0 {
			lengthFactor = 1.0
		}
		
		keywordRelevance := float64(count) * (1.0 + lengthFactor)
		totalRelevance += keywordRelevance
	}
	
	// Normalize by number of keywords
	avgRelevance := totalRelevance / float64(len(topic.Keywords))
	
	// Apply diminishing returns
	normalizedRelevance := 1.0 - math.Exp(-avgRelevance/10.0)
	
	return normalizedRelevance
}

// calculateTopicConfidence calculates confidence in topic extraction
func (t *BasicTopicModeler) calculateTopicConfidence(topic Topic, allKeywords []Keyword) float64 {
	if len(topic.Keywords) == 0 {
		return 0.0
	}
	
	// Base confidence on keyword quality
	totalKeywordRelevance := 0.0
	keywordCount := 0
	
	// Create map for quick lookup
	topicKeywordMap := make(map[string]bool)
	for _, keyword := range topic.Keywords {
		topicKeywordMap[keyword] = true
	}
	
	// Sum relevance of keywords in this topic
	for _, keyword := range allKeywords {
		if topicKeywordMap[keyword.Text] {
			totalKeywordRelevance += keyword.Relevance
			keywordCount++
		}
	}
	
	if keywordCount == 0 {
		return 0.0
	}
	
	avgKeywordRelevance := totalKeywordRelevance / float64(keywordCount)
	
	// Factor in topic size (more keywords = higher confidence, up to a point)
	sizeFactor := float64(len(topic.Keywords)) / 5.0
	if sizeFactor > 1.0 {
		sizeFactor = 1.0
	}
	
	confidence := avgKeywordRelevance * (0.7 + 0.3*sizeFactor)
	
	// Clamp to [0, 1]
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.0 {
		confidence = 0.0
	}
	
	return confidence
}

// filterTopics filters out low-quality topics
func (t *BasicTopicModeler) filterTopics(topics []Topic) []Topic {
	filtered := []Topic{}
	
	for _, topic := range topics {
		// Filter out topics with too few keywords
		if len(topic.Keywords) < t.minTopicSize {
			continue
		}
		
		// Filter out topics with very low relevance
		if topic.Relevance < 0.1 {
			continue
		}
		
		// Filter out topics with very low confidence
		if topic.Confidence < 0.3 {
			continue
		}
		
		filtered = append(filtered, topic)
	}
	
	return filtered
}

// initializeTopicSeeds initializes predefined topic seeds
func (t *BasicTopicModeler) initializeTopicSeeds() {
	t.topicSeeds = map[string][]string{
		"Technology": {
			"computer", "software", "hardware", "technology", "digital", "internet",
			"data", "programming", "code", "algorithm", "system", "network",
			"database", "server", "cloud", "artificial", "intelligence", "machine",
			"learning", "automation", "platform", "application", "development",
		},
		"Business": {
			"business", "company", "corporate", "management", "strategy", "revenue",
			"profit", "sales", "marketing", "customer", "client", "market",
			"finance", "financial", "investment", "budget", "cost", "price",
			"commercial", "enterprise", "organization", "operations", "growth",
		},
		"Health": {
			"health", "medical", "healthcare", "patient", "doctor", "hospital",
			"treatment", "medicine", "therapy", "diagnosis", "disease", "clinic",
			"pharmaceutical", "drug", "medication", "surgery", "wellness", "fitness",
			"mental", "physical", "nutrition", "prevention", "care", "recovery",
		},
		"Education": {
			"education", "school", "university", "college", "student", "teacher",
			"learning", "study", "course", "curriculum", "academic", "research",
			"knowledge", "training", "instruction", "classroom", "degree", "program",
			"scholarship", "educational", "pedagogy", "textbook", "examination",
		},
		"Finance": {
			"finance", "financial", "money", "bank", "banking", "investment",
			"loan", "credit", "debt", "insurance", "portfolio", "asset",
			"liability", "equity", "bond", "stock", "market", "trading",
			"capital", "fund", "budget", "accounting", "tax", "economic",
		},
		"Legal": {
			"legal", "law", "court", "judge", "lawyer", "attorney", "case",
			"lawsuit", "litigation", "contract", "agreement", "regulation",
			"compliance", "statute", "legislation", "jurisdiction", "precedent",
			"constitutional", "criminal", "civil", "judicial", "proceedings",
		},
		"Science": {
			"science", "research", "experiment", "hypothesis", "theory", "study",
			"analysis", "scientific", "laboratory", "method", "data", "results",
			"conclusion", "evidence", "observation", "measurement", "biology",
			"chemistry", "physics", "mathematics", "statistics", "publication",
		},
		"Security": {
			"security", "cybersecurity", "protection", "privacy", "encryption",
			"authentication", "authorization", "firewall", "vulnerability", "threat",
			"risk", "attack", "breach", "malware", "virus", "hacking", "secure",
			"compliance", "audit", "monitoring", "incident", "response", "defense",
		},
		"Communication": {
			"communication", "message", "email", "phone", "meeting", "conference",
			"presentation", "discussion", "conversation", "dialogue", "notification",
			"announcement", "broadcast", "social", "media", "network", "channel",
			"platform", "interface", "interaction", "collaboration", "feedback",
		},
		"Project Management": {
			"project", "management", "planning", "schedule", "timeline", "milestone",
			"deadline", "task", "assignment", "resource", "allocation", "budget",
			"scope", "requirement", "deliverable", "stakeholder", "team", "coordination",
			"tracking", "progress", "status", "report", "workflow", "process",
		},
	}
}