#!/bin/bash

# =============================================================================
# Keystone Core PDF Book Generator using pdf-book-exporter
#
# This script generates professional book-quality PDFs using Pandoc + LaTeX.
# It requires additional dependencies but produces the highest quality output.
#
# Prerequisites:
#   macOS:   brew install pandoc && brew install --cask mactex
#   Ubuntu:  sudo apt install pandoc texlive-full
#   Python:  pip install Pygments
#
# For detailed setup: https://github.com/rootsongjc/pdf-book-exporter
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
OUTPUT_DIR="$PROJECT_ROOT/build/pdfs"
CONTENT_DIR="$SCRIPT_DIR/content/en/docs"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}$1${NC}"; }
log_success() { echo -e "${GREEN}$1${NC}"; }
log_warn() { echo -e "${YELLOW}$1${NC}"; }
log_error() { echo -e "${RED}$1${NC}"; }

# Check prerequisites
check_dependencies() {
    log_info "Checking dependencies..."

    local missing=()

    if ! command -v pandoc &> /dev/null; then
        missing+=("pandoc")
    fi

    if ! command -v pdflatex &> /dev/null && ! command -v xelatex &> /dev/null; then
        missing+=("LaTeX (pdflatex or xelatex)")
    fi

    if ! python3 -c "import pygments" 2>/dev/null; then
        missing+=("Pygments (pip install Pygments)")
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        log_error "Missing dependencies:"
        for dep in "${missing[@]}"; do
            echo "  - $dep"
        done
        echo ""
        log_info "Installation instructions:"
        echo "  macOS:   brew install pandoc && brew install --cask mactex"
        echo "  Ubuntu:  sudo apt install pandoc texlive-full"
        echo "  Python:  pip install Pygments"
        echo ""
        log_warn "Falling back to standard PDF generator..."
        exec node "$SCRIPT_DIR/generate-pdfs.js"
    fi

    log_success "All dependencies found"
}

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Check if pdf-book-exporter is available
PDF_BOOK_EXPORTER=""
if [ -d "$PROJECT_ROOT/tools/pdf-book-exporter" ]; then
    PDF_BOOK_EXPORTER="$PROJECT_ROOT/tools/pdf-book-exporter"
elif command -v pdf-book-exporter &> /dev/null; then
    PDF_BOOK_EXPORTER="pdf-book-exporter"
fi

# Generate combined markdown for each section
generate_combined_markdown() {
    local section=$1
    local title=$2
    local output_file="$OUTPUT_DIR/${section}.md"

    log_info "Combining markdown for $title..."

    # Start with frontmatter
    cat > "$output_file" << EOF
---
title: "Keystone Core - ${title}"
author: "Keystone Core Team"
date: "$(date '+%Y-%m-%d')"
documentclass: report
papersize: a4
geometry: margin=2.5cm
fontsize: 11pt
toc: true
toc-depth: 3
colorlinks: true
linkcolor: blue
urlcolor: blue
header-includes:
  - \usepackage{fancyhdr}
  - \pagestyle{fancy}
  - \fancyhead[L]{Keystone Core}
  - \fancyhead[R]{${title}}
  - \fancyfoot[C]{\thepage}
---

EOF

    # Find and concatenate all markdown files in the section
    if [ -d "$CONTENT_DIR/$section" ]; then
        # Get _index.md first
        if [ -f "$CONTENT_DIR/$section/_index.md" ]; then
            echo "" >> "$output_file"
            # Skip frontmatter
            sed -n '/^---$/,/^---$/d; p' "$CONTENT_DIR/$section/_index.md" >> "$output_file"
        fi

        # Then get all other .md files sorted by weight
        find "$CONTENT_DIR/$section" -name "*.md" ! -name "_index.md" -type f | while read -r md_file; do
            echo "" >> "$output_file"
            echo "\\newpage" >> "$output_file"
            echo "" >> "$output_file"
            # Skip frontmatter
            sed -n '/^---$/,/^---$/d; p' "$md_file" >> "$output_file"
        done
    fi

    log_success "  Created $output_file"
}

# Generate PDF using Pandoc
generate_pdf_pandoc() {
    local input_file=$1
    local output_file=$2
    local title=$3

    log_info "Generating PDF: $title..."

    # Use XeLaTeX for better Unicode support
    pandoc "$input_file" \
        -o "$output_file" \
        --pdf-engine=xelatex \
        --highlight-style=tango \
        --toc \
        --toc-depth=3 \
        -V geometry:margin=2.5cm \
        -V documentclass=report \
        -V fontsize=11pt \
        -V colorlinks=true \
        -V linkcolor=blue \
        -V urlcolor=blue \
        2>/dev/null || {
            # Fallback to pdflatex if xelatex fails
            log_warn "  XeLaTeX failed, trying pdflatex..."
            pandoc "$input_file" \
                -o "$output_file" \
                --pdf-engine=pdflatex \
                --highlight-style=tango \
                --toc \
                --toc-depth=3 \
                -V geometry:margin=2.5cm \
                -V documentclass=report \
                -V fontsize=11pt \
                -V colorlinks=true
        }

    if [ -f "$output_file" ]; then
        local size=$(du -h "$output_file" | cut -f1)
        log_success "  Generated: $output_file ($size)"
    else
        log_error "  Failed to generate: $output_file"
        return 1
    fi
}

# Main
main() {
    echo ""
    log_info "=== Keystone Core PDF Book Generator ==="
    log_info "Using Pandoc + LaTeX for professional book output"
    echo ""

    check_dependencies

    echo ""
    log_info "=== Generating Section PDFs ==="

    # Define sections
    declare -A sections=(
        ["getting-started"]="Getting Started"
        ["concepts"]="Core Concepts"
        ["reference"]="Reference"
        ["operations"]="Operations Guide"
        ["community"]="Community"
    )

    for section in "${!sections[@]}"; do
        title="${sections[$section]}"
        md_file="$OUTPUT_DIR/${section}.md"
        pdf_file="$OUTPUT_DIR/keystone-core-${section}-book.pdf"

        generate_combined_markdown "$section" "$title"
        generate_pdf_pandoc "$md_file" "$pdf_file" "$title"

        # Cleanup intermediate markdown
        rm -f "$md_file"
    done

    echo ""
    log_info "=== Generating Complete Documentation ==="

    # Generate complete book
    complete_md="$OUTPUT_DIR/complete.md"
    complete_pdf="$OUTPUT_DIR/keystone-core-complete-book.pdf"

    cat > "$complete_md" << EOF
---
title: "Keystone Core"
subtitle: "Complete Documentation"
author: "Keystone Core Team"
date: "$(date '+%Y-%m-%d')"
documentclass: report
papersize: a4
geometry: margin=2.5cm
fontsize: 11pt
toc: true
toc-depth: 3
colorlinks: true
linkcolor: blue
urlcolor: blue
header-includes:
  - \usepackage{fancyhdr}
  - \pagestyle{fancy}
  - \fancyhead[L]{Keystone Core}
  - \fancyhead[R]{\leftmark}
  - \fancyfoot[C]{\thepage}
---

EOF

    # Combine all sections
    for section in "getting-started" "concepts" "reference" "operations" "community"; do
        title="${sections[$section]}"
        echo "" >> "$complete_md"
        echo "# $title" >> "$complete_md"
        echo "" >> "$complete_md"

        if [ -d "$CONTENT_DIR/$section" ]; then
            # Get _index.md first
            if [ -f "$CONTENT_DIR/$section/_index.md" ]; then
                sed -n '/^---$/,/^---$/d; p' "$CONTENT_DIR/$section/_index.md" >> "$complete_md"
            fi

            # Then get all other .md files
            find "$CONTENT_DIR/$section" -name "*.md" ! -name "_index.md" -type f | sort | while read -r md_file; do
                echo "" >> "$complete_md"
                echo "\\newpage" >> "$complete_md"
                echo "" >> "$complete_md"
                sed -n '/^---$/,/^---$/d; p' "$md_file" >> "$complete_md"
            done
        fi
    done

    generate_pdf_pandoc "$complete_md" "$complete_pdf" "Complete Documentation"

    # Cleanup
    rm -f "$complete_md"

    echo ""
    log_success "=== PDF Book Generation Complete ==="
    log_info "Output directory: $OUTPUT_DIR/"
    echo ""

    # List generated files
    ls -lh "$OUTPUT_DIR"/*.pdf 2>/dev/null | awk '{print "  " $NF " (" $5 ")"}'

    echo ""
    log_success "Done!"
}

main "$@"
