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
   git clone https://github.com/shawnbutts/keystone-core.git
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

Generate PDF versions of the documentation for offline use. We offer four approaches:

| Method | Quality | Dependencies | Best For |
|--------|---------|--------------|----------|
| **Containerized** (recommended) | Good/Professional | Docker or Podman only | No local deps, reproducible |
| **Paged.js + Playwright** | Good | Node.js only | Quick generation, CI/CD |
| **Simple Mode** | Basic | Node.js only | Fast generation, minimal features |
| **Pandoc + LaTeX** | Professional | Python, Pandoc, LaTeX | Book-quality output |

### Quick Start

**Default method (Paged.js + Playwright):**
```bash
make docs-pdf
```

This automatically:
- Installs npm dependencies if needed
- Installs Playwright browsers if needed
- Builds the Hugo site
- Generates professionally formatted PDFs with:
  - Cover pages with version and date
  - Table of contents
  - Running headers/footers
  - Proper page breaks
  - Print-optimized typography

### Containerized PDF Generation (Recommended)

Generate PDFs without installing any dependencies locally using Docker or Podman:

```bash
# Generate standard PDFs (Paged.js + Playwright)
make docs-pdf-container

# Generate book-quality PDFs (Pandoc + LaTeX)
make docs-pdf-book-container

# Generate all PDFs (both methods)
make docs-all-container
```

**Benefits:**
- No local dependencies required (no Hugo, Node.js, Pandoc, or LaTeX)
- Consistent builds across machines
- Works with both Docker and Podman (auto-detected)
- Isolated, reproducible environment

**First run will take longer** as it builds the container image (~2-3GB) with all dependencies:
- Hugo Extended
- Node.js 20 + npm
- Playwright + Chromium
- Pandoc + full LaTeX (texlive)
- Python Pygments

Subsequent runs are fast as they reuse the cached container image.

**Build the container image only:**
```bash
make docs-container-build
```

### PDF Generation Methods

#### Method 1: Paged.js + Playwright (Recommended)

Uses [Paged.js](https://pagedjs.org/) for CSS Paged Media support and Playwright for rendering.

```bash
# Using Make
make docs-pdf

# Manual
cd docs && npm run generate-pdfs

# Single section only
cd docs && node generate-pdfs.js --section=concepts
```

**Features:**
- Cover page with title, version, and generation date
- Table of contents (in complete documentation PDF)
- Running headers with section names
- Page numbers in footer
- Print-optimized CSS with proper margins
- Code syntax highlighting
- Automatic page breaks before chapters

#### Method 2: Simple Mode

Faster generation without Paged.js, but fewer formatting features.

```bash
make docs-pdf-simple
```

#### Method 3: Pandoc + LaTeX (Book Quality)

Generates professional book-quality PDFs using Pandoc and LaTeX. Best typography and formatting but requires additional dependencies.

**Prerequisites:**
```bash
# macOS
brew install pandoc
brew install --cask mactex   # ~4GB download

# Ubuntu/Debian
sudo apt install pandoc texlive-full

# Python (for syntax highlighting)
pip install Pygments
```

**Generate:**
```bash
make docs-pdf-book
```

**Features:**
- Professional LaTeX typography
- Proper table of contents with page numbers
- Chapter numbering
- Running headers from LaTeX
- Better handling of complex layouts
- Ideal for printed books

### Generated PDFs

All PDFs are created in `build/pdfs/`:

**Paged.js/Simple mode:**
- `keystone-core-getting-started.pdf` - Getting Started guide
- `keystone-core-concepts.pdf` - Core Concepts documentation
- `keystone-core-reference.pdf` - Complete API/CLI reference
- `keystone-core-operations.pdf` - Operations guide
- `keystone-core-community.pdf` - Community guide
- `keystone-core-complete.pdf` - Complete documentation

**Book mode (Pandoc + LaTeX):**
- `keystone-core-getting-started-book.pdf`
- `keystone-core-concepts-book.pdf`
- `keystone-core-reference-book.pdf`
- `keystone-core-operations-book.pdf`
- `keystone-core-community-book.pdf`
- `keystone-core-complete-book.pdf`

### Print CSS Customization

The print styling is defined in `static/css/print.css`. Key customizations:

- **Page size**: A4 with 25mm/20mm margins
- **Typography**: Source Sans Pro for body, Source Code Pro for code
- **Colors**: Blue accent (#2563eb) for headings and links
- **Headers/Footers**: Running section titles and page numbers
- **Code blocks**: Dark background with syntax highlighting

### Browser Print (Alternative)

You can also use your browser's print function:
1. Start Hugo server: `hugo server`
2. Navigate to http://localhost:1313
3. Use browser print (Cmd+P / Ctrl+P)
4. Select "Save as PDF"

### Why This Architecture?

We offer multiple PDF generation options because:

**Paged.js + Playwright:**
- **Node.js only** - No additional dependencies beyond Hugo setup
- **CSS Paged Media** - Standard-based approach for print styling
- **Good quality** - Cover pages, TOC, running headers
- **Fast** - Generates all PDFs in under 30 seconds

**Pandoc + LaTeX:**
- **Best typography** - LaTeX is the gold standard for document typesetting
- **Book features** - Proper chapters, indexes, cross-references
- **Professional output** - Suitable for printed documentation
- **Trade-off** - Requires ~4GB MacTeX installation

See also:
- [Paged.js](https://pagedjs.org/) - CSS Paged Media polyfill
- [pdf-book-exporter](https://github.com/rootsongjc/pdf-book-exporter) - Hugo book to PDF

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
