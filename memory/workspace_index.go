package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultChunkMaxChars = 500

// SplitIntoChunks 将文本按段落分割成块，供 workspace 索引使用（与 CLI indexFile 逻辑一致）
func SplitIntoChunks(text string, maxChunkSize int) []string {
	if maxChunkSize <= 0 {
		maxChunkSize = defaultChunkMaxChars
	}
	paragraphs := splitParagraphs(text)
	chunks := make([]string, 0)
	currentChunk := ""

	for _, para := range paragraphs {
		if len(currentChunk)+len(para) > maxChunkSize && currentChunk != "" {
			chunks = append(chunks, currentChunk)
			currentChunk = para
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += para
		}
	}

	if currentChunk != "" {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

func splitParagraphs(text string) []string {
	paragraphs := make([]string, 0)
	current := ""

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current != "" {
				paragraphs = append(paragraphs, current)
				current = ""
			}
		} else {
			if current != "" {
				current += " "
			}
			current += line
		}
	}

	if current != "" {
		paragraphs = append(paragraphs, current)
	}

	return paragraphs
}

// IndexFileToManager 将单个文件内容索引到 manager（MEMORY.md 或日更）
func IndexFileToManager(ctx context.Context, manager *MemoryManager, filePath string, source MemorySource, memType MemoryType) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	text := string(content)
	if text == "" {
		return nil
	}

	chunks := SplitIntoChunks(text, defaultChunkMaxChars)
	items := make([]MemoryItem, 0, len(chunks))
	for i, chunk := range chunks {
		items = append(items, MemoryItem{
			Text:   chunk,
			Source: source,
			Type:   memType,
			Metadata: MemoryMetadata{
				FilePath: filePath,
				Tags:     []string{"indexed"},
			},
		})
		if i > 0 {
			items[i-1].Metadata.LineNumber = i * 10
		}
	}

	if len(items) > 0 {
		if err := manager.AddMemoryBatch(ctx, items); err != nil {
			return fmt.Errorf("add memories: %w", err)
		}
	}

	return nil
}

// IndexWorkspaceToStore 将 workspace/memory 下的 MEMORY.md 与日更文件写入给定 store（与 OpenClaw sync 对齐，供 watcher 与 CLI 共用）
func IndexWorkspaceToStore(ctx context.Context, store Store, provider EmbeddingProvider, memoryDir string) error {
	managerConfig := DefaultManagerConfig(store, provider)
	manager, err := NewMemoryManager(managerConfig)
	if err != nil {
		return err
	}

	longTermPath := filepath.Join(memoryDir, "MEMORY.md")
	if _, err := os.Stat(longTermPath); err == nil {
		if err := IndexFileToManager(ctx, manager, longTermPath, MemorySourceLongTerm, MemoryTypeFact); err != nil {
			return fmt.Errorf("index %s: %w", longTermPath, err)
		}
	}

	dailyFiles, err := filepath.Glob(filepath.Join(memoryDir, "????-??-??.md"))
	if err != nil {
		return fmt.Errorf("glob daily notes: %w", err)
	}
	for _, dailyFile := range dailyFiles {
		if err := IndexFileToManager(ctx, manager, dailyFile, MemorySourceDaily, MemoryTypeContext); err != nil {
			return fmt.Errorf("index %s: %w", dailyFile, err)
		}
	}

	return nil
}
