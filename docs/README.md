# Keystone Core Documentation

This directory contains the source files for the Keystone Core documentation site, built with [Hugo](https://gohugo.io/) and the [Docsy](https://www.docsy.dev/) theme.

## Prerequisites

- **Hugo Extended**: Version 0.110.0 or later
  - macOS: `brew install hugo`
  - Linux: Download from [Hugo releases](https://github.com/gohugoio/hugo/releases)
  - Windows: `choco install hugo-extended`

- **Git**: For cloning the Docsy theme submodule

## Getting Started

### First-time Setup

1. **Clone the repository** (if you haven't already):
   ```bash
   git clone https://github.com/kscore/keystone-core.git
   cd keystone-core
   ```

2. **Initialize the Docsy theme submodule**:
   ```bash
   git submodule update --init --recursive
   ```

3. **Install npm dependencies**:
   ```bash
   cd docs
   npm install
   cd ..
   ```

   This installs:
   - Docsy theme dependencies (Bootstrap, Font Awesome)
   - Playwright for PDF generation
   - Build tools

### Building the Documentation

The documentation site is configured to build to `build/docs/` (see `publishDir` in `hugo.toml`).

**Build the site**:
```bash
cd docs
hugo
# Output will be in ../build/docs/
```

**Build and serve locally** (with live reload):
```bash
cd docs
hugo server
# Open http://localhost:1313 in your browser
```

**Build for production**:
```bash
cd docs
hugo --minify
# Output will be in ../build/docs/
```

## Directory Structure

```
docs/
├── hugo.toml           # Hugo configuration
├── content/            # Documentation content (Markdown)
│   └── en/             # English content
│       ├── _index.md   # Homepage
│       └── docs/       # Documentation sections
├── themes/             # Hugo themes
│   └── docsy/          # Docsy theme (git submodule)
├── static/             # Static assets (images, CSS, JS)
├── layouts/            # Custom Hugo layouts (overrides theme)
├── data/               # Data files for templates
├── archetypes/         # Content templates
└── assets/             # Asset pipeline files
```

## Writing Documentation

### Adding a New Page

1. Create a new Markdown file in the appropriate `content/en/docs/` subdirectory
2. Add front matter with title, weight, and description
3. Write content using Markdown and Hugo shortcodes

Example:
```markdown
---
title: "My New Page"
weight: 5
description: >
  A brief description of this page
---

## Section Title

Your content here...
```

### Organizing Content

- **Weight**: Controls the order in the navigation sidebar (lower numbers first)
- **LinkTitle**: Optional shorter title for navigation
- **Description**: Used in search results and page metadata

### Hugo Shortcodes

Docsy provides many useful shortcodes:

- `{{< blocks/cover >}}` - Hero section
- `{{< blocks/section >}}` - Content section
- `{{< blocks/feature >}}` - Feature grid
- `{{< alert >}}` - Callout boxes
- `{{< tabpane >}}` - Tabbed content

See [Docsy documentation](https://www.docsy.dev/docs/adding-content/shortcodes/) for more.

## Documentation Sections

The documentation is organized into the following sections:

1. **Getting Started** (Phases 1): Overview, Installation, Quick Start, Architecture
2. **Concepts** (Phase 2): Deep dives into each subsystem
3. **Tutorials** (Phase 3): Hands-on step-by-step guides
4. **Reference** (Phase 4): API, CLI, configuration reference
5. **Operations** (Phase 5): Deployment, monitoring, troubleshooting
6. **Community** (Phase 6): Contributing, roadmap, support
7. **Blog** (Phase 7): Release notes, announcements, case studies

## Theme Customization

To customize the Docsy theme:

1. **Override layouts**: Add files to `layouts/` matching the theme's structure
2. **Custom CSS**: Add files to `assets/scss/`
3. **Custom JS**: Add files to `assets/js/`
4. **Update params**: Edit `[params]` section in `hugo.toml`

## Troubleshooting

### Hugo Build Fails

- **Error**: `module "github.com/google/docsy" not found`
  - **Fix**: Run `git submodule update --init --recursive`

- **Error**: Font Awesome or Bootstrap errors
  - **Fix**: Install npm dependencies in `themes/docsy/`

### Theme Not Appearing

- **Check**: Verify `theme = "docsy"` in `hugo.toml`
- **Check**: Ensure Docsy submodule is initialized

### Slow Builds

- Use `hugo --gc` to clean up unused resources
- Run `hugo --cleanDestinationDir` to remove old files

## PDF Generation

Generate PDF versions of the documentation for offline use using Playwright (headless Chromium).

### Prerequisites

Node.js 18+ is required (same as Hugo/Docsy). If you followed the setup instructions above, all dependencies are already installed.

If you need to install Playwright browsers separately:
```bash
cd docs
npm run install-browsers
```

### Generate PDFs

**Using Make (recommended):**
```bash
make docs-pdf
```

This automatically:
- Installs npm dependencies if needed
- Installs Playwright browsers if needed
- Builds the Hugo site
- Generates all PDF files

**Manual approach:**
```bash
cd docs
npm install                 # Install dependencies (if not done)
npm run install-browsers    # Install Chromium (if not done)
npm run generate-pdfs       # Generate PDFs
```

### Generated PDFs

PDFs are created in `build/pdfs/`:
- `kscore-getting-started.pdf` - Getting Started guide
- `kscore-concepts.pdf` - Core Concepts documentation
- `kscore-reference.pdf` - Complete API/CLI reference
- `kscore-operations.pdf` - Operations guide
- `kscore-community.pdf` - Community guide
- `kscore-complete.pdf` - Complete documentation

### Browser Print (Alternative)

You can also use your browser's print function:
1. Start Hugo server: `hugo server`
2. Navigate to http://localhost:1313
3. Use browser print (Cmd+P / Ctrl+P)
4. Select "Save as PDF"

### Why Playwright?

We switched from wkhtmltopdf to Playwright because:
- **Actively maintained** - Regular updates from Microsoft
- **Better rendering** - Uses real Chromium engine
- **Modern CSS support** - Full support for flexbox, grid, modern features
- **Cross-platform** - Works consistently across all platforms
- **Already installed** - Part of npm dependencies for Docsy

## Contributing

When adding or updating documentation:

1. Test locally with `hugo server`
2. Check for broken links
3. Ensure proper formatting and front matter
4. Follow the existing structure and style
5. Update this README if adding new sections

## Resources

- [Hugo Documentation](https://gohugo.io/documentation/)
- [Docsy Theme Documentation](https://www.docsy.dev/docs/)
- [Keystone Core Design Documents](../DESIGN.md)
- [Keystone Core Epic Plans](../epics/)
