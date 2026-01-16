#!/usr/bin/env bash

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
# Options:
#   --skip-mermaid    Skip Mermaid diagram rendering (faster, diagrams as code)
#
# For detailed setup: https://github.com/rootsongjc/pdf-book-exporter
# =============================================================================

set -e

# Parse command line options
SKIP_MERMAID=false
for arg in "$@"; do
    case $arg in
        --skip-mermaid)
            SKIP_MERMAID=true
            shift
            ;;
    esac
done

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

    # Check for mermaid-cli (mmdc)
    MMDC_PATH=""
    if command -v mmdc &> /dev/null; then
        MMDC_PATH="mmdc"
    elif [ -x "$SCRIPT_DIR/node_modules/.bin/mmdc" ]; then
        MMDC_PATH="$SCRIPT_DIR/node_modules/.bin/mmdc"
    elif [ -x "$PROJECT_ROOT/node_modules/.bin/mmdc" ]; then
        MMDC_PATH="$PROJECT_ROOT/node_modules/.bin/mmdc"
    fi

    if [ -z "$MMDC_PATH" ]; then
        log_warn "mermaid-cli (mmdc) not found - Mermaid diagrams will not be rendered"
        log_info "  Install with: cd docs && npm install"
    else
        log_success "  Found mermaid-cli: $MMDC_PATH"
    fi

    # Check for puppeteer config (needed for container environment)
    PUPPETEER_CONFIG=""
    if [ -f "$SCRIPT_DIR/puppeteer-config.json" ]; then
        PUPPETEER_CONFIG="$SCRIPT_DIR/puppeteer-config.json"
        log_success "  Found puppeteer config: $PUPPETEER_CONFIG"
    elif [ -f "/puppeteer-config.json" ]; then
        PUPPETEER_CONFIG="/puppeteer-config.json"
        log_success "  Found puppeteer config: $PUPPETEER_CONFIG"
    elif [ -x "/usr/bin/chromium" ]; then
        # Running in container - create puppeteer config dynamically
        PUPPETEER_CONFIG="$OUTPUT_DIR/puppeteer-config.json"
        cat > "$PUPPETEER_CONFIG" << 'PCONF'
{
  "executablePath": "/usr/bin/chromium",
  "args": ["--no-sandbox", "--disable-setuid-sandbox", "--disable-dev-shm-usage"]
}
PCONF
        log_success "  Created puppeteer config for container: $PUPPETEER_CONFIG"
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        log_error "Missing dependencies:"
        for dep in "${missing[@]}"; do
            echo "  - $dep"
        done
        echo ""
        log_info "Installation instructions:"
        echo "  macOS:   brew install pandoc && brew install mermaid-cli && brew install --cask basictex && sudo tlmgr install framed"
        echo "  Ubuntu:  sudo apt install pandoc texlive-full"
        echo "  Python:  pip3 install Pygments"
        echo ""
        log_warn "Falling back to standard PDF generator..."
        exec node "$SCRIPT_DIR/generate-pdfs.js"
    fi

    log_success "All dependencies found"
}

# Global variables
DIAGRAM_COUNTER=0
MMDC_PATH=""
PUPPETEER_CONFIG=""

# Sanitize Unicode characters that LaTeX cannot handle
# Replaces common Unicode symbols with LaTeX-safe equivalents
sanitize_unicode() {
    local input_file=$1
    local output_file=$2
    local perl_script=$(mktemp)

    # Write Perl script to temp file to avoid shell quoting issues
    cat > "$perl_script" << 'PERL_SCRIPT'
#!/usr/bin/perl
use utf8;
use open qw(:std :utf8);

while (<>) {
    # Checkmarks and status symbols
    s/✅/[OK]/g;
    s/❌/[X]/g;
    s/✓/[v]/g;
    s/✗/[x]/g;
    s/⚠️?/[WARN]/g;
    s/⛔/[STOP]/g;

    # Arrows
    s/→/->/g;
    s/←/<-/g;
    s/↔/<->/g;
    s/↑/^/g;
    s/↓/v/g;
    s/⬆/^/g;
    s/⬇/v/g;

    # Box drawing characters (for directory trees)
    s/├/|--/g;
    s/└/`--/g;
    s/│/|/g;
    s/─/-/g;
    s/┌/,--/g;
    s/┐/--./g;
    s/┘/--`/g;
    s/┬/-+-/g;
    s/┴/-+-/g;
    s/┤/--|/g;
    s/┼/-+-/g;

    # Bullets and shapes
    s/•/-/g;
    s/·/-/g;
    s/●/*/g;
    s/○/o/g;
    s/■/[#]/g;
    s/□/[ ]/g;
    s/▪/*/g;
    s/▫/o/g;
    s/▶/>/g;
    s/◀/</g;
    s/★/*/g;
    s/☆/*/g;
    s/✦/*/g;
    s/✧/*/g;

    # Punctuation and typography
    s/…/.../g;
    s/—/--/g;
    s/–/-/g;
    s/'/'/g;
    s/'/'/g;
    s/"/"/g;
    s/"/"/g;

    # Math symbols
    s/×/x/g;
    s/∩/n/g;

    # Emojis (replace with text equivalents)
    s/🎉/(party)/g;
    s/👍/(thumbsup)/g;
    s/💬/(comment)/g;
    s/📝/(note)/g;
    s/🚀/(rocket)/g;
    s/🤖/(bot)/g;

    # Remove variation selectors (invisible Unicode modifiers)
    s/\x{FE0F}//g;

    # Escape backslashes in Windows paths (registry, file paths)
    # This handles paths like HKLM:\SOFTWARE, C:\Program Files, etc.
    # Process multiple passes to catch all backslashes in paths like \Dir1\Dir2\Dir3

    # First pass: common Windows system directories
    s/\\(SOFTWARE|SYSTEM|Program Files|Windows|Users|Documents|AppData|ProgramData|Temp|Microsoft|CurrentControlSet|Control|Session Manager|Environment|Policies|Scripts|drivers|etc|kscore|KeystoneCore|System32)/\\textbackslash{}$1/g;

    # Second pass: Windows paths - backslash followed by capital letter word
    # BUT only after another \textbackslash{} (meaning we're in a path context)
    # This catches the second+ segments like SOFTWARE\MyApp after SOFTWARE was already converted
    # The pattern includes the leading backslash of the \textbackslash{} command
    s/(\\textbackslash\{\}[A-Za-z0-9_-]+)\\([A-Z][A-Za-z0-9_-]*)/$1\\textbackslash{}$2/g;

    # Third pass: common lowercase path components
    # Catches paths like \ca.crt, \agent.yaml, \bin\, \logs\
    s/\\(ca|cert|key|agent|bin|logs|config|apps|data|crt|yaml|exe|msi|ps1|myapp)/\\textbackslash{}$1/g;

    # Fourth pass: Windows paths after colon (C:\path, HKLM:\path)
    # Only match backslash after a colon to avoid breaking LaTeX commands
    s/:\\([a-zA-Z])/:\\textbackslash{}$1/g;

    print;
}
PERL_SCRIPT

    perl "$perl_script" "$input_file" > "$output_file"
    rm -f "$perl_script"
}

# Convert internal documentation links for PDF
# Removes link syntax for internal docs (keeps text), preserves external links
convert_internal_links() {
    local input_file=$1
    local output_file=$2
    local perl_script=$(mktemp)

    cat > "$perl_script" << 'PERL_SCRIPT'
#!/usr/bin/perl
use utf8;
use open qw(:std :utf8);

while (<>) {
    # Keep external links (http://, https://, mailto:) as-is
    # Only process internal documentation links

    # Remove /docs/ links - convert [text](/docs/...) to just "text"
    s/\[([^\]]+)\]\(\/docs\/[^)]+\)/$1/g;

    # Remove relative links like ../page/ or ./page/
    s/\[([^\]]+)\]\(\.\.\/[^)]+\)/$1/g;
    s/\[([^\]]+)\]\(\.\/[^)]+\)/$1/g;

    # Remove pure anchor links that reference sections we can't resolve
    # But keep #anchor links that look like they might be local (single word)
    # e.g., [text](#some-long-path/thing) -> text
    s/\[([^\]]+)\]\(#[^)]*\/[^)]*\)/$1/g;

    print;
}
PERL_SCRIPT

    perl "$perl_script" "$input_file" > "$output_file"
    rm -f "$perl_script"
}

# Convert Mermaid code blocks to images
process_mermaid_diagrams() {
    local input_file=$1
    local output_file=$2
    local diagram_dir="$OUTPUT_DIR/mermaid-diagrams"

    # Skip Mermaid processing if requested or mmdc not available
    if [ "$SKIP_MERMAID" = true ]; then
        log_info "    Skipping Mermaid rendering (--skip-mermaid)"
        cp "$input_file" "$output_file"
        return
    fi

    if [ -z "$MMDC_PATH" ]; then
        # No mmdc available, just copy the file
        log_warn "    mmdc not available, keeping Mermaid as code blocks"
        cp "$input_file" "$output_file"
        return
    fi

    mkdir -p "$diagram_dir"

    local diagram_count=0
    local temp_file=$(mktemp)
    local in_mermaid=false
    local mermaid_content=""

    while IFS= read -r line || [[ -n "$line" ]]; do
        if [[ "$line" =~ ^\`\`\`mermaid ]]; then
            in_mermaid=true
            mermaid_content=""
            continue
        elif [[ "$line" =~ ^\`\`\` ]] && [ "$in_mermaid" = true ]; then
            in_mermaid=false
            DIAGRAM_COUNTER=$((DIAGRAM_COUNTER + 1))
            diagram_count=$((diagram_count + 1))

            # Save mermaid content to temp file
            local mmd_file="$diagram_dir/diagram_${DIAGRAM_COUNTER}.mmd"
            local png_file="$diagram_dir/diagram_${DIAGRAM_COUNTER}.png"

            echo "$mermaid_content" > "$mmd_file"

            # Convert to PNG using mmdc (with puppeteer config if available)
            local mmdc_args="-i $mmd_file -o $png_file -b white -s 2"
            if [ -n "$PUPPETEER_CONFIG" ]; then
                mmdc_args="-p $PUPPETEER_CONFIG $mmdc_args"
            fi
            local mmdc_output
            if mmdc_output=$($MMDC_PATH $mmdc_args 2>&1) && [ -f "$png_file" ]; then
                # Insert image reference instead of code block
                echo "" >> "$temp_file"
                echo "![Diagram ${DIAGRAM_COUNTER}](${png_file})" >> "$temp_file"
                echo "" >> "$temp_file"
            else
                # If conversion fails, keep as code block and show why
                log_warn "    Failed to render Mermaid diagram ${DIAGRAM_COUNTER}"
                if [ -n "$mmdc_output" ]; then
                    log_warn "    Error: ${mmdc_output:0:200}"
                fi
                # Keep diagram as a plain code block for readability
                echo '```' >> "$temp_file"
                echo "# Mermaid Diagram (render failed)" >> "$temp_file"
                echo "$mermaid_content" >> "$temp_file"
                echo '```' >> "$temp_file"
            fi
            continue
        fi

        if [ "$in_mermaid" = true ]; then
            if [ -n "$mermaid_content" ]; then
                mermaid_content="${mermaid_content}"$'\n'"${line}"
            else
                mermaid_content="$line"
            fi
        else
            echo "$line" >> "$temp_file"
        fi
    done < "$input_file"

    mv "$temp_file" "$output_file"

    if [ $diagram_count -gt 0 ]; then
        log_info "    Converted $diagram_count Mermaid diagram(s) to images"
    fi
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

    # Copy logo to output directory for LaTeX to find
    local logo_path=""
    if [ -f "$SCRIPT_DIR/assets/icons/logo.png" ]; then
        mkdir -p "$OUTPUT_DIR"
        cp "$SCRIPT_DIR/assets/icons/logo.png" "$OUTPUT_DIR/logo.png"
        logo_path="$OUTPUT_DIR/logo.png"
    fi

    # Start with frontmatter and cover page
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
  - \usepackage{graphicx}
  - \pagestyle{fancy}
  - \fancyhead[L]{Keystone Core}
  - \fancyhead[R]{${title}}
  - \fancyfoot[C]{\thepage}
---

\begin{titlepage}
\centering
\vspace*{2cm}
EOF

    # Add logo if available
    if [ -n "$logo_path" ] && [ -f "$logo_path" ]; then
        cat >> "$output_file" << EOF
\includegraphics[width=0.35\textwidth]{${logo_path}}
\vspace{1.5cm}
EOF
    fi

    cat >> "$output_file" << EOF

{\Huge\bfseries Keystone Core\par}
\vspace{0.5cm}
{\Large Cloud-Native Runtime Infrastructure Control Plane\par}
\vspace{2cm}
{\LARGE\bfseries ${title}\par}
\vspace{2cm}
{\large\itshape GitOps deploys it. We keep it running.\par}
\vspace{3cm}
{\large $(date '+%B %Y')\par}
\vfill
{\small Keystone Core Team\par}
\end{titlepage}

\newpage

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
        md_sanitized="$OUTPUT_DIR/${section}-sanitized.md"
        md_links="$OUTPUT_DIR/${section}-links.md"
        md_processed="$OUTPUT_DIR/${section}-processed.md"
        pdf_file="$OUTPUT_DIR/keystone-core-${section}-book.pdf"

        generate_combined_markdown "$section" "$title"
        sanitize_unicode "$md_file" "$md_sanitized"
        convert_internal_links "$md_sanitized" "$md_links"
        process_mermaid_diagrams "$md_links" "$md_processed"
        generate_pdf_pandoc "$md_processed" "$pdf_file" "$title"

        # Cleanup intermediate markdown
        rm -f "$md_file" "$md_sanitized" "$md_links" "$md_processed"
    done

    echo ""
    log_info "=== Generating Complete Documentation ==="

    # Generate complete book
    complete_md="$OUTPUT_DIR/complete.md"
    complete_pdf="$OUTPUT_DIR/keystone-core-complete-book.pdf"

    # Copy logo to output directory for LaTeX to find
    LOGO_PATH=""
    if [ -f "$SCRIPT_DIR/assets/icons/logo.png" ]; then
        cp "$SCRIPT_DIR/assets/icons/logo.png" "$OUTPUT_DIR/logo.png"
        LOGO_PATH="$OUTPUT_DIR/logo.png"
    elif [ -f "$SCRIPT_DIR/static/images/logo.svg" ]; then
        # SVG needs conversion, skip logo if only SVG available
        log_warn "  Only SVG logo available, skipping cover logo"
    fi

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
  - \usepackage{graphicx}
  - \usepackage{titling}
  - \pagestyle{fancy}
  - \fancyhead[L]{Keystone Core}
  - \fancyhead[R]{\leftmark}
  - \fancyfoot[C]{\thepage}
---

\begin{titlepage}
\centering
\vspace*{2cm}
EOF

    # Add logo if available
    if [ -n "$LOGO_PATH" ] && [ -f "$LOGO_PATH" ]; then
        cat >> "$complete_md" << EOF
\includegraphics[width=0.4\textwidth]{${LOGO_PATH}}
\vspace{1.5cm}
EOF
    fi

    cat >> "$complete_md" << EOF

{\Huge\bfseries Keystone Core\par}
\vspace{0.5cm}
{\Large Cloud-Native Runtime Infrastructure Control Plane\par}
\vspace{2cm}
{\large\itshape GitOps deploys it. We keep it running.\par}
\vspace{3cm}
{\large Complete Documentation\par}
\vspace{1cm}
{\large $(date '+%B %Y')\par}
\vfill
{\small Keystone Core Team\par}
\end{titlepage}

\newpage

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

    # Sanitize Unicode, convert links, and process Mermaid diagrams
    complete_md_sanitized="$OUTPUT_DIR/complete-sanitized.md"
    complete_md_links="$OUTPUT_DIR/complete-links.md"
    complete_md_processed="$OUTPUT_DIR/complete-processed.md"
    sanitize_unicode "$complete_md" "$complete_md_sanitized"
    convert_internal_links "$complete_md_sanitized" "$complete_md_links"
    process_mermaid_diagrams "$complete_md_links" "$complete_md_processed"

    generate_pdf_pandoc "$complete_md_processed" "$complete_pdf" "Complete Documentation"

    # Cleanup
    rm -f "$complete_md" "$complete_md_sanitized" "$complete_md_links" "$complete_md_processed"
    rm -rf "$OUTPUT_DIR/mermaid-diagrams"
    rm -f "$OUTPUT_DIR/logo.png"  # Cleanup copied logo

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
