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
    # Playwright/Chromium dependencies
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

# Install Hugo Extended (must be >= 0.146.0 for Docsy theme compatibility)
ARG HUGO_VERSION=0.153.0
RUN curl -L "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-amd64.deb" -o /tmp/hugo.deb \
    && dpkg -i /tmp/hugo.deb \
    && rm /tmp/hugo.deb

# Set working directory
WORKDIR /workspace/docs

# Create entrypoint script that ensures Playwright browser is installed
RUN echo '#!/bin/bash\n\
set -e\n\
npm ci\n\
npx playwright install chromium --with-deps 2>/dev/null || true\n\
exec "$@"' > /entrypoint.sh && chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]

# Default command
CMD ["npm", "run", "generate-pdfs"]
