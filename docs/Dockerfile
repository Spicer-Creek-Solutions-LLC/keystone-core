# Dockerfile for Keystone Core documentation PDF generation
# Supports both Playwright/Paged.js and Pandoc/LaTeX PDF generation methods
#
# Build: docker build -t kscore-docs .
# Usage: docker run --rm -v $(pwd)/..:/workspace kscore-docs make docs-pdf

FROM node:20-bookworm

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    # Hugo dependencies
    curl \
    git \
    # Pandoc and LaTeX for book-quality PDFs
    pandoc \
    texlive-latex-base \
    texlive-latex-recommended \
    texlive-latex-extra \
    texlive-fonts-recommended \
    texlive-fonts-extra \
    texlive-xetex \
    lmodern \
    # Pygments for syntax highlighting
    python3-pygments \
    # Chromium and its dependencies (used by Playwright and Puppeteer/mermaid-cli)
    chromium \
    libnss3 \
    libnspr4 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libxkbcommon0 \
    libxcomposite1 \
    libxdamage1 \
    libxfixes3 \
    libxrandr2 \
    libgbm1 \
    libasound2 \
    libpango-1.0-0 \
    libcairo2 \
    # Cleanup
    && rm -rf /var/lib/apt/lists/*

# Configure Puppeteer to use system Chromium (for mermaid-cli)
ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true
ENV PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium

# Install Hugo Extended (must be >= 0.146.0 for Docsy theme compatibility)
ARG HUGO_VERSION=0.153.0
RUN curl -L "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-amd64.deb" -o /tmp/hugo.deb \
    && dpkg -i /tmp/hugo.deb \
    && rm /tmp/hugo.deb

# Set working directory
WORKDIR /workspace/docs

# Create mermaid-cli puppeteer config for container environment
RUN cat > /puppeteer-config.json << 'EOF'
{
  "executablePath": "/usr/bin/chromium",
  "args": ["--no-sandbox", "--disable-setuid-sandbox", "--disable-dev-shm-usage"]
}
EOF

# Configure Playwright to use system Chromium
ENV PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/usr/bin/chromium

# Create entrypoint script that ensures dependencies are installed
RUN cat > /entrypoint.sh << 'ENTRY'
#!/bin/bash
set -e
npm ci
# Copy puppeteer config for mermaid-cli if not exists
if [ ! -f /workspace/docs/puppeteer-config.json ]; then
  cp /puppeteer-config.json /workspace/docs/puppeteer-config.json 2>/dev/null || true
fi
# Install Playwright browser
npx playwright install chromium --with-deps 2>/dev/null || true
exec "$@"
ENTRY
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]

# Default command
CMD ["npm", "run", "generate-pdfs"]
