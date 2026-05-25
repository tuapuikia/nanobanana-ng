package main

type ImageGenerationRequest struct {
	Prompt                     string   `json:"prompt"`
	InputImage                 string   `json:"inputImage,omitempty"`
	InputDirectory             string   `json:"inputDirectory,omitempty"`
	OutputCount                int      `json:"outputCount,omitempty"`
	Mode                       string   `json:"mode"`
	Styles                     []string `json:"styles,omitempty"`
	Variations                 []string `json:"variations,omitempty"`
	Format                     string   `json:"format,omitempty"`
	FileFormat                 string   `json:"fileFormat,omitempty"`
	Seed                       *int     `json:"seed,omitempty"`
	Preview                    bool     `json:"preview,omitempty"`
	NoPreview                  bool     `json:"noPreview,omitempty"`
	StoryFile                  string   `json:"storyFile,omitempty"`
	CharacterImage             string   `json:"characterImage,omitempty"`
	ReferencePage              string   `json:"referencePage,omitempty"`
	Page                       string   `json:"page,omitempty"`
	StartPage                  string   `json:"startPage,omitempty"`
	Layout                     string   `json:"layout,omitempty"`
	Style                      string   `json:"style,omitempty"`
	Color                      bool     `json:"color,omitempty"`
	CreateCharacter            bool     `json:"createCharacter,omitempty"`
	AutoGenerateCharacters     bool     `json:"autoGenerateCharacters,omitempty"`
	CharacterGenerationOnly    bool     `json:"characterGenerationOnly,omitempty"`
	AutoGenerateEnvironments   bool     `json:"autoGenerateEnvironments,omitempty"`
	EnvironmentGenerationOnly  bool     `json:"environmentGenerationOnly,omitempty"`
	IncludeText                bool     `json:"includeText,omitempty"`
	MinScore                   any      `json:"minScore,omitempty"`
	MinLikeness                any      `json:"minLikeness,omitempty"`
	MinStory                   any      `json:"minStory,omitempty"`
	MinContinuity              any      `json:"minContinuity,omitempty"`
	MinLettering               any      `json:"minLettering,omitempty"`
	MinNoBubbles               any      `json:"minNoBubbles,omitempty"`
	RetryCount                 int      `json:"retryCount,omitempty"`
	TwoPhase                   bool     `json:"twoPhase,omitempty"`
	UseMemory                  bool     `json:"useMemory,omitempty"`
	Temperature                *float64 `json:"temperature,omitempty"`
	TopP                       *float64 `json:"topP,omitempty"`
	ShowTags                   bool     `json:"showTags,omitempty"`
	ReviewModel                string   `json:"reviewModel,omitempty"`
}

type ImageGenerationResponse struct {
	Success        bool     `json:"success"`
	Message        string   `json:"message"`
	GeneratedFiles []string `json:"generatedFiles,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type StorySequenceArgs struct {
	Type       string `json:"type,omitempty"`
	Style      string `json:"style,omitempty"`
	Transition string `json:"transition,omitempty"`
}

type AuthConfig struct {
	APIKey  string
	KeyType string
}

type FileSearchResult struct {
	Found         bool
	FilePath      string
	SearchedPaths []string
}

type FileSearchDirResult struct {
	Found   bool
	DirPath string
	Files   []string
}

type GenerateImageArgs struct {
	Prompt      string   `json:"prompt"`
	OutputCount *int     `json:"outputCount,omitempty"`
	Styles      []string `json:"styles,omitempty"`
	Variations  []string `json:"variations,omitempty"`
	Format      string   `json:"format,omitempty"`
	Seed        *int     `json:"seed,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
	Preview     *bool    `json:"preview,omitempty"`
	NoPreview   *bool    `json:"noPreview,omitempty"`
	NoPreviewK  *bool    `json:"no-preview,omitempty"`
}

type EditImageArgs struct {
	Prompt     string `json:"prompt"`
	File       string `json:"file"`
	Preview    *bool  `json:"preview,omitempty"`
	NoPreview  *bool  `json:"noPreview,omitempty"`
	NoPreviewK *bool  `json:"no-preview,omitempty"`
}

type RestoreImageArgs struct {
	Prompt     string `json:"prompt"`
	File       string `json:"file"`
	Preview    *bool  `json:"preview,omitempty"`
	NoPreview  *bool  `json:"noPreview,omitempty"`
	NoPreviewK *bool  `json:"no-preview,omitempty"`
}

type GenerateIconArgs struct {
	Prompt     string    `json:"prompt"`
	Sizes      []float64 `json:"sizes,omitempty"`
	Type       string    `json:"type,omitempty"`
	Style      string    `json:"style,omitempty"`
	Format     string    `json:"format,omitempty"`
	Background string    `json:"background,omitempty"`
	Corners    string    `json:"corners,omitempty"`
	Preview    *bool     `json:"preview,omitempty"`
	NoPreview  *bool     `json:"noPreview,omitempty"`
	NoPreviewK *bool     `json:"no-preview,omitempty"`
}

type GeneratePatternArgs struct {
	Prompt     string `json:"prompt"`
	Size       string `json:"size,omitempty"`
	Type       string `json:"type,omitempty"`
	Style      string `json:"style,omitempty"`
	Density    string `json:"density,omitempty"`
	Colors     string `json:"colors,omitempty"`
	Repeat     string `json:"repeat,omitempty"`
	Preview    *bool  `json:"preview,omitempty"`
	NoPreview  *bool  `json:"noPreview,omitempty"`
	NoPreviewK *bool  `json:"no-preview,omitempty"`
}

type GenerateStoryArgs struct {
	Prompt     string `json:"prompt"`
	Steps      *int   `json:"steps,omitempty"`
	Type       string `json:"type,omitempty"`
	Style      string `json:"style,omitempty"`
	Layout     string `json:"layout,omitempty"`
	Transition string `json:"transition,omitempty"`
	Format     string `json:"format,omitempty"`
	Preview    *bool  `json:"preview,omitempty"`
	NoPreview  *bool  `json:"noPreview,omitempty"`
	NoPreviewK *bool  `json:"no-preview,omitempty"`
}

type GenerateDiagramArgs struct {
	Prompt      string `json:"prompt"`
	Type        string `json:"type,omitempty"`
	Style       string `json:"style,omitempty"`
	Layout      string `json:"layout,omitempty"`
	Complexity  string `json:"complexity,omitempty"`
	Colors      string `json:"colors,omitempty"`
	Annotations string `json:"annotations,omitempty"`
	Preview     *bool  `json:"preview,omitempty"`
	NoPreview   *bool  `json:"noPreview,omitempty"`
	NoPreviewK  *bool  `json:"no-preview,omitempty"`
}

type GenerateMangaArgs struct {
	Prompt                    string   `json:"prompt,omitempty"`
	StoryFile                 string   `json:"story_file,omitempty"`
	InputImage                string   `json:"input_image,omitempty"`
	InputDirectory            string   `json:"input_directory,omitempty"`
	CharacterImage            string   `json:"character_image,omitempty"`
	ReferencePage             string   `json:"reference_page,omitempty"`
	Style                     string   `json:"style,omitempty"`
	Layout                    string   `json:"layout,omitempty"`
	Page                      string   `json:"page,omitempty"`
	FromPage                  string   `json:"from_page,omitempty"`
	Color                     *bool    `json:"color,omitempty"`
	CreateCharacter           *bool    `json:"create_character,omitempty"`
	Preview                   *bool    `json:"preview,omitempty"`
	NoPreview                 *bool    `json:"noPreview,omitempty"`
	NoPreviewK                *bool    `json:"no-preview,omitempty"`
	GenerateCharacters        *bool    `json:"generate_characters,omitempty"`
	CharacterGenerationOnly   *bool    `json:"character_generation_only,omitempty"`
	GenerateEnvironments      *bool    `json:"generate_environments,omitempty"`
	EnvironmentGenerationOnly *bool    `json:"environment_generation_only,omitempty"`
	IncludeText               *bool    `json:"include_text,omitempty"`
	MinScore                  any      `json:"min_score,omitempty"`
	MinLikeness               any      `json:"min_likeness,omitempty"`
	MinStory                  any      `json:"min_story,omitempty"`
	MinContinuity             any      `json:"min_continuity,omitempty"`
	MinLettering              any      `json:"min_lettering,omitempty"`
	MinNoBubbles              any      `json:"min_no_bubbles,omitempty"`
	RetryCount                *int     `json:"retry_count,omitempty"`
	TwoPhase                  *bool    `json:"two_phase,omitempty"`
	UseMemory                 *bool    `json:"use_memory,omitempty"`
	Temperature               *float64 `json:"temperature,omitempty"`
	TopP                      *float64 `json:"topP,omitempty"`
	ShowTags                  *bool    `json:"show_tags,omitempty"`
	ReviewModel               string   `json:"review_model,omitempty"`
}
