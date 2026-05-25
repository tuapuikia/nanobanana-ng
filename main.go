package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	transportType := flag.String("transport", "stdio", "Transport type (stdio or sse)")
	host := flag.String("host", "127.0.0.1", "Host for SSE transport")
	port := flag.Int("port", 8080, "Port for SSE transport")
	flag.Parse()

	// 1. Validate Authentication
	authConfig, err := ValidateAuthentication()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fh := NewFileHandler()
	mh := NewMemoryHandler(fh)

	ig, err := NewImageGenerator(authConfig, fh, mh)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize MCP Server
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "nanobanana-server",
		Version: "1.0.0",
	}, nil)

	// 3. Register Tools

	// --- generate_image ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_image",
		Description: "Generate single or multiple images from text prompts with style and variation options",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "The text prompt describing the image to generate",
				},
				"outputCount": map[string]any{
					"type":        "number",
					"description": "Number of variations to generate (1-8, default: 1)",
					"minimum":     1,
					"maximum":     8,
					"default":     1,
				},
				"styles": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "Array of artistic styles: photorealistic, watercolor, oil-painting, sketch, pixel-art, anime, vintage, modern, abstract, minimalist",
				},
				"variations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "Array of variation types: lighting, angle, color-palette, composition, mood, season, time-of-day",
				},
				"format": map[string]any{
					"type":        "string",
					"enum":        []string{"grid", "separate"},
					"description": "Output format: separate files or single grid image",
					"default":     "separate",
				},
				"seed": map[string]any{
					"type":        "number",
					"description": "Seed for reproducible variations",
				},
				"temperature": map[string]any{
					"type":        "number",
					"description": "Controls randomness in generation (0.0 to 1.0)",
				},
				"topP": map[string]any{
					"type":        "number",
					"description": "Controls diversity of generation (0.0 to 1.0)",
				},
				"preview": map[string]any{
					"type":        "boolean",
					"description": "Automatically open generated images in default viewer",
					"default":     false,
				},
			},
			"required": []string{"prompt"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GenerateImageArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		genReq := ImageGenerationRequest{
			Prompt:      toolArgs.Prompt,
			Mode:        "generate",
			Styles:      toolArgs.Styles,
			Variations:  toolArgs.Variations,
			Format:      toolArgs.Format,
			Seed:        toolArgs.Seed,
			Temperature: toolArgs.Temperature,
			TopP:        toolArgs.TopP,
		}
		if toolArgs.OutputCount != nil {
			genReq.OutputCount = *toolArgs.OutputCount
		} else {
			genReq.OutputCount = 1
		}
		if toolArgs.Preview != nil {
			genReq.Preview = *toolArgs.Preview
		}
		if toolArgs.NoPreview != nil {
			genReq.NoPreview = *toolArgs.NoPreview
		} else if toolArgs.NoPreviewK != nil {
			genReq.NoPreview = *toolArgs.NoPreviewK
		}

		res, err := ig.GenerateTextToImage(genReq)
		return formatResult(res, err)
	})

	// --- edit_image ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "edit_image",
		Description: "Edit an existing image based on a text prompt",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "The text prompt describing the edits to make",
				},
				"file": map[string]any{
					"type":        "string",
					"description": "The filename of the input image to edit",
				},
				"preview": map[string]any{
					"type":        "boolean",
					"description": "Automatically open generated images in default viewer",
					"default":     false,
				},
			},
			"required": []string{"prompt", "file"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs EditImageArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		genReq := ImageGenerationRequest{
			Prompt:     toolArgs.Prompt,
			InputImage: toolArgs.File,
			Mode:       "edit",
		}
		if toolArgs.Preview != nil {
			genReq.Preview = *toolArgs.Preview
		}
		if toolArgs.NoPreview != nil {
			genReq.NoPreview = *toolArgs.NoPreview
		} else if toolArgs.NoPreviewK != nil {
			genReq.NoPreview = *toolArgs.NoPreviewK
		}

		res, err := ig.EditImage(genReq)
		return formatResult(res, err)
	})

	// --- restore_image ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "restore_image",
		Description: "Restore or enhance an existing image",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "The text prompt describing the restoration to perform",
				},
				"file": map[string]any{
					"type":        "string",
					"description": "The filename of the input image to restore",
				},
				"preview": map[string]any{
					"type":        "boolean",
					"description": "Automatically open generated images in default viewer",
					"default":     false,
				},
			},
			"required": []string{"prompt", "file"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs RestoreImageArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		genReq := ImageGenerationRequest{
			Prompt:     toolArgs.Prompt,
			InputImage: toolArgs.File,
			Mode:       "restore",
		}
		if toolArgs.Preview != nil {
			genReq.Preview = *toolArgs.Preview
		}
		if toolArgs.NoPreview != nil {
			genReq.NoPreview = *toolArgs.NoPreview
		} else if toolArgs.NoPreviewK != nil {
			genReq.NoPreview = *toolArgs.NoPreviewK
		}

		res, err := ig.EditImage(genReq)
		return formatResult(res, err)
	})

	// --- generate_icon ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_icon",
		Description: "Generate app icons, favicons, and UI elements in multiple sizes and formats",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Description of the icon or UI element to generate",
				},
				"sizes": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "number",
					},
					"description": "Array of icon sizes in pixels (16, 32, 64, 128, 256, 512, 1024)",
				},
				"type": map[string]any{
					"type":        "string",
					"enum":        []string{"app-icon", "favicon", "ui-element"},
					"description": "Type of icon to generate",
					"default":     "app-icon",
				},
				"style": map[string]any{
					"type":        "string",
					"enum":        []string{"flat", "skeuomorphic", "minimal", "modern"},
					"description": "Visual style of the icon",
					"default":     "modern",
				},
				"format": map[string]any{
					"type":        "string",
					"enum":        []string{"png", "jpeg"},
					"description": "Output format",
					"default":     "png",
				},
				"background": map[string]any{
					"type":        "string",
					"description": "Background type: transparent, white, black, or color name",
					"default":     "transparent",
				},
				"corners": map[string]any{
					"type":        "string",
					"enum":        []string{"rounded", "sharp"},
					"description": "Corner style for app icons",
					"default":     "rounded",
				},
				"preview": map[string]any{
					"type":        "boolean",
					"description": "Automatically open generated images in default viewer",
					"default":     false,
				},
			},
			"required": []string{"prompt"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GenerateIconArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		// build prompt
		iconPrompt := buildIconPromptString(toolArgs)

		genReq := ImageGenerationRequest{
			Prompt:     iconPrompt,
			Mode:       "generate",
			FileFormat: toolArgs.Format,
		}
		if len(toolArgs.Sizes) > 0 {
			genReq.OutputCount = len(toolArgs.Sizes)
		} else {
			genReq.OutputCount = 1
		}
		if toolArgs.Preview != nil {
			genReq.Preview = *toolArgs.Preview
		}
		if toolArgs.NoPreview != nil {
			genReq.NoPreview = *toolArgs.NoPreview
		} else if toolArgs.NoPreviewK != nil {
			genReq.NoPreview = *toolArgs.NoPreviewK
		}

		res, err := ig.GenerateTextToImage(genReq)
		return formatResult(res, err)
	})

	// --- generate_pattern ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_pattern",
		Description: "Generate seamless patterns and textures for backgrounds and design elements",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Description of the pattern or texture to generate",
				},
				"size": map[string]any{
					"type":        "string",
					"description": "Pattern tile size (e.g., \"256x256\", \"512x512\")",
					"default":     "256x256",
				},
				"type": map[string]any{
					"type":        "string",
					"enum":        []string{"seamless", "texture", "wallpaper"},
					"description": "Type of pattern to generate",
					"default":     "seamless",
				},
				"style": map[string]any{
					"type":        "string",
					"enum":        []string{"geometric", "organic", "abstract", "floral", "tech"},
					"description": "Pattern style",
					"default":     "abstract",
				},
				"density": map[string]any{
					"type":        "string",
					"enum":        []string{"sparse", "medium", "dense"},
					"description": "Element density in the pattern",
					"default":     "medium",
				},
				"colors": map[string]any{
					"type":        "string",
					"enum":        []string{"mono", "duotone", "colorful"},
					"description": "Color scheme",
					"default":     "colorful",
				},
				"repeat": map[string]any{
					"type":        "string",
					"enum":        []string{"tile", "mirror"},
					"description": "Tiling method for seamless patterns",
					"default":     "tile",
				},
				"preview": map[string]any{
					"type":        "boolean",
					"description": "Automatically open generated images in default viewer",
					"default":     false,
				},
			},
			"required": []string{"prompt"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GeneratePatternArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		patternPrompt := buildPatternPromptString(toolArgs)

		genReq := ImageGenerationRequest{
			Prompt:      patternPrompt,
			OutputCount: 1,
			Mode:        "generate",
		}
		if toolArgs.Preview != nil {
			genReq.Preview = *toolArgs.Preview
		}
		if toolArgs.NoPreview != nil {
			genReq.NoPreview = *toolArgs.NoPreview
		} else if toolArgs.NoPreviewK != nil {
			genReq.NoPreview = *toolArgs.NoPreviewK
		}

		res, err := ig.GenerateTextToImage(genReq)
		return formatResult(res, err)
	})

	// --- generate_story ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_story",
		Description: "Generate a sequence of related images that tell a visual story or show a process",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Description of the story or process to visualize",
				},
				"steps": map[string]any{
					"type":        "number",
					"description": "Number of sequential images to generate (2-8)",
					"minimum":     2,
					"maximum":     8,
					"default":     4,
				},
				"type": map[string]any{
					"type":        "string",
					"enum":        []string{"story", "process", "tutorial", "timeline"},
					"description": "Type of sequence to generate",
					"default":     "story",
				},
				"style": map[string]any{
					"type":        "string",
					"enum":        []string{"consistent", "evolving"},
					"description": "Visual consistency across frames",
					"default":     "consistent",
				},
				"layout": map[string]any{
					"type":        "string",
					"enum":        []string{"separate", "grid", "comic"},
					"description": "Output layout format",
					"default":     "separate",
				},
				"transition": map[string]any{
					"type":        "string",
					"enum":        []string{"smooth", "dramatic", "fade"},
					"description": "Transition style between steps",
					"default":     "smooth",
				},
				"format": map[string]any{
					"type":        "string",
					"enum":        []string{"storyboard", "individual"},
					"description": "Output format",
					"default":     "individual",
				},
				"preview": map[string]any{
					"type":        "boolean",
					"description": "Automatically open generated images in default viewer",
					"default":     false,
				},
			},
			"required": []string{"prompt"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GenerateStoryArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		genReq := ImageGenerationRequest{
			Prompt:     toolArgs.Prompt,
			Mode:       "generate",
			Variations: []string{"sequence-step"},
		}
		if toolArgs.Steps != nil {
			genReq.OutputCount = *toolArgs.Steps
		} else {
			genReq.OutputCount = 4
		}
		if toolArgs.Preview != nil {
			genReq.Preview = *toolArgs.Preview
		}
		if toolArgs.NoPreview != nil {
			genReq.NoPreview = *toolArgs.NoPreview
		} else if toolArgs.NoPreviewK != nil {
			genReq.NoPreview = *toolArgs.NoPreviewK
		}

		storyArgs := StorySequenceArgs{
			Type:       toolArgs.Type,
			Style:      toolArgs.Style,
			Transition: toolArgs.Transition,
		}

		res, err := ig.GenerateStorySequence(genReq, storyArgs)
		return formatResult(res, err)
	})

	// --- generate_diagram ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_diagram",
		Description: "Generate technical diagrams, flowcharts, and architectural mockups",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Description of the diagram content and structure",
				},
				"type": map[string]any{
					"type":        "string",
					"enum":        []string{"flowchart", "architecture", "network", "database", "wireframe", "mindmap", "sequence"},
					"description": "Type of diagram to generate",
					"default":     "flowchart",
				},
				"style": map[string]any{
					"type":        "string",
					"enum":        []string{"professional", "clean", "hand-drawn", "technical"},
					"description": "Visual style of the diagram",
					"default":     "professional",
				},
				"layout": map[string]any{
					"type":        "string",
					"enum":        []string{"horizontal", "vertical", "hierarchical", "circular"},
					"description": "Layout orientation",
					"default":     "hierarchical",
				},
				"complexity": map[string]any{
					"type":        "string",
					"enum":        []string{"simple", "detailed", "comprehensive"},
					"description": "Level of detail in the diagram",
					"default":     "detailed",
				},
				"colors": map[string]any{
					"type":        "string",
					"enum":        []string{"mono", "accent", "categorical"},
					"description": "Color scheme",
					"default":     "accent",
				},
				"annotations": map[string]any{
					"type":        "string",
					"enum":        []string{"minimal", "detailed"},
					"description": "Label and annotation level",
					"default":     "detailed",
				},
				"preview": map[string]any{
					"type":        "boolean",
					"description": "Automatically open generated images in default viewer",
					"default":     false,
				},
			},
			"required": []string{"prompt"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GenerateDiagramArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		diagPrompt := buildDiagramPromptString(toolArgs)

		genReq := ImageGenerationRequest{
			Prompt:      diagPrompt,
			OutputCount: 1,
			Mode:        "generate",
		}
		if toolArgs.Preview != nil {
			genReq.Preview = *toolArgs.Preview
		}
		if toolArgs.NoPreview != nil {
			genReq.NoPreview = *toolArgs.NoPreview
		} else if toolArgs.NoPreviewK != nil {
			genReq.NoPreview = *toolArgs.NoPreviewK
		}

		res, err := ig.GenerateTextToImage(genReq)
		return formatResult(res, err)
	})

	// --- generate_manga ---
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_manga",
		Description: "Generate a manga page or panel from a story script, with optional character reference for consistency",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Description of the scene or context (e.g., \"Battle scene in a futuristic city\")",
				},
				"story_file": map[string]any{
					"type":        "string",
					"description": "Path to the markdown/text file containing the story script",
				},
				"input_image": map[string]any{
					"type":        "string",
					"description": "Path to an existing manga page or image to edit/colorize",
				},
				"input_directory": map[string]any{
					"type":        "string",
					"description": "Path to a directory containing images to edit/colorize",
				},
				"character_image": map[string]any{
					"type":        "string",
					"description": "Path to an image of the main character to ensure consistency",
				},
				"reference_page": map[string]any{
					"type":        "string",
					"description": "Specific page number to use as a style reference (e.g., \"5\" or \"Page 5\")",
				},
				"style": map[string]any{
					"type":        "string",
					"enum":        []string{"shonen", "shojo", "seinen", "4-koma", "webtoon"},
					"description": "Manga style",
					"default":     "shonen",
				},
				"layout": map[string]any{
					"type":        "string",
					"enum":        []string{"single_page", "strip", "webtoon", "square"},
					"description": "Page layout",
					"default":     "square",
				},
				"page": map[string]any{
					"type":        "string",
					"description": "Specific page number or header to generate (e.g., \"2\", \"Page 2\")",
				},
				"from_page": map[string]any{
					"type":        "string",
					"description": "Start generation from this page number/header until the end (e.g., \"3\")",
				},
				"color": map[string]any{
					"type":        "boolean",
					"description": "Generate in full color instead of black and white",
					"default":     false,
				},
				"create_character": map[string]any{
					"type":        "boolean",
					"description": "Mode: Create character sheet from input image",
					"default":     false,
				},
				"preview": map[string]any{
					"type":        "boolean",
					"description": "Automatically open generated images in default viewer",
					"default":     false,
				},
				"generate_characters": map[string]any{
					"type":        "boolean",
					"description": "Auto-generate character sheets from story description",
					"default":     false,
				},
				"character_generation_only": map[string]any{
					"type":        "boolean",
					"description": "Only generate character sheets, skip manga page generation",
					"default":     false,
				},
				"generate_environments": map[string]any{
					"type":        "boolean",
					"description": "Auto-generate environment backgrounds from story description",
					"default":     false,
				},
				"environment_generation_only": map[string]any{
					"type":        "boolean",
					"description": "Only generate environment backgrounds, skip manga page generation",
					"default":     false,
				},
				"min_score": map[string]any{
					"type":        []string{"number", "string"},
					"description": "Minimum score (1-10) required for auto-review to pass. Supports natural language: ignore, lenient, balanced, strict, perfect.",
					"default":     8,
				},
				"min_likeness": map[string]any{
					"type":        []string{"number", "string"},
					"description": "Minimum likeness score required. Supports natural language: ignore, lenient, balanced, strict, perfect.",
				},
				"min_story": map[string]any{
					"type":        []string{"number", "string"},
					"description": "Minimum story accuracy score required. Supports natural language: ignore, lenient, balanced, strict, perfect.",
				},
				"min_continuity": map[string]any{
					"type":        []string{"number", "string"},
					"description": "Minimum continuity score required. Supports natural language: ignore, lenient, balanced, strict, perfect.",
				},
				"min_lettering": map[string]any{
					"type":        []string{"number", "string"},
					"description": "Minimum lettering score required (Phase 2). Supports natural language: ignore, lenient, balanced, strict, perfect.",
				},
				"min_no_bubbles": map[string]any{
					"type":        []string{"number", "string"},
					"description": "Minimum no-bubbles score required (Phase 1). Supports natural language: ignore, lenient, balanced, strict, perfect.",
				},
				"retry_count": map[string]any{
					"type":        "number",
					"description": "Maximum number of retries if auto-review fails",
					"default":     3,
				},
				"two_phase": map[string]any{
					"type":        "boolean",
					"description": "Use two-phase generation: Art then Text. Disabled by default.",
					"default":     false,
				},
				"use_memory": map[string]any{
					"type":        "boolean",
					"description": "Use saved successful prompts from memory. Disabled by default.",
					"default":     false,
				},
				"temperature": map[string]any{
					"type":        "number",
					"description": "Controls randomness in generation (0.0 to 1.0)",
				},
				"topP": map[string]any{
					"type":        "number",
					"description": "Controls diversity of generation (0.0 to 1.0)",
				},
				"show_tags": map[string]any{
					"type":        "boolean",
					"description": "Show the list of tags that will be generated and used for the story file",
					"default":     false,
				},
				"review_model": map[string]any{
					"type":        "string",
					"description": "Specific model name to use for the auto-review process (e.g., \"gemini-3.1-flash-image-preview\")",
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
		var toolArgs GenerateMangaArgs
		if err := json.Unmarshal(args, &toolArgs); err != nil {
			return nil, nil, err
		}

		genReq := ImageGenerationRequest{
			Prompt:         buildMangaPrompt(toolArgs),
			StoryFile:      toolArgs.StoryFile,
			InputImage:     toolArgs.InputImage,
			InputDirectory: toolArgs.InputDirectory,
			CharacterImage: toolArgs.CharacterImage,
			ReferencePage:  toolArgs.ReferencePage,
			Mode:           "manga",
			Page:           toolArgs.Page,
			StartPage:      toolArgs.FromPage,
			Layout:         toolArgs.Layout,
			Style:          toolArgs.Style,
			MinScore:       toolArgs.MinScore,
			MinLikeness:    toolArgs.MinLikeness,
			MinStory:       toolArgs.MinStory,
			MinContinuity:  toolArgs.MinContinuity,
			MinLettering:   toolArgs.MinLettering,
			MinNoBubbles:   toolArgs.MinNoBubbles,
			Temperature:    toolArgs.Temperature,
			TopP:           toolArgs.TopP,
			ReviewModel:    toolArgs.ReviewModel,
		}

		if toolArgs.Color != nil {
			genReq.Color = *toolArgs.Color
		}
		if toolArgs.CreateCharacter != nil {
			genReq.CreateCharacter = *toolArgs.CreateCharacter
		}
		if toolArgs.Preview != nil {
			genReq.Preview = *toolArgs.Preview
		}
		if toolArgs.NoPreview != nil {
			genReq.NoPreview = *toolArgs.NoPreview
		} else if toolArgs.NoPreviewK != nil {
			genReq.NoPreview = *toolArgs.NoPreviewK
		}

		if toolArgs.GenerateCharacters != nil {
			genReq.AutoGenerateCharacters = *toolArgs.GenerateCharacters
		}
		if toolArgs.CharacterGenerationOnly != nil {
			genReq.CharacterGenerationOnly = *toolArgs.CharacterGenerationOnly
		}
		if toolArgs.GenerateEnvironments != nil {
			genReq.AutoGenerateEnvironments = *toolArgs.GenerateEnvironments
		}
		if toolArgs.EnvironmentGenerationOnly != nil {
			genReq.EnvironmentGenerationOnly = *toolArgs.EnvironmentGenerationOnly
		}
		if toolArgs.IncludeText != nil {
			genReq.IncludeText = *toolArgs.IncludeText
		}
		if toolArgs.RetryCount != nil {
			genReq.RetryCount = *toolArgs.RetryCount
		} else {
			genReq.RetryCount = 3
		}
		if toolArgs.TwoPhase != nil {
			genReq.TwoPhase = *toolArgs.TwoPhase
		}
		if toolArgs.UseMemory != nil {
			genReq.UseMemory = *toolArgs.UseMemory
		}
		if toolArgs.ShowTags != nil {
			genReq.ShowTags = *toolArgs.ShowTags
		}

		res, err := ig.GenerateMangaPage(genReq)
		return formatResult(res, err)
	})

	// 4. Start Server Transport
	ctx := context.Background()
	switch *transportType {
	case "stdio":
		fmt.Fprintln(os.Stderr, "Nano Banana MCP Server running on stdio")
		if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
			log.Fatalf("Stdio server failed: %v", err)
		}
	case "sse":
		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return s
		}, nil)

		fmt.Fprintf(os.Stderr, "Nano Banana MCP Server running on SSE at %s:%d\n", *host, *port)
		if err := http.ListenAndServe(fmt.Sprintf("%s:%d", *host, *port), handler); err != nil {
			log.Fatalf("SSE server failed: %v", err)
		}
	default:
		log.Fatalf("Unknown transport type: %s", *transportType)
	}
}

func formatResult(res ImageGenerationResponse, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: err.Error()},
			},
		}, nil, nil
	}

	if !res.Success {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: res.Message + "\nError detail: " + res.Error},
			},
		}, nil, nil
	}

	var filesList string
	if len(res.GeneratedFiles) > 0 {
		filesList = "\n\nGenerated files:\n"
		for _, f := range res.GeneratedFiles {
			filesList += fmt.Sprintf("• %s\n", f)
		}
	} else {
		filesList = "\n\nGenerated files: None"
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: res.Message + filesList},
		},
	}, nil, nil
}

func buildIconPromptString(args GenerateIconArgs) string {
	basePrompt := args.Prompt
	if basePrompt == "" {
		basePrompt = "app icon"
	}
	iconType := args.Type
	if iconType == "" {
		iconType = "app-icon"
	}
	style := args.Style
	if style == "" {
		style = "modern"
	}
	background := args.Background
	if background == "" {
		background = "transparent"
	}
	corners := args.Corners
	if corners == "" {
		corners = "rounded"
	}

	prompt := fmt.Sprintf("%s, %s style %s", basePrompt, style, iconType)
	if iconType == "app-icon" {
		prompt += fmt.Sprintf(", %s corners", corners)
	}
	if background != "transparent" {
		prompt += fmt.Sprintf(", %s background", background)
	}
	prompt += ", clean design, high quality, professional"
	return prompt
}

func buildPatternPromptString(args GeneratePatternArgs) string {
	basePrompt := args.Prompt
	if basePrompt == "" {
		basePrompt = "abstract pattern"
	}
	patType := args.Type
	if patType == "" {
		patType = "seamless"
	}
	style := args.Style
	if style == "" {
		style = "abstract"
	}
	density := args.Density
	if density == "" {
		density = "medium"
	}
	colors := args.Colors
	if colors == "" {
		colors = "colorful"
	}
	size := args.Size
	if size == "" {
		size = "256x256"
	}

	prompt := fmt.Sprintf("%s, %s style %s pattern, %s density, %s colors", basePrompt, style, patType, density, colors)
	if patType == "seamless" {
		prompt += ", tileable, repeating pattern"
	}
	prompt += fmt.Sprintf(", %s tile size, high quality", size)
	return prompt
}

func buildDiagramPromptString(args GenerateDiagramArgs) string {
	basePrompt := args.Prompt
	if basePrompt == "" {
		basePrompt = "system diagram"
	}
	diagType := args.Type
	if diagType == "" {
		diagType = "flowchart"
	}
	style := args.Style
	if style == "" {
		style = "professional"
	}
	layout := args.Layout
	if layout == "" {
		layout = "hierarchical"
	}
	complexity := args.Complexity
	if complexity == "" {
		complexity = "detailed"
	}
	colors := args.Colors
	if colors == "" {
		colors = "accent"
	}
	annotations := args.Annotations
	if annotations == "" {
		annotations = "detailed"
	}

	prompt := fmt.Sprintf("%s, %s diagram, %s style, %s layout", basePrompt, diagType, style, layout)
	prompt += fmt.Sprintf(", %s level of detail, %s color scheme", complexity, colors)
	prompt += fmt.Sprintf(", %s annotations and labels", annotations)
	prompt += ", clean technical illustration, clear visual hierarchy"
	return prompt
}
