package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genai"
)

type ImageGenerator struct {
	ai        *genai.Client
	artModel  string
	textModel string
	modelName string
	fh        *FileHandler
	mh        *MemoryHandler
}

const (
	DefaultModel     = "gemini-3.1-flash-image-preview"
	DefaultTextModel = "gemini-3.1-flash-image-preview"
)

type ReferenceImage struct {
	Data       string `json:"data"`
	MIMEType   string `json:"mimeType"`
	SourcePath string `json:"sourcePath"`
}

type ReviewResponse struct {
	LikenessScore   float64 `json:"likeness_score"`
	ContinuityScore float64 `json:"continuity_score"`
	NoBubblesScore  float64 `json:"no_bubbles_score"`
	LetteringScore  float64 `json:"lettering_score"`
	StoryScore      float64 `json:"story_score"`
	TotalScore      float64 `json:"total_score"`
	Reason          string  `json:"reason"`
	Pass            bool    `json:"pass"`
}

func NewImageGenerator(authConfig AuthConfig, fh *FileHandler, mh *MemoryHandler) (*ImageGenerator, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  authConfig.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create GenAI client: %w", err)
	}

	artModel := os.Getenv("NANOBANANA_ART_MODEL")
	if artModel == "" {
		artModel = DefaultModel
	}

	textModel := os.Getenv("NANOBANANA_TEXT_MODEL")
	if textModel == "" {
		textModel = DefaultTextModel
	}

	modelName := os.Getenv("NANOBANANA_MODEL")
	if modelName == "" {
		modelName = textModel
	}

	fmt.Fprintf(os.Stderr, "DEBUG - Models initialized: Art=%s, Text=%s, Default=%s\n", artModel, textModel, modelName)

	return &ImageGenerator{
		ai:        client,
		artModel:  artModel,
		textModel: textModel,
		modelName: modelName,
		fh:        fh,
		mh:        mh,
	}, nil
}

func ValidateAuthentication() (AuthConfig, error) {
	fmt.Fprintln(os.Stderr, "DEBUG - Validating authentication...")

	if nanoGeminiKey := os.Getenv("NANOBANANA_GEMINI_API_KEY"); nanoGeminiKey != "" {
		fmt.Fprintln(os.Stderr, "✓ Found NANOBANANA_GEMINI_API_KEY environment variable")
		return AuthConfig{APIKey: nanoGeminiKey, KeyType: "GEMINI_API_KEY"}, nil
	}
	fmt.Fprintln(os.Stderr, "DEBUG - NANOBANANA_GEMINI_API_KEY not found")

	if nanoGoogleKey := os.Getenv("NANOBANANA_GOOGLE_API_KEY"); nanoGoogleKey != "" {
		fmt.Fprintln(os.Stderr, "✓ Found NANOBANANA_GOOGLE_API_KEY environment variable")
		return AuthConfig{APIKey: nanoGoogleKey, KeyType: "GOOGLE_API_KEY"}, nil
	}
	fmt.Fprintln(os.Stderr, "DEBUG - NANOBANANA_GOOGLE_API_KEY not found")

	if geminiKey := os.Getenv("GEMINI_API_KEY"); geminiKey != "" {
		fmt.Fprintln(os.Stderr, "✓ Found GEMINI_API_KEY environment variable (fallback)")
		return AuthConfig{APIKey: geminiKey, KeyType: "GEMINI_API_KEY"}, nil
	}
	fmt.Fprintln(os.Stderr, "DEBUG - GEMINI_API_KEY not found")

	if googleKey := os.Getenv("GOOGLE_API_KEY"); googleKey != "" {
		fmt.Fprintln(os.Stderr, "✓ Found GOOGLE_API_KEY environment variable (fallback)")
		return AuthConfig{APIKey: googleKey, KeyType: "GOOGLE_API_KEY"}, nil
	}
	fmt.Fprintln(os.Stderr, "DEBUG - GOOGLE_API_KEY not found")

	if geminiCliAppKey := os.Getenv("GEMINI_CLI_APP"); geminiCliAppKey != "" {
		fmt.Fprintln(os.Stderr, "✓ Found GEMINI_CLI_APP environment variable (fallback)")
		return AuthConfig{APIKey: geminiCliAppKey, KeyType: "GEMINI_API_KEY"}, nil
	}
	fmt.Fprintln(os.Stderr, "DEBUG - GEMINI_CLI_APP not found")

	return AuthConfig{}, fmt.Errorf("ERROR: No valid API key found. Please set NANOBANANA_GEMINI_API_KEY, NANOBANANA_GOOGLE_API_KEY, GEMINI_API_KEY, GEMINI_CLI_APP, or GOOGLE_API_KEY environment variable.\n" +
		"For more details on authentication, visit: https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/authentication.md")
}

func isValidBase64ImageData(data string) bool {
	if len(data) < 100 {
		return false
	}
	base64Regex := regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)
	if !base64Regex.MatchString(data) {
		return false
	}
	return len(data) >= 1000
}

func (ig *ImageGenerator) getSafetySettings() []*genai.SafetySetting {
	return []*genai.SafetySetting{
		{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdBlockNone},
	}
}

func (ig *ImageGenerator) openImagePreview(filePath string) {
	platform := runtime.GOOS
	var cmd *exec.Cmd

	switch platform {
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", filePath)
	default:
		cmd = exec.Command("xdg-open", filePath)
	}

	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG - Failed to open preview for %s: %v\n", filePath, err)
	} else {
		fmt.Fprintf(os.Stderr, "DEBUG - Opened preview for: %s\n", filePath)
	}
}

func (ig *ImageGenerator) shouldAutoPreview(request ImageGenerationRequest) bool {
	if request.NoPreview {
		return false
	}
	return request.Preview
}

func (ig *ImageGenerator) handlePreview(files []string, request ImageGenerationRequest) {
	if !ig.shouldAutoPreview(request) || len(files) == 0 {
		return
	}
	for _, file := range files {
		ig.openImagePreview(file)
	}
}

func (ig *ImageGenerator) logToDisk(message string) {
	logDir := ig.fh.EnsureOutputDirectory()
	logFile := filepath.Join(logDir, "nanobanana-output.log")
	timestamp := time.Now().Format(time.RFC3339)
	entry := fmt.Sprintf("[%s] %s\n", timestamp, message)

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG - Failed to write to log file: %v\n", err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG - Failed to write to log file: %v\n", err)
	}
}

func (ig *ImageGenerator) logGeneration(modelName string, generatedFiles []string, referenceInfo string) {
	logEntry := fmt.Sprintf("Model: %s, Generated Files: %s", modelName, strings.Join(generatedFiles, ", "))
	if referenceInfo != "" {
		logEntry += fmt.Sprintf(", Reference: %s", referenceInfo)
	}
	ig.logToDisk(logEntry)
	fmt.Fprintln(os.Stderr, "DEBUG - Logged generation to file.")
}

func (ig *ImageGenerator) getAspectRatioString(layout string) string {
	switch layout {
	case "webtoon":
		return "9:16"
	case "strip":
		return "16:9"
	case "single_page":
		return "3:4"
	case "square":
	default:
	}
	return "1:1"
}

func (ig *ImageGenerator) getAspectRatioInstruction(layout string) string {
	switch layout {
	case "webtoon":
		return "aspect ratio 9:16, vertical orientation"
	case "strip":
		return "aspect ratio 16:9, landscape orientation"
	case "single_page":
		return "aspect ratio 3:4, portrait orientation"
	case "square":
	default:
	}
	return "aspect ratio 1:1, square orientation"
}

func buildMangaPrompt(args GenerateMangaArgs) string {
	basePrompt := args.Prompt
	if basePrompt == "" {
		basePrompt = "Manga page"
	}
	style := args.Style
	if style == "" {
		style = "shonen"
	}
	layout := args.Layout
	if layout == "" {
		layout = "square"
	}
	isColor := false
	if args.Color != nil {
		isColor = *args.Color
	}

	var prompt string
	if isColor {
		prompt = fmt.Sprintf("%s, FULL COLOR %s style", basePrompt, style)
	} else {
		prompt = fmt.Sprintf("%s, %s manga style", basePrompt, style)
	}

	prompt += fmt.Sprintf(", %s layout, professional art, high quality", layout)

	if !isColor {
		prompt += ", detailed ink work, screentones, black and white, traditional manga format"
	} else {
		prompt += ", vibrant colors, anime style coloring, digital painting"
	}

	if layout == "webtoon" {
		prompt += ", vertical scrolling format"
	}

	return prompt
}

func (ig *ImageGenerator) buildBatchPrompts(request ImageGenerationRequest) []string {
	var prompts []string
	basePrompt := request.Prompt

	if len(request.Styles) == 0 && len(request.Variations) == 0 && request.OutputCount <= 1 {
		return []string{basePrompt}
	}

	if len(request.Styles) > 0 {
		for _, style := range request.Styles {
			prompts = append(prompts, fmt.Sprintf("%s, %s style", basePrompt, style))
		}
	}

	if len(request.Variations) > 0 {
		basePrompts := prompts
		if len(basePrompts) == 0 {
			basePrompts = []string{basePrompt}
		}
		var variationPrompts []string

		for _, baseP := range basePrompts {
			for _, variation := range request.Variations {
				switch variation {
				case "lighting":
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, dramatic lighting", baseP))
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, soft lighting", baseP))
				case "angle":
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, from above", baseP))
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, close-up view", baseP))
				case "color-palette":
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, warm color palette", baseP))
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, cool color palette", baseP))
				case "composition":
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, centered composition", baseP))
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, rule of thirds composition", baseP))
				case "mood":
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, cheerful mood", baseP))
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, dramatic mood", baseP))
				case "season":
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, in spring", baseP))
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, in winter", baseP))
				case "time-of-day":
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, at sunrise", baseP))
					variationPrompts = append(variationPrompts, fmt.Sprintf("%s, at sunset", baseP))
				}
			}
		}
		if len(variationPrompts) > 0 {
			prompts = variationPrompts
		}
	}

	if len(prompts) == 0 && request.OutputCount > 1 {
		for i := 0; i < request.OutputCount; i++ {
			prompts = append(prompts, basePrompt)
		}
	}

	if request.OutputCount > 0 && len(prompts) > request.OutputCount {
		prompts = prompts[:request.OutputCount]
	}

	if len(prompts) == 0 {
		return []string{basePrompt}
	}
	return prompts
}

func (ig *ImageGenerator) extractImagePaths(text string) []string {
	reg := regexp.MustCompile(`(?i)(?:[\w\-./\\:]+)\.(?:png|jpg|jpeg|webp)`)
	matches := reg.FindAllString(text, -1)
	if len(matches) == 0 {
		return []string{}
	}

	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}

func (ig *ImageGenerator) detectColorRequirement(text string) *bool {
	lowerText := strings.ToLower(text)
	colorKeywords := []string{
		"full color",
		"generate in color",
		"colored manga",
		"vibrant colors",
		"anime style coloring",
		"digital painting",
	}
	bwKeywords := []string{
		"black and white",
		"black & white",
		"b/w",
		"monochrome",
		"screentones",
		"ink work",
		"traditional manga style",
	}

	for _, kw := range colorKeywords {
		if strings.Contains(lowerText, kw) {
			b := true
			return &b
		}
	}
	for _, kw := range bwKeywords {
		if strings.Contains(lowerText, kw) {
			b := false
			return &b
		}
	}
	return nil
}

func (ig *ImageGenerator) parseScoreParameter(value any, defaultValue float64) float64 {
	if value == nil {
		return defaultValue
	}

	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		strValue := strings.TrimSpace(strings.ToLower(v))
		mapping := map[string]float64{
			"ignore":       0,
			"none":         0,
			"very-lenient": 2,
			"lenient":      4,
			"balanced":     6,
			"normal":       7,
			"strict":       9,
			"perfect":      10,
		}
		if mappedVal, ok := mapping[strValue]; ok {
			fmt.Fprintf(os.Stderr, "DEBUG - Translated natural language score \"%s\" to %.1f\n", strValue, mappedVal)
			return mappedVal
		}
		parsed, err := strconv.ParseFloat(strValue, 64)
		if err == nil {
			return parsed
		}
	}
	return defaultValue
}

func (ig *ImageGenerator) GenerateTextToImage(request ImageGenerationRequest) (ImageGenerationResponse, error) {
	ctx := context.Background()
	outputPath := ig.fh.EnsureOutputDirectory()
	var generatedFiles []string
	prompts := ig.buildBatchPrompts(request)
	var firstError string

	fmt.Fprintf(os.Stderr, "DEBUG - Generating %d image variation(s)\n", len(prompts))

	for i, currentPrompt := range prompts {
		fmt.Fprintf(os.Stderr, "DEBUG - Generating variation %d/%d: %s\n", i+1, len(prompts), currentPrompt)

		var responseModalities []string
		if request.IncludeText {
			responseModalities = []string{"IMAGE", "TEXT"}
		} else {
			responseModalities = []string{"IMAGE"}
		}

		config := &genai.GenerateContentConfig{
			ResponseModalities: responseModalities,
			SafetySettings:     ig.getSafetySettings(),
		}
		if request.Temperature != nil {
			t := float32(*request.Temperature)
			config.Temperature = &t
		}
		if request.TopP != nil {
			p := float32(*request.TopP)
			config.TopP = &p
		}

		resp, err := ig.ai.Models.GenerateContent(
			ctx,
			ig.modelName,
			[]*genai.Content{{
				Role:  "user",
				Parts: []*genai.Part{{Text: currentPrompt}},
			}},
			config,
		)
		if err != nil {
			firstError = err.Error()
			fmt.Fprintf(os.Stderr, "DEBUG - Error in GenerateTextToImage: %v\n", err)
			continue
		}

		if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
			for _, part := range resp.Candidates[0].Content.Parts {
				var b64Data string
				if part.InlineData != nil && len(part.InlineData.Data) > 0 {
					b64Data = base64.StdEncoding.EncodeToString(part.InlineData.Data)
				} else if part.Text != "" && isValidBase64ImageData(part.Text) {
					b64Data = part.Text
				}

				if b64Data != "" {
					filename := ig.fh.GenerateFilename(currentPrompt, request.FileFormat, i)
					fullPath, errSave := ig.fh.SaveImageFromBase64(b64Data, outputPath, filename)
					if errSave == nil {
						generatedFiles = append(generatedFiles, fullPath)
						ig.logGeneration(ig.modelName, []string{fullPath}, "")
					} else {
						firstError = errSave.Error()
					}
					break
				}
			}
		}
	}

	if len(generatedFiles) > 0 {
		ig.handlePreview(generatedFiles, request)
		return ImageGenerationResponse{
			Success:        true,
			Message:        fmt.Sprintf("Successfully generated %d image(s)", len(generatedFiles)),
			GeneratedFiles: generatedFiles,
		}, nil
	}

	return ImageGenerationResponse{
		Success: false,
		Message: "Failed to generate any images",
		Error:   firstError,
	}, nil
}

func (ig *ImageGenerator) EditImage(request ImageGenerationRequest) (ImageGenerationResponse, error) {
	ctx := context.Background()
	outputPath := ig.fh.EnsureOutputDirectory()

	fileRes := ig.fh.FindInputFile(request.InputImage)
	if !fileRes.Found {
		return ImageGenerationResponse{
			Success: false,
			Message: fmt.Sprintf("Input image not found: %s", request.InputImage),
		}, nil
	}

	imgB64, err := ig.fh.ReadImageAsBase64(fileRes.FilePath)
	if err != nil {
		return ImageGenerationResponse{
			Success: false,
			Message: "Failed to read input image",
			Error:   err.Error(),
		}, nil
	}

	promptText := request.Prompt
	if request.Mode == "restore" {
		promptText = fmt.Sprintf("Restore and enhance this image: %s", request.Prompt)
	}

	imgBytes, _ := base64.StdEncoding.DecodeString(imgB64)

	resp, err := ig.ai.Models.GenerateContent(
		ctx,
		ig.modelName,
		[]*genai.Content{{
			Role: "user",
			Parts: []*genai.Part{
				{Text: promptText},
				{InlineData: &genai.Blob{Data: imgBytes, MIMEType: "image/png"}},
			},
		}},
		&genai.GenerateContentConfig{
			ResponseModalities: []string{"IMAGE"},
			SafetySettings:     ig.getSafetySettings(),
		},
	)
	if err != nil {
		return ImageGenerationResponse{
			Success: false,
			Message: "Error calling model",
			Error:   err.Error(),
		}, nil
	}

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		for _, part := range resp.Candidates[0].Content.Parts {
			var b64Data string
			if part.InlineData != nil && len(part.InlineData.Data) > 0 {
				b64Data = base64.StdEncoding.EncodeToString(part.InlineData.Data)
			} else if part.Text != "" && isValidBase64ImageData(part.Text) {
				b64Data = part.Text
			}

			if b64Data != "" {
				filename := ig.fh.GenerateFilename(request.Prompt, request.FileFormat, 0)
				fullPath, errSave := ig.fh.SaveImageFromBase64(b64Data, outputPath, filename)
				if errSave == nil {
					ig.logGeneration(ig.modelName, []string{fullPath}, fmt.Sprintf("Input: %s", fileRes.FilePath))
					ig.handlePreview([]string{fullPath}, request)
					return ImageGenerationResponse{
						Success:        true,
						Message:        "Image edited successfully",
						GeneratedFiles: []string{fullPath},
					}, nil
				} else {
					return ImageGenerationResponse{
						Success: false,
						Message: "Failed to save edited image",
						Error:   errSave.Error(),
					}, nil
				}
			}
		}
	}

	return ImageGenerationResponse{
		Success: false,
		Message: "Model returned no image data",
	}, nil
}

func (ig *ImageGenerator) GenerateStorySequence(request ImageGenerationRequest, storyArgs StorySequenceArgs) (ImageGenerationResponse, error) {
	steps := request.OutputCount
	if steps <= 0 {
		steps = 4
	}

	basePrompt := request.Prompt
	var prompts []string
	for idx := 1; idx <= steps; idx++ {
		prompts = append(prompts, fmt.Sprintf("%s, sequence step %d of %d", basePrompt, idx, steps))
	}

	request.OutputCount = 1
	var generatedFiles []string
	var firstError string

	for _, p := range prompts {
		req := request
		req.Prompt = p
		res, err := ig.GenerateTextToImage(req)
		if err == nil && res.Success {
			generatedFiles = append(generatedFiles, res.GeneratedFiles...)
		} else if err != nil {
			firstError = err.Error()
		} else {
			firstError = res.Error
		}
	}

	if len(generatedFiles) > 0 {
		return ImageGenerationResponse{
			Success:        true,
			Message:        fmt.Sprintf("Generated %d story sequence images", len(generatedFiles)),
			GeneratedFiles: generatedFiles,
		}, nil
	}

	return ImageGenerationResponse{
		Success: false,
		Message: "Failed to generate story sequence",
		Error:   firstError,
	}, nil
}

func (ig *ImageGenerator) GenerateMangaPage(request ImageGenerationRequest) (ImageGenerationResponse, error) {
	ctx := context.Background()

	// 1. Character Creation Only / Mode
	if request.CreateCharacter {
		logMsg := fmt.Sprintf("Character Creation Mode: Converting %s for %s", request.InputImage, request.StoryFile)
		fmt.Fprintln(os.Stderr, "DEBUG -", logMsg)
		if request.InputImage == "" || request.StoryFile == "" {
			return ImageGenerationResponse{
				Success: false,
				Message: "Missing inputImage or storyFile",
				Error:   "Missing inputImage or storyFile",
			}, nil
		}

		storyFileRes := ig.fh.FindInputFile(request.StoryFile)
		storyDir := "."
		if storyFileRes.Found {
			storyDir = filepath.Dir(storyFileRes.FilePath)
		}

		charsDir := filepath.Join(storyDir, "characters")
		ig.fh.EnsureDirectory(charsDir)

		inputRes := ig.fh.FindInputFile(request.InputImage)
		if !inputRes.Found {
			return ImageGenerationResponse{
				Success: false,
				Message: fmt.Sprintf("Input image not found: %s", request.InputImage),
			}, nil
		}

		b64, err := ig.fh.ReadImageAsBase64(inputRes.FilePath)
		if err != nil {
			return ImageGenerationResponse{
				Success: false,
				Message: "Failed to read input image",
				Error:   err.Error(),
			}, nil
		}

		// Save character sheet (both B&W and Color)
		cleanBase := strings.TrimSuffix(filepath.Base(inputRes.FilePath), filepath.Ext(inputRes.FilePath))
		bwName := fmt.Sprintf("%s_portrait.png", cleanBase)
		colorName := fmt.Sprintf("%s_portrait_color.png", cleanBase)

		var generatedFiles []string
		path1, err1 := ig.fh.SaveImageFromBase64(b64, charsDir, bwName)
		if err1 == nil {
			generatedFiles = append(generatedFiles, path1)
		}
		path2, err2 := ig.fh.SaveImageFromBase64(b64, charsDir, colorName)
		if err2 == nil {
			generatedFiles = append(generatedFiles, path2)
		}

		ig.logToDisk(logMsg)
		return ImageGenerationResponse{
			Success:        true,
			Message:        fmt.Sprintf("Created character portraits in %s", charsDir),
			GeneratedFiles: generatedFiles,
		}, nil
	}

	// 2. Standard Manga Generation
	if request.StoryFile == "" {
		return ImageGenerationResponse{
			Success: false,
			Message: "Story file is required for manga generation (unless --createCharacter is provided)",
			Error:   "Missing storyFile parameter",
		}, nil
	}

	ratioInstruction := ig.getAspectRatioInstruction(request.Layout)
	fmt.Fprintf(os.Stderr, "DEBUG - Adding aspect ratio instruction: %s\n", ratioInstruction)

	storyFileResult := ig.fh.FindInputFile(request.StoryFile)
	if !storyFileResult.Found {
		return ImageGenerationResponse{
			Success: false,
			Message: fmt.Sprintf("Story file not found: %s", request.StoryFile),
			Error:   fmt.Sprintf("Searched in: %s", strings.Join(storyFileResult.SearchedPaths, ", ")),
		}, nil
	}

	storyContent, err := ig.fh.ReadTextFile(storyFileResult.FilePath)
	if err != nil {
		return ImageGenerationResponse{
			Success: false,
			Message: "Failed to read story file",
			Error:   err.Error(),
		}, nil
	}

	// Auto-detect color requirement
	if !request.Color {
		detectedColor := ig.detectColorRequirement(storyContent)
		if detectedColor != nil {
			request.Color = *detectedColor
			request.Prompt = buildMangaPrompt(GenerateMangaArgs{
				Prompt: request.Prompt,
				Style:  request.Style,
				Layout: request.Layout,
				Color:  &request.Color,
			})
			msg := fmt.Sprintf("Auto-detected %s preference from story file.", (func() string {
				if request.Color {
					return "COLOR"
				}
				return "B&W"
			})())
			fmt.Fprintln(os.Stderr, "DEBUG -", msg)
			ig.logToDisk(msg)
		}
	}

	memoryPath, err := ig.mh.GetMemoryFilePath(storyFileResult.FilePath)
	if err != nil {
		return ImageGenerationResponse{
			Success: false,
			Message: "Failed to get memory file path",
			Error:   err.Error(),
		}, nil
	}

	storyDir := filepath.Dir(storyFileResult.FilePath)
	promptsDir := filepath.Join(storyDir, "prompts")
	ig.fh.EnsureDirectory(promptsDir)

	globalContext, pages := splitStory(storyContent)

	// Filter pages
	var pagesToProcess []MangaPage
	if request.Page != "" {
		targets := regexp.MustCompile(`[,&]|\s+and\s+`).Split(request.Page, -1)
		var cleanedTargets []string
		for _, t := range targets {
			t = strings.TrimSpace(strings.ToLower(t))
			if t != "" {
				cleanedTargets = append(cleanedTargets, t)
			}
		}

		for _, p := range pages {
			headerLower := strings.ToLower(p.Header)
			headerNumbers := regexp.MustCompile(`\d+`).FindAllString(headerLower, -1)

			matched := false
			for _, target := range cleanedTargets {
				isNumeric := regexp.MustCompile(`^\d+$`).MatchString(target)
				if isNumeric {
					for _, num := range headerNumbers {
						if num == target {
							matched = true
							break
						}
					}
				} else {
					if strings.Contains(headerLower, target) {
						matched = true
					}
				}
				if matched {
					break
				}
			}

			if matched {
				pagesToProcess = append(pagesToProcess, p)
			}
		}

		if len(pagesToProcess) == 0 {
			return ImageGenerationResponse{
				Success: false,
				Message: fmt.Sprintf("Page(s) \"%s\" not found in story file.", request.Page),
				Error:   "Page not found",
			}, nil
		}
		fmt.Fprintf(os.Stderr, "DEBUG - Filtering for pages \"%s\". Found %d match(es).\n", request.Page, len(pagesToProcess))
	} else if request.StartPage != "" {
		target := strings.TrimSpace(strings.ToLower(request.StartPage))
		isNumeric := regexp.MustCompile(`^\d+$`).MatchString(target)

		startIndex := -1
		for idx, p := range pages {
			headerLower := strings.ToLower(p.Header)
			headerNumbers := regexp.MustCompile(`\d+`).FindAllString(headerLower, -1)
			matched := false
			if isNumeric {
				for _, num := range headerNumbers {
					if num == target {
						matched = true
						break
					}
				}
			} else {
				if strings.Contains(headerLower, target) {
					matched = true
				}
			}
			if matched {
				startIndex = idx
				break
			}
		}

		if startIndex != -1 {
			pagesToProcess = pages[startIndex:]
			fmt.Fprintf(os.Stderr, "DEBUG - Starting generation from \"%s\" (index %d). Processing %d pages.\n", request.StartPage, startIndex, len(pagesToProcess))
		} else {
			return ImageGenerationResponse{
				Success: false,
				Message: fmt.Sprintf("Start page \"%s\" not found in story file.", request.StartPage),
				Error:   "Start page not found",
			}, nil
		}
	} else {
		pagesToProcess = pages
	}

	// 3. Tag Show Mode
	if request.ShowTags {
		seenPagePaths := make(map[string]bool)
		var allTags []struct{ Tag, Label string }

		// Gather from globalContext
		paths := ig.extractImagePaths(globalContext)
		for _, imgPath := range paths {
			fileRes := ig.fh.FindInputFile(imgPath)
			if fileRes.Found && !seenPagePaths[fileRes.FilePath] {
				label := strings.TrimSuffix(filepath.Base(fileRes.FilePath), filepath.Ext(fileRes.FilePath))
				tag := regexp.MustCompile(`(?i)[_\s]+(portrait|sheet|reference|ref|far|view|env|environment)$`).ReplaceAllString(label, "")
				tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", " ")
				allTags = append(allTags, struct{ Tag, Label string }{tag, label})
				seenPagePaths[fileRes.FilePath] = true
			}
		}

		// Gather from pages
		for _, p := range pagesToProcess {
			paths = ig.extractImagePaths(p.Content)
			for _, imgPath := range paths {
				fileRes := ig.fh.FindInputFile(imgPath)
				if fileRes.Found && !seenPagePaths[fileRes.FilePath] {
					label := strings.TrimSuffix(filepath.Base(fileRes.FilePath), filepath.Ext(fileRes.FilePath))
					tag := regexp.MustCompile(`(?i)[_\s]+(portrait|sheet|reference|ref|far|view|env|environment)$`).ReplaceAllString(label, "")
					tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", " ")
					allTags = append(allTags, struct{ Tag, Label string }{tag, label})
					seenPagePaths[fileRes.FilePath] = true
				}
			}
		}

		if len(allTags) == 0 {
			return ImageGenerationResponse{
				Success: true,
				Message: "No tags or reference images found in story file.",
			}, nil
		}

		var tagOutput strings.Builder
		tagOutput.WriteString(fmt.Sprintf("[STRICT REFERENCE MAPPING FOR: %s]\n\n", request.StoryFile))
		for _, t := range allTags {
			tagOutput.WriteString(fmt.Sprintf("- Tag: \"%s\" matches Reference Image: \"%s\"\n", t.Tag, t.Label))
		}
		tagOutput.WriteString(`
[USAGE TIPS]
1. Use "(Tag: name)" in your panel descriptions to link an entity to its reference image.
2. Example: "Stan (Tag: stan) is sitting at his Singapore Office (Tag: singapore office env)."
3. The Tag name is case-insensitive but must match the spelling in the quotes above exactly.
4. For maximum consistency, use the Tag at the first mention of the entity in every panel.`)

		return ImageGenerationResponse{
			Success: true,
			Message: tagOutput.String(),
		}, nil
	}

	// 4. Character extraction and setup
	globalReferenceImages := []ReferenceImage{}
	loadedGlobalImagePaths := make(map[string]bool)

	// Explicit Character Image
	if request.CharacterImage != "" {
		charRes := ig.fh.FindInputFile(request.CharacterImage)
		if charRes.Found && !loadedGlobalImagePaths[charRes.FilePath] {
			b64, errRead := ig.fh.ReadImageAsBase64(charRes.FilePath)
			if errRead == nil {
				globalReferenceImages = append(globalReferenceImages, ReferenceImage{
					Data:       b64,
					MIMEType:   "image/png",
					SourcePath: request.CharacterImage,
				})
				loadedGlobalImagePaths[charRes.FilePath] = true
				globalContext += fmt.Sprintf("\n\n(See attached character reference: %s)", request.CharacterImage)
			}
		}
	} else {
		// Auto-detect / Generate Characters
		charsDir := filepath.Join(storyDir, "characters")
		ig.fh.EnsureDirectory(charsDir)

		charsList := parseCharacters(globalContext, storyDir, request.Color)
		for _, ch := range charsList {
			charRes := ig.fh.FindInputFile(ch.AbsPath)
			if charRes.Found && !loadedGlobalImagePaths[charRes.FilePath] {
				b64, errRead := ig.fh.ReadImageAsBase64(charRes.FilePath)
				if errRead == nil {
					globalReferenceImages = append(globalReferenceImages, ReferenceImage{
						Data:       b64,
						MIMEType:   "image/png",
						SourcePath: ch.AbsPath,
					})
					loadedGlobalImagePaths[charRes.FilePath] = true
					fmt.Fprintf(os.Stderr, "DEBUG - Loaded character reference for: %s\n", ch.Name)
				}
			} else if !charRes.Found && request.AutoGenerateCharacters {
				// Generate character sheet
				// 1. Generate B&W portrait
				fmt.Fprintf(os.Stderr, "DEBUG - Generating base B&W ref for: %s\n", ch.Name)
				layoutPrompt := "Square 1:1"
				if request.Layout == "strip" {
					layoutPrompt = "Wide Landscape 16:9"
				} else if request.Layout == "webtoon" {
					layoutPrompt = "Tall Vertical 9:16"
				} else if request.Layout == "single_page" {
					layoutPrompt = "Portrait 3:4"
				}

				viewsPrompt := "Composition: Single Full Body Standing Pose (3/4 View), centered."
				if request.Layout == "strip" {
					viewsPrompt = "Include the following views: Front view, 3/4 view, Profile view, and Back view. Order them: Front, 3/4, Profile, Back side-by-side."
				}

				bwPrompt := fmt.Sprintf(`Character Design Sheet (%s): %s. %s. 
%s
Ensure the character appeal and details strictly follow the guidelines provided in the user story file description.
Determine the character's age category based on the description and apply the corresponding anatomical guidelines:
- Child (approx 7-10): Head-to-body ratio 1:6, softer jawlines, shorter/slender limbs.
- Adult (approx 25-40): Standard 1:7.5 to 1:8 head-to-body ratio, defined bone structure, balanced muscle tone.
- Elder (70+): Slight natural spinal curvature (kyphosis), settled center of gravity, prominent joint articulation.
%s manga style, black and white, screentones, high quality line art.
Full body from head to toe (must include complete legs and shoes), neutral pose, white background. DO NOT SQUASH or compress the figure vertically. Ensure legs are long and anatomically correct. Avoid chibi, dwarf, or super-deformed proportions. Zoom out to fit the entire character within the frame. Leave ample white space margin around the character to prevent cropping of feet or head.`,
					layoutPrompt, ch.Name, ch.Desc, viewsPrompt, request.Style)

				bwFilename := fmt.Sprintf("%s_portrait.png", regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(strings.ToLower(ch.Name), "_"))

				// Call model
				respGen, errGen := ig.ai.Models.GenerateContent(
					ctx,
					ig.modelName,
					[]*genai.Content{{
						Role:  "user",
						Parts: []*genai.Part{{Text: bwPrompt}},
					}},
					&genai.GenerateContentConfig{
						ResponseModalities: []string{"IMAGE"},
						SafetySettings:     ig.getSafetySettings(),
					},
				)
				if errGen == nil && len(respGen.Candidates) > 0 && len(respGen.Candidates[0].Content.Parts) > 0 {
					for _, part := range respGen.Candidates[0].Content.Parts {
						var b64Data string
						if part.InlineData != nil && len(part.InlineData.Data) > 0 {
							b64Data = base64.StdEncoding.EncodeToString(part.InlineData.Data)
						}
						if b64Data != "" {
							fullP, errS := ig.fh.SaveImageFromBase64(b64Data, charsDir, bwFilename)
							if errS == nil {
								globalReferenceImages = append(globalReferenceImages, ReferenceImage{
									Data:       b64Data,
									MIMEType:   "image/png",
									SourcePath: fullP,
								})
								loadedGlobalImagePaths[fullP] = true
								fmt.Fprintf(os.Stderr, "DEBUG - Generated base B&W portrait: %s\n", fullP)

								// If color mode, generate color portrait from B&W
								if request.Color {
									colorFilename := fmt.Sprintf("%s_portrait_color.png", regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(strings.ToLower(ch.Name), "_"))
									colorPrompt := fmt.Sprintf(`Character Design Sheet (%s): %s. %s. 
GENERATE IN FULL COLOR. Vibrant colors, detailed shading.
Use the attached B&W image as the STRICT reference for line art and design. Colorize it accurately.
%s
Ensure the character appeal and details strictly follow the guidelines provided in the user story file description.
Full body from head to toe (must include complete legs and shoes), neutral pose, white background. DO NOT SQUASH or compress the figure vertically. Ensure legs are long and anatomically correct. Avoid chibi, dwarf, or super-deformed proportions. Zoom out to fit the entire character within the frame. Leave ample white space margin around the character to prevent cropping of feet or head.`,
										layoutPrompt, ch.Name, ch.Desc, viewsPrompt)

									imgBytes, _ := base64.StdEncoding.DecodeString(b64Data)

									respCol, errCol := ig.ai.Models.GenerateContent(
										ctx,
										ig.modelName,
										[]*genai.Content{{
											Role: "user",
											Parts: []*genai.Part{
												{Text: colorPrompt},
												{InlineData: &genai.Blob{Data: imgBytes, MIMEType: "image/png"}},
											},
										}},
										&genai.GenerateContentConfig{
											ResponseModalities: []string{"IMAGE"},
											SafetySettings:     ig.getSafetySettings(),
										},
									)

									if errCol == nil && len(respCol.Candidates) > 0 && len(respCol.Candidates[0].Content.Parts) > 0 {
										for _, cpart := range respCol.Candidates[0].Content.Parts {
											var cb64 string
											if cpart.InlineData != nil && len(cpart.InlineData.Data) > 0 {
												cb64 = base64.StdEncoding.EncodeToString(cpart.InlineData.Data)
											}
											if cb64 != "" {
												colP, errCS := ig.fh.SaveImageFromBase64(cb64, charsDir, colorFilename)
												if errCS == nil {
													globalReferenceImages = append(globalReferenceImages, ReferenceImage{
														Data:       cb64,
														MIMEType:   "image/png",
														SourcePath: colP,
													})
													loadedGlobalImagePaths[colP] = true
													fmt.Fprintf(os.Stderr, "DEBUG - Generated color portrait: %s\n", colP)
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 5. Environment background generation
	envsDir := filepath.Join(storyDir, "environments")
	ig.fh.EnsureDirectory(envsDir)
	envsList := parseEnvironments(globalContext, storyDir)
	for _, env := range envsList {
		envRes := ig.fh.FindInputFile(env.AbsPath)
		if envRes.Found && !loadedGlobalImagePaths[envRes.FilePath] {
			b64, errRead := ig.fh.ReadImageAsBase64(envRes.FilePath)
			if errRead == nil {
				globalReferenceImages = append(globalReferenceImages, ReferenceImage{
					Data:       b64,
					MIMEType:   "image/png",
					SourcePath: envRes.FilePath,
				})
				loadedGlobalImagePaths[envRes.FilePath] = true
				fmt.Fprintf(os.Stderr, "DEBUG - Loaded environment background for: %s\n", env.Name)
			}
		} else if !envRes.Found && request.AutoGenerateEnvironments {
			fmt.Fprintf(os.Stderr, "DEBUG - Generating background: %s\n", env.Name)
			envPrompt := fmt.Sprintf(`Background Concept Art: %s. %s. 
Clean environment design, detailed layout, established architecture. White background or natural sky/scenery context.
%s
No characters. High quality, professional concept illustration.`,
				env.Name, env.Desc, (func() string {
					if request.Color {
						return "FULL COLOR, vibrant background lighting."
					}
					return "Black and white manga background style with screen tones."
				})())

			respEnv, errEnv := ig.ai.Models.GenerateContent(
				ctx,
				ig.modelName,
				[]*genai.Content{{
					Role:  "user",
					Parts: []*genai.Part{{Text: envPrompt}},
				}},
				&genai.GenerateContentConfig{
					ResponseModalities: []string{"IMAGE"},
					SafetySettings:     ig.getSafetySettings(),
				},
			)
			if errEnv == nil && len(respEnv.Candidates) > 0 && len(respEnv.Candidates[0].Content.Parts) > 0 {
				for _, part := range respEnv.Candidates[0].Content.Parts {
					var b64Data string
					if part.InlineData != nil && len(part.InlineData.Data) > 0 {
						b64Data = base64.StdEncoding.EncodeToString(part.InlineData.Data)
					}
					if b64Data != "" {
						fullP, errS := ig.fh.SaveImageFromBase64(b64Data, envsDir, env.Filename)
						if errS == nil {
							globalReferenceImages = append(globalReferenceImages, ReferenceImage{
								Data:       b64Data,
								MIMEType:   "image/png",
								SourcePath: fullP,
							})
							loadedGlobalImagePaths[fullP] = true
							fmt.Fprintf(os.Stderr, "DEBUG - Generated background reference: %s\n", fullP)
						}
					}
				}
			}
		}
	}

	if request.EnvironmentGenerationOnly {
		return ImageGenerationResponse{
			Success: true,
			Message: fmt.Sprintf("Environment background concepts generated in %s", envsDir),
		}, nil
	}

	var generatedFiles []string
	var previousPagePath string
	var firstError string

	// Iterate through pages
	for _, page := range pagesToProcess {
		fmt.Fprintf(os.Stderr, "DEBUG - Processing %s...\n", page.Header)
		ig.logToDisk(fmt.Sprintf("--- Processing %s ---", page.Header))
		ig.logToDisk("Phase 1 (Art) Started")

		memory, errCheck := ig.mh.CheckMemory(memoryPath, page.Header)
		if errCheck != nil {
			fmt.Fprintf(os.Stderr, "DEBUG - Failed to check memory: %v\n", errCheck)
		}

		var existingArtPath string

		// Check Phase 2 (Final) Memory
		if request.Page == "" && memory.Phase2 != "" {
			msg := fmt.Sprintf("Memory: Found PASSED Phase 2 file: %s. SKIPPING generation.", memory.Phase2)
			fmt.Fprintln(os.Stderr, "DEBUG -", msg)
			ig.logToDisk("✅ " + msg)
			previousPagePath = memory.Phase2
			generatedFiles = append(generatedFiles, memory.Phase2)
			continue
		}

		// Check Phase 1 Memory
		var phase1Prompt string
		if request.TwoPhase && memory.Phase1 != "" {
			existingArtPath = memory.Phase1
			phase1Prompt = memory.Phase1Prompt
			msg := fmt.Sprintf("Memory: Found PASSED Phase 1 file: %s. Resuming from Phase 2.", memory.Phase1)
			fmt.Fprintln(os.Stderr, "DEBUG -", msg)
			ig.logToDisk("✅ " + msg)
		}

		// File-based Phase 1 Art Resume Check (Fallback)
		if existingArtPath == "" && request.TwoPhase {
			safeHeader := ig.fh.GetSanitizedBaseName(page.Header)
			filenamePattern := fmt.Sprintf("manga_page_%s_phase_1.png", safeHeader)
			latestPath := ig.fh.FindLatestFile(strings.TrimSuffix(filenamePattern, ".png"))
			if latestPath != "" {
				existingArtPath = latestPath
				msg := fmt.Sprintf("Found existing Phase 1 art on disk: %s. Resuming from Phase 2.", latestPath)
				fmt.Fprintln(os.Stderr, "DEBUG -", msg)
				ig.logToDisk(msg)
			}
		}

		// Extract page-specific references
		pageImagePaths := ig.extractImagePaths(page.Content)
		pageReferenceImages := []ReferenceImage{}
		loadedPageImagePaths := make(map[string]bool)

		for _, imgPath := range pageImagePaths {
			fileRes := ig.fh.FindInputFile(imgPath)
			if fileRes.Found && !loadedGlobalImagePaths[fileRes.FilePath] && !loadedPageImagePaths[fileRes.FilePath] {
				b64, errRead := ig.fh.ReadImageAsBase64(fileRes.FilePath)
				if errRead == nil {
					pageReferenceImages = append(pageReferenceImages, ReferenceImage{
						Data:       b64,
						MIMEType:   "image/png",
						SourcePath: fileRes.FilePath,
					})
					loadedPageImagePaths[fileRes.FilePath] = true
				}
			}
		}

		// Combine references
		allReferences := []ReferenceImage{}
		allReferences = append(allReferences, globalReferenceImages...)
		allReferences = append(allReferences, pageReferenceImages...)

		// Add previous page if color/continuity reference
		if previousPagePath != "" {
			b64Prev, errRead := ig.fh.ReadImageAsBase64(previousPagePath)
			if errRead == nil {
				allReferences = append(allReferences, ReferenceImage{
					Data:       b64Prev,
					MIMEType:   "image/png",
					SourcePath: previousPagePath,
				})
			}
		}

		// Phase 1 - Art Generation Loop
		var phase1Warning string
		var phase1File string

		maxRetries := request.RetryCount
		if maxRetries <= 0 {
			maxRetries = 3
		}

		if existingArtPath != "" {
			phase1File = existingArtPath
		} else {
			// Generate Phase 1 Art
			attempt := 1
			for attempt <= maxRetries {
				fmt.Fprintf(os.Stderr, "DEBUG - Phase 1 generation attempt %d/%d for %s\n", attempt, maxRetries, page.Header)

				// Read failures from memory
				failures, _ := ig.mh.GetFailures(memoryPath, page.Header)
				var failFeedbacks []string
				for _, r := range failures.Reasons {
					failFeedbacks = append(failFeedbacks, fmt.Sprintf("- Flaw in previous attempt: %s", r))
				}

				feedbackPrompt := ""
				if len(failFeedbacks) > 0 {
					feedbackPrompt = "\n[CRITICAL FEEDBACK FOR CORRECTION]\nFix these issues in this attempt:\n" + strings.Join(failFeedbacks, "\n") + "\n"
				}

				// Build prompt
				var rawPrompt string
				if request.Prompt != "" {
					rawPrompt = request.Prompt
				} else {
					rawPrompt = page.Content
				}

				var layoutPrompt string
				if request.Layout == "strip" {
					layoutPrompt = "Wide Landscape 16:9"
				} else if request.Layout == "webtoon" {
					layoutPrompt = "Tall Vertical 9:16"
				} else if request.Layout == "single_page" {
					layoutPrompt = "Portrait 3:4"
				} else {
					layoutPrompt = "Square 1:1"
				}

				mangaStylePrompt := fmt.Sprintf("%s manga style", request.Style)
				if request.Color {
					mangaStylePrompt = fmt.Sprintf("FULL COLOR %s style", request.Style)
				}

				bwClause := "screentones, black and white, detailed ink work"
				if request.Color && !request.TwoPhase {
					bwClause = "vibrant colors, digital coloring, anime cell shading"
				}

				scenePrompt := fmt.Sprintf(`Manga Page Layout (%s): %s. 
Style: %s, %s.
DO NOT include any speech bubbles, thought bubbles, empty bubbles, or narrative caption text boxes containing letters. Speech bubbles and dialogue lettering are strictly forbidden in this phase.
Ensure the layout matches the panels and actions described in the script.
%s
%s`, layoutPrompt, rawPrompt, mangaStylePrompt, bwClause, feedbackPrompt, ratioInstruction)

				phase1Prompt = scenePrompt

				// Save Phase 1 Prompt before generation
				safeHeader := ig.fh.GetSanitizedBaseName(page.Header)
				promptFile := filepath.Join(promptsDir, fmt.Sprintf("page_%s_phase1_attempt_%d.txt", safeHeader, attempt))
				_ = ig.fh.SaveTextFile(promptFile, scenePrompt)

				parts := []*genai.Part{
					{Text: scenePrompt},
				}

				// Attach references
				for _, ref := range allReferences {
					refBytes, errDec := base64.StdEncoding.DecodeString(ref.Data)
					if errDec == nil {
						label := filepath.Base(ref.SourcePath)
						parts = append(parts, &genai.Part{Text: fmt.Sprintf("Reference (%s):", label)})
						parts = append(parts, &genai.Part{InlineData: &genai.Blob{Data: refBytes, MIMEType: ref.MIMEType}})
					}
				}

				respGen, errGen := ig.ai.Models.GenerateContent(
					ctx,
					ig.artModel,
					[]*genai.Content{{
						Role:  "user",
						Parts: parts,
					}},
					&genai.GenerateContentConfig{
						ResponseModalities: []string{"IMAGE"},
						SafetySettings:     ig.getSafetySettings(),
					},
				)

				if errGen != nil {
					firstError = errGen.Error()
					fmt.Fprintf(os.Stderr, "DEBUG - Phase 1 generation failed: %v\n", errGen)
					attempt++
					continue
				}

				var tempPhase1B64 string
				if len(respGen.Candidates) > 0 && len(respGen.Candidates[0].Content.Parts) > 0 {
					for _, part := range respGen.Candidates[0].Content.Parts {
						if part.InlineData != nil && len(part.InlineData.Data) > 0 {
							tempPhase1B64 = base64.StdEncoding.EncodeToString(part.InlineData.Data)
						} else if part.Text != "" && isValidBase64ImageData(part.Text) {
							tempPhase1B64 = part.Text
						}
						if tempPhase1B64 != "" {
							break
						}
					}
				}

				if tempPhase1B64 == "" {
					firstError = "No image data returned from art model"
					attempt++
					continue
				}

				outputDir := ig.fh.EnsureOutputDirectory()
				tempFilename := fmt.Sprintf("manga_page_%s_phase_1.png", safeHeader)
				if !request.TwoPhase {
					tempFilename = fmt.Sprintf("manga_page_%s.png", safeHeader)
				}

				tempPath, errSave := ig.fh.SaveImageFromBase64(tempPhase1B64, outputDir, tempFilename)
				if errSave != nil {
					firstError = errSave.Error()
					attempt++
					continue
				}

				// QA Auto-Review Loop
				var review ReviewResponse
				var errReview error
				p1Retry := 1
				for p1Retry <= 3 {
					review, errReview = ig.reviewGeneratedImage(ctx, tempPath, allReferences, request, request.TwoPhase)
					if errReview == nil && review.TotalScore > 0 {
						break
					}
					fmt.Fprintf(os.Stderr, "DEBUG - Phase 1 Review suspicious (Score 0). Retrying review (%d/3)...\n", p1Retry)
					time.Sleep(1 * time.Second)
					p1Retry++
				}

				if errReview != nil || !review.Pass {
					errorMsg := fmt.Sprintf("Phase 1 Review FAILED for %s (Attempt %d). Score: %.1f. Reason: %s", page.Header, attempt, review.TotalScore, review.Reason)
					fmt.Fprintln(os.Stderr, "DEBUG -", errorMsg)
					ig.logToDisk(fmt.Sprintf("Phase 1 (Art) FAILED: %s", review.Reason))
					ig.logToDisk(errorMsg)

					// Update failure memory
					_ = ig.mh.UpdateMemory(memoryPath, page.Header, 1, "FAILED", MemoryUpdateData{
						Reason:     review.Reason,
						FailedPath: tempPath,
					})

					if attempt >= maxRetries {
						return ImageGenerationResponse{
							Success: false,
							Message: fmt.Sprintf("Failed at Phase 1 after %d attempts.", maxRetries),
							Error:   errorMsg,
						}, nil
					}
					attempt++
					continue
				}

				// Passed!
				fmt.Fprintln(os.Stderr, "DEBUG - ✅ Phase 1 Passed Review. Proceeding to Phase 2.")
				ig.logToDisk("✅ Phase 1 Passed Review. Proceeding to Phase 2.")
				phase1File = tempPath
				phase1Warning = review.Reason

				// Save memory
				_ = ig.mh.UpdateMemory(memoryPath, page.Header, 1, "PASSED", MemoryUpdateData{
					FilePath: tempPath,
					Prompt:   scenePrompt,
				})

				ig.logGeneration(ig.artModel, []string{tempPath}, fmt.Sprintf("Phase 1 for %s", page.Header))
				break
			}
		}

		ig.logToDisk("Phase 1 (Art) Completed")

		// Phase 2 - Dialogue bubble lettering and coloring
		if request.TwoPhase {
			ig.logToDisk("Phase 2 (Final) Started")

			finalPath, errP2 := ig.addTextToMangaPage(
				ctx,
				phase1File,
				page.Content,
				page.Header,
				allReferences,
				request.Color,
				phase1Prompt,
				phase1Warning,
				promptsDir,
				request.ReviewModel,
			)

			if errP2 != nil {
				ig.logToDisk(fmt.Sprintf("Phase 2 (Final) FAILED: %v", errP2))
				firstError = errP2.Error()
				continue
			}

			// Update memory for Phase 2
			_ = ig.mh.UpdateMemory(memoryPath, page.Header, 2, "PASSED", MemoryUpdateData{
				FilePath: finalPath,
			})

			previousPagePath = finalPath
			generatedFiles = append(generatedFiles, finalPath)
			ig.logToDisk("Phase 2 (Final) Completed")
		} else {
			// Single phase generated page is final
			previousPagePath = phase1File
			generatedFiles = append(generatedFiles, phase1File)
			// Save memory for Phase 2 as passed
			_ = ig.mh.UpdateMemory(memoryPath, page.Header, 2, "PASSED", MemoryUpdateData{
				FilePath: phase1File,
			})
			ig.logToDisk("Single Phase Completed")
		}

		// Trigger preview
		ig.handlePreview([]string{previousPagePath}, request)
	}

	if len(generatedFiles) > 0 {
		return ImageGenerationResponse{
			Success:        true,
			Message:        fmt.Sprintf("Manga page(s) generated successfully: %d page(s)", len(generatedFiles)),
			GeneratedFiles: generatedFiles,
		}, nil
	}

	return ImageGenerationResponse{
		Success: false,
		Message: "Failed to generate manga pages",
		Error:   firstError,
	}, nil
}

type MangaPage struct {
	Header  string
	Content string
}

func splitStory(storyContent string) (string, []MangaPage) {
	pageRegex := regexp.MustCompile(`(?i)(?:^|\n)((?:#{1,3}\s*Page\s*\d+|Page\s*\d+:)[^\n]*)`)

	matches := pageRegex.FindAllStringSubmatchIndex(storyContent, -1)
	if len(matches) == 0 {
		return "", []MangaPage{{Header: "Single Page", Content: storyContent}}
	}

	globalContext := storyContent[:matches[0][0]]
	var pages []MangaPage

	for idx, match := range matches {
		headerStart := match[2]
		headerEnd := match[3]
		header := strings.TrimSpace(storyContent[headerStart:headerEnd])

		contentStart := match[1]
		contentEnd := len(storyContent)
		if idx+1 < len(matches) {
			contentEnd = matches[idx+1][0]
		}
		content := storyContent[contentStart:contentEnd]

		pages = append(pages, MangaPage{Header: header, Content: content})
	}

	return strings.TrimSpace(globalContext), pages
}

func (ig *ImageGenerator) reviewGeneratedImage(
	ctx context.Context,
	generatedImagePath string,
	references []ReferenceImage,
	options ImageGenerationRequest,
	isPhase1 bool,
) (ReviewResponse, error) {
	minScore := ig.parseScoreParameter(options.MinScore, 8)
	minLikeness := ig.parseScoreParameter(options.MinLikeness, 9)
	minStory := ig.parseScoreParameter(options.MinStory, 7)
	minContinuity := ig.parseScoreParameter(options.MinContinuity, 7)
	minLettering := ig.parseScoreParameter(options.MinLettering, 9.5)
	minNoBubbles := ig.parseScoreParameter(options.MinNoBubbles, 9.5)
	reviewModel := options.ReviewModel
	if reviewModel == "" {
		reviewModel = ig.textModel
	}
	
	isColor := options.Color

	fmt.Fprintf(os.Stderr, "DEBUG - Auto-Reviewing generated image using %s (Min Total Score: %.1f)...\n", reviewModel, minScore)

	var characterRefs []ReferenceImage
	for _, ref := range references {
		if strings.Contains(ref.SourcePath, "characters/") || strings.Contains(ref.SourcePath, "_portrait") {
			characterRefs = append(characterRefs, ref)
		}
	}

	if len(characterRefs) == 0 {
		fmt.Fprintln(os.Stderr, "DEBUG - No character references found for review. Skipping.")
		return ReviewResponse{Pass: true, TotalScore: 10, Reason: "No references to check against."}, nil
	}

	var prompt strings.Builder
	prompt.WriteString(`You are a strict Quality Assurance AI for a manga production pipeline.
Task: Compare the "Generated Image" with the provided "Reference Images" (including "Previous Page Reference" if available) AND the "Story Description".
`)

	if isPhase1 {
		prompt.WriteString("PHASE: ART PHASE (No Speech Bubbles allowed. Note: Captions, sound effects, and background text are PERMITTED.)\n")
		prompt.WriteString("TARGET FORMAT: DRAFT ART (Accepts Black & White OR Color). Do NOT penalize B&W art even if the final target is Color. Focus on Line Art.\n")
	} else {
		prompt.WriteString("PHASE: FINAL PHASE (Lettering/Color included)\n")
		if isColor {
			prompt.WriteString("TARGET FORMAT: FULL COLOR. (If the Story Description says \"black and white\", IGNORE IT. The user requested COLOR.)\n")
		} else {
			prompt.WriteString("TARGET FORMAT: BLACK AND WHITE (Manga Style).\n")
		}
	}

	prompt.WriteString(fmt.Sprintf("\nSTORY DESCRIPTION / CONTEXT:\n\"%s\"\n", options.Prompt))

	prompt.WriteString(`
EVALUATION CRITERIA (Scored out of 100% each):
1. [CRITICAL] Character Design & Identity (100% max): Does the character look EXACTLY like the main Character Reference sheet? Check eye shape, hair style/bangs, facial structure, BODY TYPE, and COSTUME. Identity and Design must be 100% consistent with the Ground Truth Character Sheet.
2. [CRITICAL] Continuity (100% max): Does the overall visual style (line weight, shading, lighting) match the "Previous Page Reference"?
3. [CRITICAL] `)
	if isPhase1 {
		prompt.WriteString("NO SPEECH BUBBLES")
	} else {
		prompt.WriteString("Lettering & Text")
	}
	prompt.WriteString(" (100% max):\n")
	if isPhase1 {
		prompt.WriteString("   Does the image contain any round SPEECH BUBBLES or THOUGHT BUBBLES? These are forbidden. NOTE: Rectangular caption boxes, sound effects (SFX), and incidental text on objects/walls are ALL PERMITTED. Only actual dialogue bubbles (usually white ovals with tails) are a failure.\n")
	} else {
		prompt.WriteString("   Are ALL speech bubbles and caption boxes filled with the CORRECT text from the Story Description? 1. Check for GIBBERISH. 2. Check for MISSING DIALOGUE. 3. Check for ALTERED TEXT. The text in the image must match the script WORD-FOR-WORD. Paraphrasing is a FAILURE.\n")
	}

	prompt.WriteString(`4. [CRITICAL] Story Accuracy & Panel Layout (100% max): Does the image match the provided Story Description (actions, emotions, items) AND the PANEL LAYOUT? If the script describes multiple panels (e.g., "Panel 1", "Panel 2"), the image MUST show that structure. If it describes a splash page, it must be one large image.

TOTAL POSSIBLE SCORE: 400%.
10/10 quality in all categories equals 400%.

SCORING RUBRIC (Be Extremely Strict):
- 100%: Perfect match. Identical face, hair, costume, and layout. `)
	if isPhase1 {
		prompt.WriteString("No speech bubbles.\n")
	} else {
		prompt.WriteString("All text matches script WORD-FOR-WORD (no missing lines, no typos).\n")
	}
	prompt.WriteString("- 90%: Excellent likeness and layout. ")
	if isPhase1 {
		prompt.WriteString("No speech bubbles. ")
	} else {
		prompt.WriteString("No empty bubbles, maybe 1 minor typo. ")
	}
	prompt.WriteString(`Only pixel-level differences.
- 70-80%: Recognizable, but minor costume or layout details are off. FACE MUST MATCH.
- 50-60%: Looks like a different person, wrong outfit, wrong panel count, OR `)
	if isPhase1 {
		prompt.WriteString("Contains speech bubbles")
	} else {
		prompt.WriteString("contains GIBBERISH, MISSING DIALOGUE, or PARAPHRASED text")
	}
	prompt.WriteString(`.
- 10-40%: Completely wrong person, wrong layout, or text is missing entirely.

CRITICAL PENALTIES:
`)
	if isPhase1 {
		prompt.WriteString("- [STRICT] SPEECH BUBBLES: If ANY round speech bubble or thought bubble is found, the no_bubbles_score MUST be below 40%. Captions, boxes, and SFX are allowed.\n")
	} else {
		prompt.WriteString("- [STRICT] TEXT ACCURACY: If ANY text is missing, gibberish, or paraphrased (different words than script), the lettering_score MUST be below 40%.\n- [STRICT] NO DUPLICATES: If the same line of dialogue appears twice (e.g. once in a good bubble, once in a bad/ghost bubble), the lettering_score MUST be below 60%.\n")
	}

	if isColor {
		prompt.WriteString(`- [CONDITIONAL] COLOR CONSISTENCY: 
   - FOR PHASE 1: IGNORE ALL COLOR MISMATCHES.
   - FOR PHASE 2 / SINGLE PHASE: Compare hair/eye/costume colors. 
     - If the Reference is B&W, IGNORE color differences. 
     - If the Reference IS Color, strict consistency is required (likeness_score < 60% if mismatched).
   - IF "Previous Page Reference" is Color but this page is generated in B&W (or vice-versa) AND this contradicts the user's explicit request, penalize continuity_score. 
   - HOWEVER, if the user requested Color/B&W explicitly, DO NOT penalize just because the previous page was different.
`)
	} else {
		prompt.WriteString("- [LENIENT] COLOR: If the image is in Color despite \"TARGET FORMAT: BLACK AND WHITE\", DO NOT FAIL. Use your judgment: if the color looks good and matches the scene, ACCEPT IT. Only penalize story_score (max -10%) if the color actively ruins the mood.\n")
	}

	prompt.WriteString(`- [STRICT] PANEL LAYOUT: Count the panels. If the script asks for a 3-panel stack but the image is a single splash, the story_score MUST be below 50%.
- If the visual style (shading/art style) clashes with the "Previous Page Reference" (IGNORING Color vs B&W differences if explicit format changed), the continuity_score MUST be below 80%.
- [STRICT] FACIAL IDENTITY: Compare the eyes, nose, and jawline. If it looks like a different person from the Character Reference, the likeness_score MUST be below 60%.
- [STRICT] HAIR: The hairstyle (bangs, length, volume) must match the Main Reference exactly. If the hair is different, the likeness_score MUST be below 60%.
- [STRICT] CLOTHING: The costume DESIGN must be consistent with the reference UNLESS the Story Description or Global Context explicitly describes a different outfit. If the BASE DESIGN changes without reason, the likeness_score MUST be below 60%.
- If the image contradicts the Story Description (e.g. "fat" in text but "slim" in image), the story_score MUST be below 50%.

Strictly enforce identity and `)
	if isPhase1 {
		prompt.WriteString("ABSENCE of structural lettering elements (bubbles/captions)")
	} else {
		prompt.WriteString("FULL TEXT completion")
	}
	prompt.WriteString(". Do not allow \"style\" to excuse facial drift or ")
	if isPhase1 {
		prompt.WriteString("bubbles")
	} else {
		prompt.WriteString("empty bubbles")
	}
	prompt.WriteString(".\n")

	bubbleKey := "lettering_score"
	if isPhase1 {
		bubbleKey = "no_bubbles_score"
	}

	prompt.WriteString(fmt.Sprintf(`
Output strictly in JSON format:
{
    "likeness_score": number, // 0-100
    "continuity_score": number, // 0-100
    "%[1]s": number, // 0-100
    "story_score": number, // 0-100
    "total_score": number, // 0-400
    "reason": "string", // Specific feedback on what is wrong.
    "pass": boolean // true if total_score >= %[2].1f AND likeness_score >= %[3].1f AND %[1]s >= %[4].1f AND story_score >= %[5].1f AND continuity_score >= %[6].1f
}`, bubbleKey, minScore*40, minLikeness*10, (func() float64 {
		if isPhase1 {
			return minNoBubbles * 10
		}
		return minLettering * 10
	})(), minStory*10, minContinuity*10))

	parts := []*genai.Part{
		{Text: prompt.String()},
		{Text: "Generated Image:"},
		{InlineData: &genai.Blob{Data: []byte{}, MIMEType: "image/png"}},
	}
	generatedBytes, err := os.ReadFile(generatedImagePath)
	if err != nil {
		return ReviewResponse{}, err
	}
	parts[2].InlineData.Data = generatedBytes

	for _, ref := range references {
		refBytes, errRead := base64.StdEncoding.DecodeString(ref.Data)
		if errRead == nil {
			label := filepath.Base(ref.SourcePath)
			parts = append(parts, &genai.Part{Text: fmt.Sprintf("Reference (%s):", label)})
			parts = append(parts, &genai.Part{InlineData: &genai.Blob{Data: refBytes, MIMEType: ref.MIMEType}})
		}
	}

	resp, err := ig.ai.Models.GenerateContent(
		ctx,
		reviewModel,
		[]*genai.Content{{Role: "user", Parts: parts}},
		&genai.GenerateContentConfig{
			ResponseModalities: []string{"TEXT"},
			ResponseMIMEType:   "application/json",
			SafetySettings:     ig.getSafetySettings(),
			Temperature:        genai.Ptr(float32(0.2)),
			TopP:               genai.Ptr(float32(0.95)),
		},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG - Auto-review failed (error calling model): %v\n", err)
		return ReviewResponse{Pass: true, TotalScore: 0, Reason: "Review failed to execute."}, nil
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return ReviewResponse{Pass: true, TotalScore: 0, Reason: "No response from review model"}, nil
	}

	responseText := resp.Candidates[0].Content.Parts[0].Text
	cleanedText := regexp.MustCompile(`(?s)\s*\x60\x60\x60json\s*|\s*\x60\x60\x60\s*`).ReplaceAllString(responseText, "")
	cleanedText = strings.TrimSpace(cleanedText)

	var revResp ReviewResponse
	err = json.Unmarshal([]byte(cleanedText), &revResp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG - JSON Parse Error: %v. Raw Text: %s\n", err, responseText)
		return ReviewResponse{Pass: true, TotalScore: 0, Reason: "Review failed to parse JSON response."}, nil
	}

	return revResp, nil
}

func (ig *ImageGenerator) addTextToMangaPage(
	ctx context.Context,
	imagePath string,
	storyContent string,
	pageHeader string,
	references []ReferenceImage,
	isColor bool,
	artPrompt string,
	phase1Correction string,
	promptsDir string,
	textModel string,
) (string, error) {
	usedTextModel := textModel
	if usedTextModel == "" {
		usedTextModel = ig.textModel
	}

	fmt.Fprintf(os.Stderr, "DEBUG - Phase 2: Adding text %sto %s using %s...\n", (func() string {
		if isColor {
			return "and color "
		}
		return ""
	})(), pageHeader, usedTextModel)

	if artPrompt != "" {
		fmt.Fprintf(os.Stderr, "DEBUG - Phase 2 received visual context from Phase 1 prompt (%d chars).\n", len(artPrompt))
	}

	goDialogRegex := regexp.MustCompile(`(?m)(?:^|\n)\s*[-*]?\s*\*?([a-zA-Z0-9 '\-]+?)\*?\s*:\s*["“](.*?)["”]`)
	matches := goDialogRegex.FindAllStringSubmatch(storyContent, -1)
	var dialogueList []string
	for _, m := range matches {
		dialogueList = append(dialogueList, fmt.Sprintf("%s: \"%s\"", strings.TrimSpace(m[1]), strings.TrimSpace(m[2])))
	}

	var checklist strings.Builder
	if len(dialogueList) > 0 {
		checklist.WriteString("\n[MANDATORY DIALOGUE CHECKLIST]\nYou MUST include the following dialogue lines EXACTLY as written. Do not skip any:\n")
		for idx, line := range dialogueList {
			checklist.WriteString(fmt.Sprintf("%d. %s\n", idx+1, line))
		}
		checklist.WriteString("Double-check that ALL lines above are present in the final image.\n")
	}

	var prompt strings.Builder
	prompt.WriteString(`You are a professional manga editor and artist. 
Task: Add dialogue bubbles and text from the provided story script onto the attached manga page art. 
`)
	if isColor {
		prompt.WriteString("Also, COLORIZE the page using the provided reference images.\n")
	}

	prompt.WriteString(fmt.Sprintf("\nSTORY SCRIPT FOR %s:\n\"%s\"\n%s\n", pageHeader, storyContent, checklist.String()))

	if artPrompt != "" {
		prompt.WriteString(fmt.Sprintf("\n[VISUAL CONTEXT FROM ART PHASE]\nThe attached \"Generated Image\" was created with this description. Use it to understand the intended colors, lighting, and atmosphere:\n\"%s\"\n", artPrompt))
	}

	if phase1Correction != "" {
		prompt.WriteString(fmt.Sprintf("\n[CORRECTION INSTRUCTION]\nThe input image has a flaw: \"%s\".\nFix this by placing a correct dialogue bubble OVER the erroneous artifact (e.g. cover the empty/bad bubble) or ensuring the final composition hides it.\n", phase1Correction))
	}

	prompt.WriteString(`
INSTRUCTIONS:
1. **Create Dialogue Bubbles**: Analyze the script and the panels in the attached art. Create speech bubbles and caption boxes that fit the dialogue and composition.
2. **Sequential Mapping**: Map the dialogue lines in the script to the bubbles you create in reading order.
3. **Lettering**: Render ALL dialogue and captions into the bubbles/boxes. Use professional manga lettering style. Ensure text is centered and legible.
4. **Verification**: EVERY line of dialogue and EVERY caption from the script MUST be present.
5. **NO HALLUCINATIONS**: Do NOT add any random text, gibberish, or text not found in the script. All text in the image must come strictly from the provided story script. Check for typos.
   - **STRICT PROHIBITION**: Do not invent new lines of dialogue. Do not paraphrase. Do not add "narration" that isn't in the script.
   - If a bubble exists in the art but has no matching text in the script, LEAVE IT EMPTY or REMOVE IT. Do not fill it with hallucinated text.
6. **CLEANUP & DEDUPLICATION**: The input art might contain "ghost" bubbles, faint text, or artifacts from the drawing phase. You MUST COVER or OVERPAINT these with your new, correct bubbles or artwork edits. Ensure there is NO DUPLICATE TEXT (e.g., the same line appearing twice). The final image must only contain the clean, sharp text from the script.
7. **NO METADATA**: Do NOT write the Page Number or Page Title ("`)
	prompt.WriteString(pageHeader)
	prompt.WriteString(`") anywhere on the image. Only the dialogue and narrative captions from the script.`)

	if isColor {
		prompt.WriteString(`
5. **Colorization**: The attached page is in Black and White. You MUST colorize it.
   - Use the attached "Reference Image" portraits to match the EXACT hair, eye, skin, and costume colors for characters.
   - Use the "Far View" environment reference for background colors.
   - Ensure consistent color palettes across the entire page.`)
	} else {
		prompt.WriteString(`
5. **Maintain Style**: The attached page art is in Black and White. Keep the final output in professional Black and White manga style (screentones, ink, etc.). 
   - **STRICTLY BLACK AND WHITE**: Do NOT add any color. Even if the attached Reference Images are in color, you MUST convert them to Black & White/Grayscale for this page.
   - The final image must look like a printed manga page (Ink on Paper).`)
	}

	prompt.WriteString(`
6. **Art Integrity & Correction**: 
   - **TREAT INPUT AS SKETCH**: Consider the attached Input Art as a "Rough Layout". You are NOT just coloring it; you are FINISHING it.
   - **FACE CORRECTION**: The input sketch may have "off-model" faces. You MUST look at the attached "Reference Image" and **REDRAW** the eyes, hair, and jawline to match the Reference exactly.
   - **PRIORITY**: Reference Image Identity > Input Sketch details. If the sketch doesn't look like the character, FIX IT.
   - The final image must have perfect on-model characters.
7. Return the final high-quality image.`)

	if promptsDir != "" {
		safeHeader := ig.fh.GetSanitizedBaseName(pageHeader)
		promptFile := filepath.Join(promptsDir, fmt.Sprintf("page_%s_phase2.txt", safeHeader))
		_ = ig.fh.SaveTextFile(promptFile, prompt.String())
	}

	parts := []*genai.Part{
		{Text: prompt.String()},
		{InlineData: &genai.Blob{Data: []byte{}, MIMEType: "image/png"}},
	}
	imgBytes, errReadFile := os.ReadFile(imagePath)
	if errReadFile != nil {
		return "", errReadFile
	}
	parts[1].InlineData.Data = imgBytes

	for _, ref := range references {
		refBytes, errRead := base64.StdEncoding.DecodeString(ref.Data)
		if errRead == nil {
			label := filepath.Base(ref.SourcePath)
			label = strings.TrimSuffix(label, filepath.Ext(label))
			parts = append(parts, &genai.Part{Text: fmt.Sprintf("Reference image for: \"%s\"", label)})
			parts = append(parts, &genai.Part{InlineData: &genai.Blob{Data: refBytes, MIMEType: ref.MIMEType}})
		}
	}

	resp, err := ig.ai.Models.GenerateContent(
		ctx,
		usedTextModel,
		[]*genai.Content{{Role: "user", Parts: parts}},
		&genai.GenerateContentConfig{
			ResponseModalities: []string{"IMAGE"},
			SafetySettings:     ig.getSafetySettings(),
		},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG - Phase 2 FAILED: %v\n", err)
		return "", err
	}

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		for _, part := range resp.Candidates[0].Content.Parts {
			var b64Data string
			if part.InlineData != nil && len(part.InlineData.Data) > 0 {
				b64Data = base64.StdEncoding.EncodeToString(part.InlineData.Data)
			} else if part.Text != "" && isValidBase64ImageData(part.Text) {
				b64Data = part.Text
			}

			if b64Data != "" {
				dir := filepath.Dir(imagePath)
				cleanBase := strings.TrimSuffix(filepath.Base(imagePath), ".png")
				cleanBase = regexp.MustCompile(`_phase_1$`).ReplaceAllString(cleanBase, "")
				newFileName := fmt.Sprintf("%s_final.png", cleanBase)
				newPath := filepath.Join(dir, newFileName)
				
				_, errSave := ig.fh.SaveImageFromBase64(b64Data, dir, newFileName)
				if errSave != nil {
					return "", errSave
				}

				ig.logGeneration(usedTextModel, []string{newPath}, fmt.Sprintf("Phase 2 (Lettering) for %s. Total: %d references.", pageHeader, len(references)))
				fmt.Fprintf(os.Stderr, "DEBUG - Phase 2 SUCCESS: Saved final image to %s. (Used %d references)\n", newPath, len(references))
				return newPath, nil
			}
		}
	}

	return "", fmt.Errorf("no image data returned from text model")
}

type CharacterInfo struct {
	Name    string
	Desc    string
	AbsPath string
}

func parseCharacters(globalContext string, storyDir string, color bool) []CharacterInfo {
	var chars []CharacterInfo
	lines := strings.Split(globalContext, "\n")
	
	sanitize := func(name string) string {
		reg := regexp.MustCompile(`[^a-z0-9]`)
		return reg.ReplaceAllString(strings.ToLower(name), "_")
	}

	charsDir := filepath.Join(storyDir, "characters")
	charListRegex := regexp.MustCompile(`^\s*[\*\-]\s*\*\*([^\*\n]+?)(?::)?\*\*(.*)$`)
	tagRegex := regexp.MustCompile(`(?i)\((?:Tag|tag):\s*([^)]+)\)`)

	var lastParentHeader string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			headerText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if strings.HasPrefix(trimmed, "###") && !strings.Contains(strings.ToLower(headerText), "page") {
				charName := strings.TrimSuffix(headerText, ":")
				charName = strings.TrimSpace(charName)
				var extractedTag string
				tagMatch := tagRegex.FindStringSubmatch(charName)
				if len(tagMatch) > 1 {
					extractedTag = strings.TrimSpace(tagMatch[1])
					charName = strings.TrimSpace(tagRegex.ReplaceAllString(charName, ""))
				}

				parentLower := strings.ToLower(lastParentHeader)
				validParent := false
				for _, kw := range []string{"character", "cast", "person", "role", "protagonist", "antagonist"} {
					if strings.Contains(parentLower, kw) {
						validParent = true
						break
					}
				}
				
				invalidKeyword := false
				nameLower := strings.ToLower(charName)
				for _, kw := range []string{"rule", "prompt", "instruction", "setting", "format", "optimization", "export"} {
					if strings.Contains(nameLower, kw) {
						invalidKeyword = true
						break
					}
				}

				if (validParent || lastParentHeader == "") && !invalidKeyword && charName != "" && strings.ToLower(charName) != "character style" && strings.ToLower(charName) != "characters" {
					var descLines []string
					for j := i + 1; j < len(lines); j++ {
						nextTrim := strings.TrimSpace(lines[j])
						if strings.HasPrefix(nextTrim, "#") {
							break
						}
						descLines = append(descLines, nextTrim)
					}
					charDesc := strings.TrimSpace(strings.Join(descLines, " "))
					
					safeName := sanitize(charName)
					if extractedTag != "" {
						safeName = sanitize(extractedTag)
					}
					
					charFilename := fmt.Sprintf("%s_portrait.png", safeName)
					if color {
						charFilename = fmt.Sprintf("%s_portrait_color.png", safeName)
					}

					chars = append(chars, CharacterInfo{
						Name:    charName,
						Desc:    charDesc,
						AbsPath: filepath.Join(charsDir, charFilename),
					})
				}
			} else {
				lastParentHeader = headerText
			}
			continue
		}

		charMatch := charListRegex.FindStringSubmatch(line)
		if len(charMatch) > 0 {
			charName := strings.TrimSpace(charMatch[1])
			charDesc := strings.TrimSpace(charMatch[2])
			charDesc = strings.TrimPrefix(charDesc, ":")
			charDesc = strings.TrimSpace(charDesc)

			var extractedTag string
			tagMatch := tagRegex.FindStringSubmatch(charName)
			if len(tagMatch) > 1 {
				extractedTag = strings.TrimSpace(tagMatch[1])
				charName = strings.TrimSpace(tagRegex.ReplaceAllString(charName, ""))
			}

			parentLower := strings.ToLower(lastParentHeader)
			validParent := false
			for _, kw := range []string{"character", "cast", "person", "role", "protagonist", "antagonist"} {
				if strings.Contains(parentLower, kw) {
					validParent = true
					break
				}
			}

			if !validParent && lastParentHeader != "" {
				for k := i - 1; k >= 0; k-- {
					if strings.HasPrefix(strings.TrimSpace(lines[k]), "#") {
						break
					}
					kTrim := strings.ToLower(strings.TrimSpace(lines[k]))
					if strings.Contains(kTrim, "character") || strings.Contains(kTrim, "cast") || strings.Contains(kTrim, "role") {
						validParent = true
						break
					}
				}
			} else if lastParentHeader == "" {
				validParent = true
			}

			if !validParent {
				continue
			}

			{
				var descLines []string
				if charDesc != "" {
					descLines = append(descLines, charDesc)
				}
				parentIndent := len(line) - len(strings.TrimLeft(line, " \t"))
				for j := i + 1; j < len(lines); j++ {
					nextLine := lines[j]
					nextTrim := strings.TrimSpace(nextLine)
					if nextTrim == "" {
						continue
					}
					if charListRegex.MatchString(nextLine) {
						lineIndent := len(nextLine) - len(strings.TrimLeft(nextLine, " \t"))
						if lineIndent <= parentIndent {
							break
						}
					}
					if strings.HasPrefix(nextTrim, "#") {
						break
					}

					cleaned := nextTrim
					cleanReg := regexp.MustCompile(`^\s*[\*\-]\s*(?:\*\*(?:Role|Vibe|Personality|Visuals|Appearance|Traits|Outfit|Features):\*\*|(?:\*\*|__)?(?:Role|Vibe|Personality|Visuals|Appearance|Traits|Outfit|Features)(?:\*\*|__)?\s*:?)?\s*`)
					cleaned = cleanReg.ReplaceAllString(cleaned, "")
					cleaned = strings.TrimSpace(cleaned)
					if cleaned != "" {
						descLines = append(descLines, cleaned)
					}
					if len(descLines) > 40 {
						break
					}
				}
				if len(descLines) > 0 {
					charDesc = strings.Join(descLines, " ")
				}
			}

			safeName := sanitize(charName)
			if extractedTag != "" {
				safeName = sanitize(extractedTag)
			}

			charFilename := fmt.Sprintf("%s_portrait.png", safeName)
			if color {
				charFilename = fmt.Sprintf("%s_portrait_color.png", safeName)
			}

			chars = append(chars, CharacterInfo{
				Name:    charName,
				Desc:    charDesc,
				AbsPath: filepath.Join(charsDir, charFilename),
			})
		}
	}
	return chars
}

type EnvironmentInfo struct {
	Name     string
	Desc     string
	Filename string
	AbsPath  string
}

func parseEnvironments(globalContext string, storyDir string) []EnvironmentInfo {
	var envs []EnvironmentInfo
	lines := strings.Split(globalContext, "\n")
	envsDir := filepath.Join(storyDir, "environments")

	sanitize := func(name string) string {
		reg := regexp.MustCompile(`[^a-z0-9]`)
		return reg.ReplaceAllString(strings.ToLower(name), "_")
	}

	tagRegex := regexp.MustCompile(`(?i)\((?:Tag|tag):\s*([^)]+)\)`)
	envItemRegex := regexp.MustCompile(`^\s*[\*\-]\s*\*\*([^\*\n]+?)(?::)?\*\*(.*)$`)

	inEnvSection := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			headerText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			
			headerLower := strings.ToLower(headerText)
			inEnvSection = false
			for _, kw := range []string{"environment", "setting", "location"} {
				if strings.Contains(headerLower, kw) {
					inEnvSection = true
					break
				}
			}
			continue
		}

		trimmedLower := strings.ToLower(trimmed)
		if strings.Contains(trimmedLower, "**environment") || strings.Contains(trimmedLower, "**setting") || strings.Contains(trimmedLower, "**location") {
			inEnvSection = true
			continue
		}

		if inEnvSection {
			match := envItemRegex.FindStringSubmatch(line)
			if len(match) > 0 {
				envName := strings.TrimSpace(match[1])
				envDesc := strings.TrimSpace(match[2])
				envDesc = strings.TrimPrefix(envDesc, ":")
				envDesc = strings.TrimSpace(envDesc)

				if strings.ToLower(envName) == "characters" || strings.ToLower(envName) == "cast" {
					inEnvSection = false
					continue
				}

				var extractedTag string
				tagMatch := tagRegex.FindStringSubmatch(envName)
				if len(tagMatch) > 1 {
					extractedTag = strings.TrimSpace(tagMatch[1])
					envName = strings.TrimSpace(tagRegex.ReplaceAllString(envName, ""))
				}

				{
					var descLines []string
					if envDesc != "" {
						descLines = append(descLines, envDesc)
					}
					parentIndent := len(line) - len(strings.TrimLeft(line, " \t"))
					for j := i + 1; j < len(lines); j++ {
						nextLine := lines[j]
						nextTrim := strings.TrimSpace(nextLine)
						if nextTrim == "" {
							continue
						}
						if envItemRegex.MatchString(nextLine) {
							lineIndent := len(nextLine) - len(strings.TrimLeft(nextLine, " \t"))
							if lineIndent <= parentIndent {
								break
							}
						}
						if strings.HasPrefix(nextTrim, "#") {
							break
						}
						cleaned := nextTrim
						cleanReg := regexp.MustCompile(`^\s*[\*\-]\s*`)
						cleaned = cleanReg.ReplaceAllString(cleaned, "")
						cleaned = strings.TrimSpace(cleaned)
						if cleaned != "" {
							descLines = append(descLines, cleaned)
						}
					}
					if len(descLines) > 0 {
						envDesc = strings.Join(descLines, " ")
					}
				}

				safeName := sanitize(envName)
				if extractedTag != "" {
					safeName = sanitize(extractedTag)
				}

				filename := fmt.Sprintf("%s_env_far.png", safeName)
				absPath := filepath.Join(envsDir, filename)

				envs = append(envs, EnvironmentInfo{
					Name:     envName,
					Desc:     envDesc,
					Filename: filename,
					AbsPath:  absPath,
				})
			}
		}
	}
	return envs
}
