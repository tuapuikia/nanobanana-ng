package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type MemoryUpdateData struct {
	FilePath   string
	Reason     string
	FailedPath string
	Prompt     string
}

type MemoryCheckResult struct {
	Phase1           string
	Phase2           string
	Phase1Prompt     string
	Phase1PromptPath string
}

type MemoryFailures struct {
	Reasons     []string
	FailedPaths []string
}

type MemoryHandler struct {
	fh *FileHandler
}

func NewMemoryHandler(fh *FileHandler) *MemoryHandler {
	return &MemoryHandler{fh: fh}
}

func (mh *MemoryHandler) GetMemoryFilePath(storyFile string) (string, error) {
	absPath, err := filepath.Abs(storyFile)
	if err != nil {
		return "", err
	}
	storyDir := filepath.Dir(absPath)
	return filepath.Join(storyDir, "manga_memory.md"), nil
}

func (mh *MemoryHandler) UpdateMemory(
	memoryPath string,
	pageHeader string,
	phase int,
	status string, // "PASSED" or "FAILED"
	data MemoryUpdateData,
) error {
	var content string
	var err error

	if _, errStat := os.Stat(memoryPath); errStat == nil {
		content, err = mh.fh.ReadTextFile(memoryPath)
		if err != nil {
			return err
		}
	}

	escapedHeader := regexp.QuoteMeta(pageHeader)
	blockPattern := fmt.Sprintf(`(?i)(## %s\s*\n)([\s\S]*?)(?=(\n## |$))`, escapedHeader)
	blockRegex, err := regexp.Compile(blockPattern)
	if err != nil {
		return err
	}

	match := blockRegex.FindStringSubmatch(content)
	var blockHeader string
	var blockContent string

	if len(match) > 0 {
		blockHeader = match[1]
		blockContent = match[2]
	} else {
		blockHeader = fmt.Sprintf("## %s\n", pageHeader)
		blockContent = ""
	}

	if status == "PASSED" {
		phaseLineMarker := fmt.Sprintf("- Phase %d:", phase)
		newLine := fmt.Sprintf("%s `%s` [PASSED]", phaseLineMarker, data.FilePath)
		phasePattern := fmt.Sprintf(`(?m)^\s*%s.*$`, regexp.QuoteMeta(phaseLineMarker))
		phaseRegex, err := regexp.Compile(phasePattern)
		if err != nil {
			return err
		}

		if phaseRegex.MatchString(blockContent) {
			blockContent = phaseRegex.ReplaceAllString(blockContent, newLine)
		} else {
			blockContent = strings.TrimRight(blockContent, "\r\n") + "\n" + newLine + "\n"
		}

		if data.Prompt != "" {
			promptMarker := fmt.Sprintf("- Phase %d Prompt:", phase)
			safeHeader := mh.fh.GetSanitizedBaseName(pageHeader)
			promptFileName := fmt.Sprintf("prompt_%s_p%d.txt", safeHeader, phase)
			promptPath := filepath.Join(filepath.Dir(memoryPath), "prompts", promptFileName)

			err = mh.fh.SaveTextFile(promptPath, data.Prompt)
			if err != nil {
				fmt.Fprintf(os.Stderr, "DEBUG - Failed to save Phase %d prompt: %v\n", phase, err)
			}

			promptLine := fmt.Sprintf("%s `%s`", promptMarker, promptFileName)
			promptPattern := fmt.Sprintf(`(?m)^\s*%s.*$`, regexp.QuoteMeta(promptMarker))
			promptRegex, err := regexp.Compile(promptPattern)
			if err == nil {
				if promptRegex.MatchString(blockContent) {
					blockContent = promptRegex.ReplaceAllString(blockContent, promptLine)
				} else {
					blockContent = strings.TrimRight(blockContent, "\r\n") + "\n" + promptLine + "\n"
				}
			}
		}
	} else {
		// FAILED
		failLine := fmt.Sprintf("- Phase %d Attempt: FAILED. Reason: %s", phase, data.Reason)
		if data.FailedPath != "" {
			failLine += fmt.Sprintf(" [FILE: `%s`]", data.FailedPath)
		}

		if !strings.Contains(blockContent, failLine) {
			blockContent = strings.TrimRight(blockContent, "\r\n") + "\n" + failLine + "\n"
		}
	}

	var newContent string
	if len(match) > 0 {
		newContent = blockRegex.ReplaceAllString(content, blockHeader+blockContent)
	} else {
		newContent = content + "\n\n" + blockHeader + blockContent
	}

	newContent = strings.TrimSpace(newContent)
	err = mh.fh.SaveTextFile(memoryPath, newContent)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "DEBUG - Updated Memory: %s Phase %d [%s]\n", pageHeader, phase, status)
	return nil
}

func (mh *MemoryHandler) GetFailures(memoryPath string, pageHeader string) (MemoryFailures, error) {
	result := MemoryFailures{
		Reasons:     []string{},
		FailedPaths: []string{},
	}

	if _, err := os.Stat(memoryPath); os.IsNotExist(err) {
		return result, nil
	}

	content, err := mh.fh.ReadTextFile(memoryPath)
	if err != nil {
		return result, err
	}

	escapedHeader := regexp.QuoteMeta(pageHeader)
	blockPattern := fmt.Sprintf(`(?i)## %s\s*\n([\s\S]*?)(?=(\n## |$))`, escapedHeader)
	blockRegex, err := regexp.Compile(blockPattern)
	if err != nil {
		return result, err
	}

	match := blockRegex.FindStringSubmatch(content)
	if len(match) == 0 {
		return result, nil
		// No memory block for this page yet
	}

	blockContent := match[1]
	lines := strings.Split(blockContent, "\n")

	// Match pattern like: - Phase 1 Attempt: FAILED. Reason: [reason] [FILE: `[path]`]
	failPattern := "- Phase \\d+ Attempt: FAILED\\. Reason: (.*?)(?: \\[FILE: `([^`]+)`\\])?$"
	failRegex, err := regexp.Compile(failPattern)
	if err != nil {
		return result, err
	}

	reasonsMap := make(map[string]bool)
	pathsMap := make(map[string]bool)

	for _, line := range lines {
		m := failRegex.FindStringSubmatch(line)
		if len(m) > 0 {
			reason := strings.TrimSpace(m[1])
			reasonsMap[reason] = true

			if len(m) > 2 && m[2] != "" {
				failedPath := m[2]
				if _, err := os.Stat(failedPath); err == nil {
					pathsMap[failedPath] = true
				}
			}
		}
	}

	for r := range reasonsMap {
		result.Reasons = append(result.Reasons, r)
	}
	for p := range pathsMap {
		result.FailedPaths = append(result.FailedPaths, p)
	}

	return result, nil
}

func (mh *MemoryHandler) CheckMemory(memoryPath string, pageHeader string) (MemoryCheckResult, error) {
	result := MemoryCheckResult{}

	if _, err := os.Stat(memoryPath); os.IsNotExist(err) {
		return result, nil
	}

	content, err := mh.fh.ReadTextFile(memoryPath)
	if err != nil {
		return result, err
	}

	escapedHeader := regexp.QuoteMeta(pageHeader)
	blockPattern := fmt.Sprintf(`(?i)## %s\s*\n([\s\S]*?)(?=(\n## |$))`, escapedHeader)
	blockRegex, err := regexp.Compile(blockPattern)
	if err != nil {
		return result, err
	}

	match := blockRegex.FindStringSubmatch(content)
	if len(match) == 0 {
		return result, nil
	}

	blockContent := match[1]

	// Extract Phase 1
	p1Regex := regexp.MustCompile("(?m)^\\s*- Phase 1: `([^`]+)` \\[PASSED\\]")
	p1Match := p1Regex.FindStringSubmatch(blockContent)
	if len(p1Match) > 1 {
		if _, err := os.Stat(p1Match[1]); err == nil {
			result.Phase1 = p1Match[1]
		}
	}

	// Extract Phase 2
	p2Regex := regexp.MustCompile("(?m)^\\s*- Phase 2: `([^`]+)` \\[PASSED\\]")
	p2Match := p2Regex.FindStringSubmatch(blockContent)
	if len(p2Match) > 1 {
		if _, err := os.Stat(p2Match[1]); err == nil {
			result.Phase2 = p2Match[1]
		}
	}

	// Extract Phase 1 Prompt File
	promptRegex := regexp.MustCompile("(?m)^\\s*- Phase 1 Prompt: `([^`]+)`")
	promptMatch := promptRegex.FindStringSubmatch(blockContent)
	if len(promptMatch) > 1 {
		promptPath := filepath.Join(filepath.Dir(memoryPath), "prompts", promptMatch[1])
		if _, err := os.Stat(promptPath); err == nil {
			result.Phase1PromptPath = promptPath
			promptContent, errRead := mh.fh.ReadTextFile(promptPath)
			if errRead == nil {
				result.Phase1Prompt = promptContent
			}
		}
	}

	return result, nil
}
