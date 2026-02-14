package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SearchResultDeduplicator 搜索结果去重器
type SearchResultDeduplicator struct {
	similarityThreshold float64 // 相似度阈值 (0-1)
}

// NewSearchResultDeduplicator 创建搜索结果去重器
func NewSearchResultDeduplicator(similarityThreshold float64) *SearchResultDeduplicator {
	if similarityThreshold <= 0 || similarityThreshold > 1 {
		similarityThreshold = 0.85 // 默认 85% 相似度
	}

	return &SearchResultDeduplicator{
		similarityThreshold: similarityThreshold,
	}
}

// Deduplicate 对搜索结果去重
func (d *SearchResultDeduplicator) Deduplicate(results []SearchResult) []SearchResult {
	if len(results) <= 1 {
		return results
	}

	// 使用内容哈希进行精确去重
	seen := make(map[string]bool)
	var deduped []SearchResult

	for _, result := range results {
		hash := d.contentHash(result.Text)

		if !seen[hash] {
			seen[hash] = true
			deduped = append(deduped, result)
		}
	}

	// 如果启用了相似度去重，进行模糊去重
	if d.similarityThreshold < 1.0 {
		deduped = d.deduplicateBySimilarity(deduped)
	}

	return deduped
}

// contentHash 计算内容哈希
func (d *SearchResultDeduplicator) contentHash(content string) string {
	// 标准化内容
	normalized := d.normalizeContent(content)

	// 计算 SHA256 哈希
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// normalizeContent 标准化内容
func (d *SearchResultDeduplicator) normalizeContent(content string) string {
	// 转小写
	content = strings.ToLower(content)

	// 移除多余空白
	content = strings.Join(strings.Fields(content), " ")

	// 移除标点符号（可选）
	// content = strings.Map(func(r rune) rune {
	// 	if unicode.IsPunct(r) {
	// 		return -1
	// 	}
	// 	return r
	// }, content)

	return strings.TrimSpace(content)
}

// deduplicateBySimilarity 基于相似度去重
func (d *SearchResultDeduplicator) deduplicateBySimilarity(results []SearchResult) []SearchResult {
	if len(results) <= 1 {
		return results
	}

	var deduped []SearchResult
	deduped = append(deduped, results[0])

	for i := 1; i < len(results); i++ {
		isDuplicate := false

		for j := 0; j < len(deduped); j++ {
			similarity := d.calculateSimilarity(results[i].Text, deduped[j].Text)

			if similarity >= d.similarityThreshold {
				isDuplicate = true
				// 如果新结果的分数更高，替换旧结果
				if results[i].Score > deduped[j].Score {
					deduped[j] = results[i]
				}
				break
			}
		}

		if !isDuplicate {
			deduped = append(deduped, results[i])
		}
	}

	return deduped
}

// calculateSimilarity 计算两个文本的相似度（使用 Jaccard 相似度）
func (d *SearchResultDeduplicator) calculateSimilarity(text1, text2 string) float64 {
	// 分词
	words1 := d.tokenize(text1)
	words2 := d.tokenize(text2)

	// 计算交集和并集
	intersection := 0
	union := make(map[string]bool)

	for word := range words1 {
		union[word] = true
	}

	for word := range words2 {
		if words1[word] {
			intersection++
		}
		union[word] = true
	}

	if len(union) == 0 {
		return 0
	}

	// Jaccard 相似度 = 交集 / 并集
	return float64(intersection) / float64(len(union))
}

// tokenize 分词
func (d *SearchResultDeduplicator) tokenize(text string) map[string]bool {
	// 标准化
	text = d.normalizeContent(text)

	// 分词
	words := strings.Fields(text)

	// 转为集合
	wordSet := make(map[string]bool)
	for _, word := range words {
		if len(word) > 2 { // 过滤短词
			wordSet[word] = true
		}
	}

	return wordSet
}

// DeduplicateByMetadata 基于元数据去重
func (d *SearchResultDeduplicator) DeduplicateByMetadata(results []SearchResult, metadataKey string) []SearchResult {
	if len(results) <= 1 {
		return results
	}

	seen := make(map[string]bool)
	var deduped []SearchResult

	for _, result := range results {
		// 获取元数据值
		var value string
		switch metadataKey {
		case "file_path":
			value = result.Metadata.FilePath
		case "session_key":
			value = result.Metadata.SessionKey
		default:
			// 未知的元数据键，保留结果
			deduped = append(deduped, result)
			continue
		}

		if value == "" {
			deduped = append(deduped, result)
			continue
		}

		if !seen[value] {
			seen[value] = true
			deduped = append(deduped, result)
		}
	}

	return deduped
}

// MergeAndDeduplicate 合并多个搜索结果并去重
func (d *SearchResultDeduplicator) MergeAndDeduplicate(resultSets ...[]SearchResult) []SearchResult {
	// 合并所有结果
	var merged []SearchResult
	for _, results := range resultSets {
		merged = append(merged, results...)
	}

	// 去重
	return d.Deduplicate(merged)
}
