package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const OutputDirName = "nanobanana-output"

type FileHandler struct{}

func NewFileHandler() *FileHandler {
	return &FileHandler{}
}

func (fh *FileHandler) GetSearchPaths() []string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
		if home == "" {
			home = "~"
		}
	}

	return []string{
		cwd,
		filepath.Join(cwd, "images"),
		filepath.Join(cwd, "input"),
		filepath.Join(cwd, OutputDirName),
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "Desktop"),
	}
}

func (fh *FileHandler) EnsureOutputDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	outputPath := filepath.Join(cwd, OutputDirName)
	fh.EnsureDirectory(outputPath)
	return outputPath
}

func (fh *FileHandler) EnsureDirectory(dirPath string) {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		_ = os.MkdirAll(dirPath, 0755)
	}
}

func (fh *FileHandler) SaveTextFile(filePath string, content string) error {
	dir := filepath.Dir(filePath)
	fh.EnsureDirectory(dir)
	return os.WriteFile(filePath, []byte(content), 0644)
}

func (fh *FileHandler) ReadTextFile(filePath string) (string, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (fh *FileHandler) FindInputFile(filename string) FileSearchResult {
	if filepath.IsAbs(filename) {
		if _, err := os.Stat(filename); err == nil {
			return FileSearchResult{
				Found:         true,
				FilePath:      filename,
				SearchedPaths: []string{},
			}
		}
	}

	searchPaths := fh.GetSearchPaths()
	for _, searchPath := range searchPaths {
		fullPath := filepath.Join(searchPath, filename)
		if _, err := os.Stat(fullPath); err == nil {
			return FileSearchResult{
				Found:         true,
				FilePath:      fullPath,
				SearchedPaths: searchPaths,
			}
		}
	}

	return FileSearchResult{
		Found:         false,
		SearchedPaths: searchPaths,
	}
}

func (fh *FileHandler) FindInputDirectory(dirName string) FileSearchDirResult {
	targetPath := dirName
	if !filepath.IsAbs(dirName) {
		cwd, err := os.Getwd()
		if err == nil {
			targetPath = filepath.Join(cwd, dirName)
		}
	}

	fi, err := os.Stat(targetPath)
	if err == nil && fi.IsDir() {
		entries, err := os.ReadDir(targetPath)
		if err == nil {
			var files []string
			imgRegex := regexp.MustCompile(`(?i)\.(png|jpg|jpeg|webp)$`)
			for _, entry := range entries {
				if !entry.IsDir() && imgRegex.MatchString(entry.Name()) {
					files = append(files, filepath.Join(targetPath, entry.Name()))
				}
			}
			return FileSearchDirResult{
				Found:   true,
				DirPath: targetPath,
				Files:   files,
			}
		}
	}

	return FileSearchDirResult{
		Found: false,
		Files: []string{},
	}
}

func (fh *FileHandler) GetSanitizedBaseName(prompt string) string {
	// Lowercase
	baseName := strings.ToLower(prompt)
	// Remove special chars: replace anything that is not lowercase letter, number, or space with ""
	reg := regexp.MustCompile(`[^a-z0-9\s]`)
	baseName = reg.ReplaceAllString(baseName, "")
	// Trim
	baseName = strings.TrimSpace(baseName)
	// Replace multiple spaces with a single underscore
	regSpace := regexp.MustCompile(`\s+`)
	baseName = regSpace.ReplaceAllString(baseName, "_")

	// Limit to 64 chars
	if len(baseName) > 64 {
		baseName = baseName[:64]
	}

	if baseName == "" {
		baseName = "generated_image"
	}
	return baseName
}

func (fh *FileHandler) GenerateFilename(prompt string, format string, index int) string {
	baseName := fh.GetSanitizedBaseName(prompt)
	extension := "png"
	if format == "jpeg" {
		extension = "jpg"
	}

	outputPath := fh.EnsureOutputDirectory()
	fileName := fmt.Sprintf("%s.%s", baseName, extension)
	counter := index
	if counter <= 0 {
		counter = 1
	}

	for {
		fullPath := filepath.Join(outputPath, fileName)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			break
		}
		fileName = fmt.Sprintf("%s_%d.%s", baseName, counter, extension)
		counter++
	}

	return fileName
}

func (fh *FileHandler) SaveImageFromBase64(base64Data string, outputPath string, filename string) (string, error) {
	fh.EnsureDirectory(outputPath)
	// Handle data URI prefix if present
	if idx := strings.Index(base64Data, ","); idx != -1 {
		base64Data = base64Data[idx+1:]
	}
	// Strip spaces
	base64Data = strings.ReplaceAll(base64Data, " ", "")
	base64Data = strings.ReplaceAll(base64Data, "\n", "")
	base64Data = strings.ReplaceAll(base64Data, "\r", "")

	buffer, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(outputPath, filename)
	err = os.WriteFile(fullPath, buffer, 0644)
	if err != nil {
		return "", err
	}
	return fullPath, nil
}

func (fh *FileHandler) ReadImageAsBase64(filePath string) (string, error) {
	buffer, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer), nil
}

type fileMatch struct {
	path  string
	name  string
	mtime int64
}

func (fh *FileHandler) FindLatestFile(baseName string) string {
	outputPath := fh.EnsureOutputDirectory()
	entries, err := os.ReadDir(outputPath)
	if err != nil {
		return ""
	}

	escapedBaseName := regexp.QuoteMeta(baseName)
	pattern := fmt.Sprintf(`(?i)^%s(_phase_1|_final|_\d+)*\.(png|jpg|jpeg)$`, escapedBaseName)
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}

	var matches []fileMatch
	for _, entry := range entries {
		if !entry.IsDir() && regex.MatchString(entry.Name()) {
			fullPath := filepath.Join(outputPath, entry.Name())
			info, err := os.Stat(fullPath)
			if err == nil {
				matches = append(matches, fileMatch{
					path:  fullPath,
					name:  entry.Name(),
					mtime: info.ModTime().UnixNano(),
				})
			}
		}
	}

	if len(matches) == 0 {
		return ""
	}

	// Sort by mtime descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].mtime > matches[j].mtime
	})

	fmt.Fprintf(os.Stderr, "DEBUG - FindLatestFile(%s) found %d matches. Newest: %s\n", baseName, len(matches), matches[0].name)
	return matches[0].path
}

func (fh *FileHandler) FindPageFile(pageNumber string) string {
	outputPath := fh.EnsureOutputDirectory()
	entries, err := os.ReadDir(outputPath)
	if err != nil {
		return ""
	}

	pattern := fmt.Sprintf(`(?i)^manga_page_%s_.*(_final|_phase_1|_\d+)*\.(png|jpg|jpeg)$`, regexp.QuoteMeta(pageNumber))
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}

	type pageMatch struct {
		path  string
		name  string
		score int
		mtime int64
	}

	var matches []pageMatch
	for _, entry := range entries {
		if !entry.IsDir() && regex.MatchString(entry.Name()) {
			fullPath := filepath.Join(outputPath, entry.Name())
			info, err := os.Stat(fullPath)
			if err == nil {
				score := 1
				if strings.Contains(entry.Name(), "_final") {
					score = 3
				} else if strings.Contains(entry.Name(), "_phase_1") {
					score = 2
				}
				matches = append(matches, pageMatch{
					path:  fullPath,
					name:  entry.Name(),
					score: score,
					mtime: info.ModTime().UnixNano(),
				})
			}
		}
	}

	if len(matches) == 0 {
		return ""
	}

	// Sort: score descending, then mtime descending
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].mtime > matches[j].mtime
	})

	fmt.Fprintf(os.Stderr, "DEBUG - FindPageFile(%s) found match: %s\n", pageNumber, filepath.Base(matches[0].path))
	return matches[0].path
}
