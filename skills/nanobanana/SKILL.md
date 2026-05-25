---
name: nanobanana
description: Gemini CLI extension for Nano Banana models - generate and manipulate images with text prompts.
---

# nanobanana Skill

This skill integrates the `nanobanana-ng` Go MCP server tools to allow Gemini CLI to generate, edit, restore, and manipulate images. It supports generating icons, patterns, diagrams, stories, and manga pages with strict visual consistency, count adherence, and QA auto-review.

## Roles & Responsibilities
- **Image Generation**: Create images based on text prompts with precise count adherence.
- **Image Editing & Restoration**: Modify existing images or restore damaged/low-quality photos.
- **Specialized Visuals**: Generate custom-tailored icons, seamless tiling patterns, and professional diagrams.
- **Story & Manga Creation**: Orchestrate multi-page visual narratives, maintaining strict consistency across frames in color, character design, style, and typography.

## Global Prompt Rules
- **Triggers**: Triggered by keywords relating to image generation, editing, icons, patterns, diagrams, stories, manga, or the **nanobanana** model.
- **Precise Count Adherence**: When Dad specifies `--count=N`, you MUST generate exactly N images (default is 1 if unspecified).
- **Style and Variation Compliance**: Always respect specified design options (`--styles` and `--variations`).
- **Visual Consistency**: For `/story` and `/manga`, keep character features, color palettes, environments, and line-art styles identical throughout all panels.

## Tool Selection & Priority
- **MCP Server**: `nanobanana`
- **Core Tools**:
  - `generate_image`: Generate new images from text prompts.
  - `edit_image`: Modify specific parts of an image.
  - `restore_image`: Restore damaged/defected images.
  - `generate_icon`: Generate app icons or favicons.
  - `generate_pattern`: Create seamless tiling patterns.
  - `generate_story`: Create a sequence of images representing a story.
  - `generate_diagram`: Produce standard technical or creative diagrams.
  - `generate_manga`: Create multi-page manga with automated QA review (likeness, layout, dialogue).

## Command Guidelines

### 1. Icon Generation (`/icon`)
- Generate clean, scalable designs suitable for target sizes.
- Ensure legibility at smaller dimensions.

### 2. Pattern Generation (`/pattern`)
- Guarantee perfect tiling without visible seams.
- Respect density options (sparse, medium, dense) and color limits.

### 3. Diagram Creation (`/diagram`)
- Follow professional diagramming conventions with clear labels.
- Ensure high readability of technical elements.

### 4. Image Editing (`/edit`)
- Apply edits naturally while keeping the original image's style and quality.

### 5. Image Restoration (`/restore`)
- Repair defects (scratches, tears) without modifying the original details or intent.

### 6. Manga Page Generation (`/manga`)
- Executes in Two-Phase mode when requested: Phase 1 generates panel art (no round dialogue bubbles), and Phase 2 colorizes and overlays dialogue text.
- Parses characters and environments from story metadata and runs auto-review loops.

## When to use this skill
- Requests to generate, draw, edit, or restore images.
- Creation of icons, patterns, flowcharts, or diagrams.
- Sequential storytelling requests or drawing manga pages/comics.
